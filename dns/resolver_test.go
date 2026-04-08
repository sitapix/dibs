package dns_test

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"github.com/sitapix/dibs/dns"
)

// wireResponse builds a minimal DNS wire-format response with the given RCODE.
func wireResponse(rcode int) []byte {
	return []byte{
		0x00, 0x00, // ID
		0x81,                    // QR=1, RD=1
		0x80 | byte(rcode&0x0F), // RA=1, RCODE
		0x00, 0x00,              // QDCOUNT
		0x00, 0x00, // ANCOUNT
		0x00, 0x00, // NSCOUNT
		0x00, 0x00, // ARCOUNT
	}
}

// wireHandler returns an http.Handler that accepts POST application/dns-message
// and responds with the given RCODE.
func wireHandler(rcode int) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "want POST", http.StatusMethodNotAllowed)
			return
		}
		_, _ = io.Copy(io.Discard, r.Body)
		w.Header().Set("Content-Type", "application/dns-message")
		_, _ = w.Write(wireResponse(rcode))
	})
}

func TestDoH_NXDOMAIN(t *testing.T) {
	srv := httptest.NewServer(wireHandler(3))
	defer srv.Close()

	provider := dns.Provider{Name: "test", URL: srv.URL}
	resolver := dns.NewDoHResolver([]dns.Provider{provider}, false, 5*time.Second)

	result := resolver.Lookup(context.Background(), "nonexistent.example")
	if result.Status != dns.StatusAvailable {
		t.Errorf("expected StatusAvailable, got %v (error: %s)", result.Status, result.Error)
	}
	if result.Domain != "nonexistent.example" {
		t.Errorf("expected domain nonexistent.example, got %s", result.Domain)
	}
}

func TestDoH_NOERROR(t *testing.T) {
	srv := httptest.NewServer(wireHandler(0))
	defer srv.Close()

	provider := dns.Provider{Name: "test", URL: srv.URL}
	resolver := dns.NewDoHResolver([]dns.Provider{provider}, false, 5*time.Second)

	result := resolver.Lookup(context.Background(), "google.com")
	if result.Status != dns.StatusTaken {
		t.Errorf("expected StatusTaken, got %v (error: %s)", result.Status, result.Error)
	}
}

func TestDoH_SERVFAIL(t *testing.T) {
	srv := httptest.NewServer(wireHandler(2))
	defer srv.Close()

	provider := dns.Provider{Name: "test", URL: srv.URL}
	resolver := dns.NewDoHResolver([]dns.Provider{provider}, false, 5*time.Second)

	result := resolver.Lookup(context.Background(), "broken.example")
	if result.Status != dns.StatusError {
		t.Errorf("expected StatusError, got %v", result.Status)
	}
	if result.Error == "" {
		t.Error("expected non-empty error message")
	}
}

func TestDoH_Timeout(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(2 * time.Second)
		_, _ = w.Write(wireResponse(0))
	}))
	defer srv.Close()

	provider := dns.Provider{Name: "test", URL: srv.URL}
	resolver := dns.NewDoHResolver([]dns.Provider{provider}, false, 100*time.Millisecond)

	start := time.Now()
	result := resolver.Lookup(context.Background(), "timeout.example")
	elapsed := time.Since(start)

	if result.Status != dns.StatusError {
		t.Errorf("expected StatusError on timeout, got %v", result.Status)
	}
	if elapsed > time.Second {
		t.Errorf("lookup took too long: %v (expected ~100ms)", elapsed)
	}
}

