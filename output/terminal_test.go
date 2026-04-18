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

// TestTerminalRenderer_BeginVerificationClearsProgressBarOnTTY pins the
// ordering fix: when writing to a TTY, the clearLine escape must appear
// before the "Verifying..." banner so they don't collide on the same line
// as the in-place progress bar. On non-TTY output the escape is suppressed.
func TestTerminalRenderer_BeginVerificationClearsProgressBarOnTTY(t *testing.T) {
	t.Run("tty emits clearLine before banner", func(t *testing.T) {
		var buf bytes.Buffer
		// Bypass NewTerminalRenderer's auto-detect (a bytes.Buffer is never a
		// TTY) by constructing the struct directly with isTTY=true.
		r := &TerminalRenderer{w: &buf, isTTY: true}
		r.BeginVerification(7)

		out := buf.String()
		clearIdx := strings.Index(out, clearLine)
		bannerIdx := strings.Index(out, "Verifying 7 available domains via RDAP...")
		if clearIdx < 0 {
			t.Fatalf("expected clearLine escape in output, got: %q", out)
		}
		if bannerIdx < 0 {
			t.Fatalf("expected banner in output, got: %q", out)
		}
		if clearIdx >= bannerIdx {
			t.Errorf("clearLine must precede banner: clearIdx=%d bannerIdx=%d, got: %q", clearIdx, bannerIdx, out)
		}
	})

	t.Run("non-tty skips clearLine", func(t *testing.T) {
		var buf bytes.Buffer
		r := &TerminalRenderer{w: &buf, isTTY: false}
		r.BeginVerification(3)

		out := buf.String()
		if strings.Contains(out, clearLine) {
			t.Errorf("non-TTY output should not contain clearLine escape, got: %q", out)
		}
		if !strings.Contains(out, "Verifying 3 available domains via RDAP...") {
			t.Errorf("expected banner in output, got: %q", out)
		}
	})
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
