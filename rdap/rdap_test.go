package rdap_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"
	"time"

	"github.com/sitapix/dibs/rdap"
)

const testBootstrapJSON = `{
  "version": "1.0",
  "services": [
    [["com", "net"], ["https://rdap.verisign.com/com/v1/"]],
    [["org"], ["https://rdap.publicinterestregistry.org/rdap/"]],
    [["xyz"], ["https://rdap.centralnic.com/xyz/"]]
  ]
}`

func TestParseBootstrap(t *testing.T) {
	bs, err := rdap.ParseBootstrap([]byte(testBootstrapJSON))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	tests := []struct {
		tld  string
		want string
	}{
		{"com", "https://rdap.verisign.com/com/v1/"},
		{"net", "https://rdap.verisign.com/com/v1/"},
		{"org", "https://rdap.publicinterestregistry.org/rdap/"},
		{"xyz", "https://rdap.centralnic.com/xyz/"},
	}
	for _, tt := range tests {
		got, ok := bs[tt.tld]
		if !ok {
			t.Errorf("missing TLD %q", tt.tld)
			continue
		}
		if got != tt.want {
			t.Errorf("bootstrap[%q] = %q, want %q", tt.tld, got, tt.want)
		}
	}
}

func TestParseBootstrap_MissingTLD(t *testing.T) {
	bs, _ := rdap.ParseBootstrap([]byte(testBootstrapJSON))
	if _, ok := bs["notinthere"]; ok {
		t.Error("expected missing TLD to not be in bootstrap")
	}
}

func TestParseBootstrap_Invalid(t *testing.T) {
	_, err := rdap.ParseBootstrap([]byte(`not json`))
	if err == nil {
		t.Error("expected error for invalid JSON")
	}
}

func TestLookup_Registered(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/rdap+json")
		w.WriteHeader(200)
		_, _ = w.Write([]byte(`{"ldhName":"example.com","status":["active"]}`))
	}))
	defer srv.Close()

	bs := rdap.Bootstrap{"com": srv.URL + "/"}
	client := rdap.NewClient(bs, 5*time.Second)
	result := client.Lookup(context.Background(), "example.com", "com")

	if result.Status != rdap.Registered {
		t.Errorf("expected Registered, got %v", result.Status)
	}
}

func TestLookup_NotFound(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(404)
	}))
	defer srv.Close()

	bs := rdap.Bootstrap{"com": srv.URL + "/"}
	client := rdap.NewClient(bs, 5*time.Second)
	result := client.Lookup(context.Background(), "notregistered.com", "com")

	if result.Status != rdap.NotFound {
		t.Errorf("expected NotFound, got %v", result.Status)
	}
}

func TestLookup_NoServer(t *testing.T) {
	bs := rdap.Bootstrap{}
	client := rdap.NewClient(bs, 5*time.Second)
	result := client.Lookup(context.Background(), "example.io", "io")

	if result.Status != rdap.Unavailable {
		t.Errorf("expected Unavailable, got %v", result.Status)
	}
}

func TestLookup_ServerError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(500)
	}))
	defer srv.Close()

	bs := rdap.Bootstrap{"com": srv.URL + "/"}
	client := rdap.NewClient(bs, 5*time.Second)
	result := client.Lookup(context.Background(), "example.com", "com")

	if result.Status != rdap.Error {
		t.Errorf("expected Error, got %v", result.Status)
	}
}

func TestLookup_Timeout(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(2 * time.Second)
		w.WriteHeader(200)
	}))
	defer srv.Close()

	bs := rdap.Bootstrap{"com": srv.URL + "/"}
	client := rdap.NewClient(bs, 100*time.Millisecond)
	result := client.Lookup(context.Background(), "example.com", "com")

	if result.Status != rdap.Error {
		t.Errorf("expected Error on timeout, got %v", result.Status)
	}
}

func TestCacheBootstrap(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "rdap-bootstrap.json")

	err := rdap.WriteBootstrapCache(path, []byte(testBootstrapJSON))
	if err != nil {
		t.Fatalf("write cache: %v", err)
	}

	data, fresh, err := rdap.ReadBootstrapCache(path, 24*time.Hour)
	if err != nil {
		t.Fatalf("read cache: %v", err)
	}
	if !fresh {
		t.Error("expected fresh cache")
	}

	bs, err := rdap.ParseBootstrap(data)
	if err != nil {
		t.Fatalf("parse cached: %v", err)
	}
	if _, ok := bs["com"]; !ok {
		t.Error("expected com in cached bootstrap")
	}
}

func TestCacheBootstrap_Missing(t *testing.T) {
	path := filepath.Join(t.TempDir(), "nonexistent.json")
	_, _, err := rdap.ReadBootstrapCache(path, 24*time.Hour)
	if err != nil {
		t.Errorf("missing file should return nil error, got: %v", err)
	}
}
