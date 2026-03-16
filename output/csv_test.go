package output

import (
	"bytes"
	"io"
	"strings"
	"testing"

	"github.com/sitapix/dibs/dns"
)

func TestCSV_HeaderAndRows(t *testing.T) {
	var buf, stderr bytes.Buffer
	r := NewCSVRenderer(&buf, &stderr)
	r.Start("mybrand", 3)
	r.Render(makeResult("mybrand.xyz", "xyz", dns.StatusAvailable, ""))
	r.Render(makeResult("mybrand.com", "com", dns.StatusTaken, ""))
	r.Render(makeResult("mybrand.museum", "museum", dns.StatusError, "timeout"))
	r.Finish(false)

	out := buf.String()
	lines := strings.Split(strings.TrimRight(out, "\n"), "\n")

	if len(lines) < 4 {
		t.Fatalf("expected at least 4 lines (header + 3 rows), got %d: %q", len(lines), out)
	}

	// Header
	if lines[0] != "domain,tld,status" {
		t.Errorf("expected header 'domain,tld,status', got %q", lines[0])
	}

	// Available row
	if lines[1] != "mybrand.xyz,xyz,available" {
		t.Errorf("expected row 'mybrand.xyz,xyz,available', got %q", lines[1])
	}

	// Taken row
	if lines[2] != "mybrand.com,com,taken" {
		t.Errorf("expected row 'mybrand.com,com,taken', got %q", lines[2])
	}

	// Error row
	if lines[3] != "mybrand.museum,museum,error" {
		t.Errorf("expected row 'mybrand.museum,museum,error', got %q", lines[3])
	}
}

func TestCSV_PartialWritesWarningToStderr(t *testing.T) {
	var buf, stderr bytes.Buffer
	r := NewCSVRenderer(&buf, &stderr)
	r.Start("mybrand", 10)
	r.Render(makeResult("mybrand.com", "com", dns.StatusTaken, ""))
	r.Finish(true)

	stderrOut := stderr.String()
	if !strings.Contains(strings.ToLower(stderrOut), "warning") {
		t.Errorf("partial finish should write warning to stderr, got: %q", stderrOut)
	}
	if !strings.Contains(strings.ToLower(stderrOut), "interrupt") {
		t.Errorf("partial finish warning should mention interrupt, got: %q", stderrOut)
	}
}

func TestCSV_NoStderrOnComplete(t *testing.T) {
	var buf, stderr bytes.Buffer
	r := NewCSVRenderer(&buf, &stderr)
	r.Start("mybrand", 1)
	r.Render(makeResult("mybrand.com", "com", dns.StatusTaken, ""))
	r.Finish(false)

	if stderr.Len() > 0 {
		t.Errorf("no stderr output expected on complete finish, got: %q", stderr.String())
	}
}

func TestCSVApplyVerificationNoDuplicates(t *testing.T) {
	var buf bytes.Buffer
	r := NewCSVRenderer(&buf, io.Discard)
	r.Start("test", 2)
	r.Render(dns.Result{Domain: "test.com", TLD: "com", Status: dns.StatusAvailable})
	r.Render(dns.Result{Domain: "test.org", TLD: "org", Status: dns.StatusTaken})
	r.ApplyVerification(
		[]dns.Result{{Domain: "test.com", TLD: "com", Status: dns.StatusTaken}},
		VerifyStats{Checked: 1, Verified: 1},
	)
	r.Finish(false)

	lines := strings.Split(strings.TrimSpace(buf.String()), "\n")
	// header + 2 data rows (test.com corrected to taken, test.org taken)
	if len(lines) != 3 {
		t.Fatalf("expected 3 lines, got %d: %v", len(lines), lines)
	}
	// test.com should appear exactly once, as taken
	comCount := 0
	for _, line := range lines {
		if strings.Contains(line, "test.com") {
			comCount++
			if !strings.Contains(line, "taken") {
				t.Errorf("test.com should be taken after correction, got: %s", line)
			}
		}
	}
	if comCount != 1 {
		t.Errorf("test.com should appear exactly once, appeared %d times", comCount)
	}
}