func TestDoH_Rotation(t *testing.T) {
	var count1, count2 atomic.Int64

	srv1 := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		count1.Add(1)
		w.Header().Set("Content-Type", "application/dns-message")
		_, _ = w.Write(wireResponse(3))
	}))
	defer srv1.Close()

	srv2 := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		count2.Add(1)
		w.Header().Set("Content-Type", "application/dns-message")
		_, _ = w.Write(wireResponse(3))
	}))
	defer srv2.Close()

	providers := []dns.Provider{
		{Name: "p1", URL: srv1.URL},
		{Name: "p2", URL: srv2.URL},
	}
	resolver := dns.NewDoHResolver(providers, true, 5*time.Second)

	for i := 0; i < 4; i++ {
		resolver.Lookup(context.Background(), fmt.Sprintf("test%d.example", i))
	}

	c1 := count1.Load()
	c2 := count2.Load()
	if c1 != 2 || c2 != 2 {
		t.Errorf("expected 2 calls each, got provider1=%d provider2=%d", c1, c2)
	}
}

// TestDoH_NoUserAgent verifies that DoH requests do not carry a User-Agent
// header on the wire. Go's net/http auto-adds "User-Agent: Go-http-client/1.1"
// when the header isn't explicitly set, which would leak the Go runtime to
// every DoH resolver. Privacy regression guard; matches Firefox bug 1543201.
func TestDoH_NoUserAgent(t *testing.T) {
	var gotUserAgent []string
	var seen atomic.Bool
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotUserAgent = r.Header["User-Agent"]
		seen.Store(true)
		w.Header().Set("Content-Type", "application/dns-message")
		_, _ = w.Write(wireResponse(3))
	}))
	defer srv.Close()

	provider := dns.Provider{Name: "test", URL: srv.URL}
	resolver := dns.NewDoHResolver([]dns.Provider{provider}, false, 5*time.Second)
	resolver.Lookup(context.Background(), "privacy.example")

	if !seen.Load() {
		t.Fatal("server never saw a request")
	}
	if len(gotUserAgent) != 0 {
		t.Errorf("DoH request leaked User-Agent header: %v; want none", gotUserAgent)
	}
}

func TestDoH_TruncatedResponse(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/dns-message")
		_, _ = w.Write([]byte{0x00, 0x00}) // too short
	}))
	defer srv.Close()

	provider := dns.Provider{Name: "test", URL: srv.URL}
	resolver := dns.NewDoHResolver([]dns.Provider{provider}, false, 5*time.Second)

	result := resolver.Lookup(context.Background(), "truncated.example")
	if result.Status != dns.StatusError {
		t.Errorf("expected StatusError for truncated response, got %v", result.Status)
	}
}

func TestSystemResolver_GoogleTaken(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping system resolver test in short mode")
	}
	resolver := dns.NewSystemResolver(5 * time.Second)
	result := resolver.Lookup(context.Background(), "google.com")
	if result.Status != dns.StatusTaken {
		t.Errorf("expected google.com to be StatusTaken, got %v (error: %s)", result.Status, result.Error)
	}
}

func TestDomainStatus_String(t *testing.T) {
	cases := []struct {
		status dns.DomainStatus
		want   string
	}{
		{dns.StatusAvailable, "available"},
		{dns.StatusTaken, "taken"},
		{dns.StatusError, "error"},
	}
	for _, c := range cases {
		got := c.status.String()
		if got != c.want {
			t.Errorf("DomainStatus(%d).String() = %q, want %q", c.status, got, c.want)
		}
	}
}

func TestProvidersMap(t *testing.T) {
	for _, name := range dns.ProviderNames() {
		p, ok := dns.GetProvider(name)
		if !ok {
			t.Errorf("provider %q not found", name)
			continue
		}
		if p.URL == "" {
			t.Errorf("provider %q has empty URL", name)
		}
	}
}

func TestProviderNames(t *testing.T) {
	names := dns.ProviderNames()
	if len(names) != 4 {
		t.Fatalf("expected 4 providers, got %d", len(names))
	}
	expected := map[string]bool{"quad9": true, "mullvad": true, "nextdns": true, "adguard": true}
	for _, name := range names {
		if !expected[name] {
			t.Errorf("unexpected provider: %s", name)
		}
	}
}
