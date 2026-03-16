package output

import (
	"bytes"
	"encoding/json"
	"io"
	"testing"

	"github.com/sitapix/dibs/dns"
)

func TestJSON_ValidStructure(t *testing.T) {
	var buf bytes.Buffer
	r := NewJSONRenderer(&buf, io.Discard)
	r.Start("mybrand", 3)
	r.Render(makeResult("mybrand.xyz", "xyz", dns.StatusAvailable, ""))
	r.Render(makeResult("mybrand.com", "com", dns.StatusTaken, ""))
	r.Render(makeResult("mybrand.museum", "museum", dns.StatusError, "timeout"))
	r.Finish(false)

	var out map[string]interface{}
	if err := json.Unmarshal(buf.Bytes(), &out); err != nil {
		t.Fatalf("output is not valid JSON: %v\nOutput: %s", err, buf.String())
	}

	if out["query"] != "mybrand" {
		t.Errorf("expected query='mybrand', got %v", out["query"])
	}
	if out["checked"].(float64) != 3 {
		t.Errorf("expected checked=3, got %v", out["checked"])
	}
	if out["partial"].(bool) != false {
		t.Errorf("expected partial=false, got %v", out["partial"])
	}

	available, ok := out["available"].([]interface{})
	if !ok {
		t.Fatalf("expected 'available' to be an array, got %T", out["available"])
	}
	if len(available) != 1 {
		t.Errorf("expected 1 available domain, got %d", len(available))
	}

	taken, ok := out["taken"].([]interface{})
	if !ok {
		t.Fatalf("expected 'taken' to be an array, got %T", out["taken"])
	}
	if len(taken) != 1 {
		t.Errorf("expected 1 taken domain, got %d", len(taken))
	}

	errors, ok := out["errors"].([]interface{})
	if !ok {
		t.Fatalf("expected 'errors' to be an array, got %T", out["errors"])
	}
	if len(errors) != 1 {
		t.Errorf("expected 1 error domain, got %d", len(errors))
	}

	// Check error entry has error field
	errEntry := errors[0].(map[string]interface{})
	if errEntry["error"] != "timeout" {
		t.Errorf("expected error='timeout', got %v", errEntry["error"])
	}
}

func TestJSON_PartialFlag(t *testing.T) {
	var buf bytes.Buffer
	r := NewJSONRenderer(&buf, io.Discard)
	r.Start("mybrand", 10)
	r.Render(makeResult("mybrand.com", "com", dns.StatusTaken, ""))
	r.Finish(true)

	var out map[string]interface{}
	if err := json.Unmarshal(buf.Bytes(), &out); err != nil {
		t.Fatalf("output is not valid JSON: %v", err)
	}
	if out["partial"].(bool) != true {
		t.Errorf("expected partial=true, got %v", out["partial"])
	}
}

func TestJSON_EmptyArraysNotNull(t *testing.T) {
	var buf bytes.Buffer
	r := NewJSONRenderer(&buf, io.Discard)
	r.Start("mybrand", 1)
	r.Render(makeResult("mybrand.com", "com", dns.StatusTaken, ""))
	r.Finish(false)

	outStr := buf.String()

	// available and errors should be [] not null
	var out map[string]interface{}
	if err := json.Unmarshal(buf.Bytes(), &out); err != nil {
		t.Fatalf("output is not valid JSON: %v", err)
	}

	available := out["available"]
	if available == nil {
		t.Errorf("'available' should be [] not null, got nil in JSON: %s", outStr)
	}

	errors := out["errors"]
	if errors == nil {
		t.Errorf("'errors' should be [] not null, got nil in JSON: %s", outStr)
	}
}

func TestJSON_DomainAndTLDFields(t *testing.T) {
	var buf bytes.Buffer
	r := NewJSONRenderer(&buf, io.Discard)
	r.Start("mybrand", 1)
	r.Render(makeResult("mybrand.xyz", "xyz", dns.StatusAvailable, ""))
	r.Finish(false)

	var out map[string]interface{}
	if err := json.Unmarshal(buf.Bytes(), &out); err != nil {
		t.Fatalf("output is not valid JSON: %v", err)
	}

	available := out["available"].([]interface{})
	entry := available[0].(map[string]interface{})
	if entry["domain"] != "mybrand.xyz" {
		t.Errorf("expected domain='mybrand.xyz', got %v", entry["domain"])
	}
	if entry["tld"] != "xyz" {
		t.Errorf("expected tld='xyz', got %v", entry["tld"])
	}
}

func TestJSONRenderer_ApplyVerification(t *testing.T) {
	var buf bytes.Buffer
	r := NewJSONRenderer(&buf, io.Discard)
	r.Start("test", 3)
	r.Render(dns.Result{Domain: "a.com", TLD: "com", Status: dns.StatusAvailable})
	r.Render(dns.Result{Domain: "b.org", TLD: "org", Status: dns.StatusAvailable})
	r.Render(dns.Result{Domain: "c.net", TLD: "net", Status: dns.StatusTaken})

	corrections := []dns.Result{{Domain: "a.com", TLD: "com", Status: dns.StatusTaken}}
	stats := VerifyStats{Checked: 2, Verified: 1, Unverified: 1}
	r.ApplyVerification(corrections, stats)
	r.Finish(false)

	var doc map[string]any
	if err := json.Unmarshal(buf.Bytes(), &doc); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}

	available := doc["available"].([]any)
	if len(available) != 1 {
		t.Errorf("expected 1 available after correction, got %d", len(available))
	}

	verify := doc["verify"].(map[string]any)
	if int(verify["corrected"].(float64)) != 1 {
		t.Error("expected corrected=1 in verify")
	}
}
