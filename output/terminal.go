package output

import (
	"fmt"
	"io"
	"os"
	"strings"
	"sync"

	"github.com/sitapix/dibs/dns"
)

const (
	colorReset = "\x1b[0m"
	colorGreen = "\x1b[32m"
	colorDim   = "\x1b[2m"
	clearLine  = "\r\x1b[2K"
	barWidth   = 16
	barFilled  = '━'
	barEmpty   = '─'
)

// VerifyStats holds RDAP verification statistics.
type VerifyStats struct {
	Checked    int // domains sent to RDAP
	Verified   int // confirmed available or corrected
	Unverified int // no RDAP server for TLD
}

// TerminalRenderer renders results to a terminal with optional ANSI colors and
// an in-place progress bar when writing to a real TTY.
type TerminalRenderer struct {
	w           io.Writer
	quiet       bool
	noColor     bool
	isTTY       bool
	mu          sync.Mutex
	total       int
	checked     int
	available   int
	verifyStats *VerifyStats
}

// NewTerminalRenderer creates a TerminalRenderer. isTTY is auto-detected from w.
func NewTerminalRenderer(w io.Writer, quiet, noColor bool) *TerminalRenderer {
	return &TerminalRenderer{
		w:       w,
		quiet:   quiet,
		noColor: noColor,
		isTTY:   IsWriterTTY(w),
	}
}

// IsWriterTTY returns true when w is an *os.File that refers to a character device.
func IsWriterTTY(w io.Writer) bool {
	f, ok := w.(*os.File)
	if !ok {
		return false
	}
	info, err := f.Stat()
	if err != nil {
		return false
	}
	return (info.Mode() & os.ModeCharDevice) != 0
}

// Start records the query and total and, on a TTY, draws the initial progress bar.
func (r *TerminalRenderer) Start(query string, total int) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.total = total
	if r.isTTY {
		fmt.Fprint(r.w, r.progressBar())
	}
}

// Render outputs one result line.
func (r *TerminalRenderer) Render(result dns.Result) {
	r.mu.Lock()
	defer r.mu.Unlock()

	r.checked++
	if result.Status == dns.StatusAvailable {
		r.available++
	}

	// In quiet mode, skip taken domains.
	if r.quiet && result.Status == dns.StatusTaken {
		if r.isTTY {
			// Redraw progress bar without printing a result line.
			fmt.Fprint(r.w, clearLine+r.progressBar())
		}
		return
	}

	line := r.formatResult(result)

	if r.isTTY {
		// Clear the current progress bar line, print result, redraw bar.
		fmt.Fprint(r.w, clearLine+line+"\n"+r.progressBar())
	} else {
		fmt.Fprintln(r.w, line)
	}
}

// Finish clears the progress bar and prints the summary.
func (r *TerminalRenderer) Finish(partial bool) {
	r.mu.Lock()
	defer r.mu.Unlock()

	if r.isTTY {
		fmt.Fprint(r.w, clearLine)
	}

	if partial {
		fmt.Fprintf(r.w, "Interrupted: showing %d/%d results\n", r.checked, r.total)
		return
	}

	pct := 0
	if r.checked > 0 {
		pct = r.available * 100 / r.checked
	}
	fmt.Fprintf(r.w, "Found %d available domains out of %d checked (%d%%)\n",
		r.available, r.checked, pct)

	if r.verifyStats != nil {
		vs := r.verifyStats
		fmt.Fprintf(r.w, "Verified %d of %d via RDAP", vs.Verified, vs.Checked)
		if vs.Unverified > 0 {
			fmt.Fprintf(r.w, " (%d domains without RDAP coverage)", vs.Unverified)
		}
		fmt.Fprintln(r.w)
	}
}

// ApplyVerification prints RDAP corrections and stores stats for Finish.
func (r *TerminalRenderer) ApplyVerification(corrections []dns.Result, stats VerifyStats) {
	r.mu.Lock()
	defer r.mu.Unlock()

	r.verifyStats = &stats

	for _, c := range corrections {
		r.available--
		line := r.formatCorrectedResult(c)
		if r.isTTY {
			fmt.Fprint(r.w, clearLine+line+"\n")
		} else {
			fmt.Fprintln(r.w, line)
		}
	}
}

func (r *TerminalRenderer) formatCorrectedResult(result dns.Result) string {
	colorOn, colorOff := colorDim, colorReset
	if r.noColor {
		colorOn, colorOff = "", ""
	}
	return fmt.Sprintf("%s✗  %-30s  taken (registered, no DNS)%s", colorOn, result.Domain, colorOff)
}

// formatResult returns a single formatted result line, with ANSI colors if enabled.
func (r *TerminalRenderer) formatResult(result dns.Result) string {
	var symbol, label, colorOn string
	switch result.Status {
	case dns.StatusAvailable:
		symbol, label, colorOn = "✓", "available", colorGreen
	case dns.StatusTaken:
		symbol, label, colorOn = "✗", "taken", colorDim
	default:
		errStr := result.Error
		if errStr == "" {
			errStr = "unknown error"
		}
		symbol, label, colorOn = "?", "error: "+errStr, colorDim
	}

	colorOff := colorReset
	if r.noColor {
		colorOn, colorOff = "", ""
	}

	return fmt.Sprintf("%s%s  %-30s  %s%s", colorOn, symbol, result.Domain, label, colorOff)
}

// progressBar returns the progress bar string for the current state.
func (r *TerminalRenderer) progressBar() string {
	pct := 0
	if r.total > 0 {
		pct = r.checked * 100 / r.total
	}

	filled := 0
	if r.total > 0 {
		filled = min(barWidth*r.checked/r.total, barWidth)
	}

	bar := strings.Repeat(string(barFilled), filled) +
		strings.Repeat(string(barEmpty), barWidth-filled)

	return fmt.Sprintf("  %s %d/%d domains  %d%%", bar, r.checked, r.total, pct)
}
