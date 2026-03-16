package output

import (
	"bytes"
	"strings"
	"testing"

	"github.com/sitapix/dibs/dns"
)

func makeResult(domain, tld string, status dns.DomainStatus, errMsg string) dns.Result {
	return dns.Result{Domain: domain, TLD: tld, Status: status, Error: errMsg}
}

func TestTerminal_ShowsAvailable(t *testing.T) {
	var buf bytes.Buffer
	r := NewTerminalRenderer(&buf, false, true)
	r.Start("mybrand", 3)
	r.Render(makeResult("mybrand.xyz", "xyz", dns.StatusAvailable, ""))
	r.Finish(false)

	out := buf.String()
	if !strings.Contains(out, "mybrand.xyz") {
		t.Errorf("expected output to contain mybrand.xyz, got: %q", out)
	}
	if !strings.Contains(out, "available") {
		t.Errorf("expected output to contain 'available', got: %q", out)
	}
}

func TestTerminal_QuietHidesTaken(t *testing.T) {
	var buf bytes.Buffer
	r := NewTerminalRenderer(&buf, true, true)
	r.Start("mybrand", 3)
	r.Render(makeResult("mybrand.com", "com", dns.StatusTaken, ""))
	r.Render(makeResult("mybrand.xyz", "xyz", dns.StatusAvailable, ""))
	r.Finish(false)

	out := buf.String()
	if strings.Contains(out, "mybrand.com") {
		t.Errorf("quiet mode should hide taken domain mybrand.com, got: %q", out)
	}
	if !strings.Contains(out, "mybrand.xyz") {
		t.Errorf("quiet mode should still show available mybrand.xyz, got: %q", out)
	}
}

func TestTerminal_NoColorHasNoANSI(t *testing.T) {
	var buf bytes.Buffer
	r := NewTerminalRenderer(&buf, false, true)
	r.Start("mybrand", 2)
	r.Render(makeResult("mybrand.com", "com", dns.StatusTaken, ""))
	r.Render(makeResult("mybrand.xyz", "xyz", dns.StatusAvailable, ""))
	r.Finish(false)

	out := buf.String()
	if strings.Contains(out, "\x1b[") {
		t.Errorf("noColor mode should not contain ANSI escape codes, got: %q", out)
	}
}

func TestTerminal_SummaryShowsCountAndPercent(t *testing.T) {
	var buf bytes.Buffer
	r := NewTerminalRenderer(&buf, false, true)
	r.Start("mybrand", 4)
	r.Render(makeResult("mybrand.com", "com", dns.StatusTaken, ""))
	r.Render(makeResult("mybrand.xyz", "xyz", dns.StatusAvailable, ""))
	r.Render(makeResult("mybrand.io", "io", dns.StatusAvailable, ""))
	r.Render(makeResult("mybrand.museum", "museum", dns.StatusError, "timeout"))
	r.Finish(false)

	out := buf.String()
	// Should say "2" available out of "4" checked with "50%"
	if !strings.Contains(out, "2") {
		t.Errorf("summary should contain count of available (2), got: %q", out)
	}
	if !strings.Contains(out, "4") {
		t.Errorf("summary should contain total checked (4), got: %q", out)
	}
	if !strings.Contains(out, "50%") {
		t.Errorf("summary should contain percentage (50%%), got: %q", out)
	}
}

func TestTerminal_PartialShowsInterrupted(t *testing.T) {
	var buf bytes.Buffer
	r := NewTerminalRenderer(&buf, false, true)
	r.Start("mybrand", 10)
	r.Render(makeResult("mybrand.com", "com", dns.StatusTaken, ""))
	r.Render(makeResult("mybrand.xyz", "xyz", dns.StatusAvailable, ""))
	r.Finish(true)

	out := buf.String()
	if !strings.Contains(strings.ToLower(out), "interrupt") {
		t.Errorf("partial finish should mention 'interrupt', got: %q", out)
	}
}

func TestTerminal_ProgressBarFormat(t *testing.T) {
	r := &TerminalRenderer{
		total:   25,
		checked: 18,
	}
	bar := r.progressBar()

	if !strings.Contains(bar, "18/25") {
		t.Errorf("progress bar should contain '18/25', got: %q", bar)
	}
	if !strings.Contains(bar, "72%") {
		t.Errorf("progress bar should contain '72%%', got: %q", bar)
	}
}

func TestTerminalRenderer_ApplyVerification(t *testing.T) {
	var buf bytes.Buffer
	r := NewTerminalRenderer(&buf, false, true)
	r.Start("test", 3)
	r.Render(dns.Result{Domain: "a.com", TLD: "com", Status: dns.StatusAvailable})
	r.Render(dns.Result{Domain: "b.org", TLD: "org", Status: dns.StatusAvailable})
	r.Render(dns.Result{Domain: "c.net", TLD: "net", Status: dns.StatusTaken})

	corrections := []dns.Result{{Domain: "a.com", TLD: "com", Status: dns.StatusTaken}}
	stats := VerifyStats{Checked: 2, Verified: 1, Unverified: 1}
	r.ApplyVerification(corrections, stats)
	r.Finish(false)

	out := buf.String()
	if !strings.Contains(out, "registered, no DNS") {
		t.Error("expected correction line in output")
	}
	if !strings.Contains(out, "1 available") {
		t.Errorf("expected 1 available after correction, got: %s", out)
	}
	if !strings.Contains(out, "Verified") {
		t.Error("expected verify summary in output")
	}
}
