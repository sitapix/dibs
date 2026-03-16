package output

import (
	"encoding/csv"
	"fmt"
	"io"
	"sync"

	"github.com/sitapix/dibs/dns"
)

// CSVRenderer buffers results and writes all CSV rows on Finish.
type CSVRenderer struct {
	w       io.Writer
	stderr  io.Writer
	mu      sync.Mutex
	results []dns.Result
}

// NewCSVRenderer creates a CSVRenderer that writes CSV to w and warnings to stderr.
func NewCSVRenderer(w, stderr io.Writer) *CSVRenderer {
	return &CSVRenderer{
		w:      w,
		stderr: stderr,
	}
}

// Start is a no-op for CSV; header is written on Finish.
func (r *CSVRenderer) Start(_ string, _ int) {}

// Render buffers a single lookup result.
func (r *CSVRenderer) Render(result dns.Result) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.results = append(r.results, result)
}

// ApplyVerification corrects buffered results from available to taken.
func (r *CSVRenderer) ApplyVerification(corrections []dns.Result, _ VerifyStats) {
	r.mu.Lock()
	defer r.mu.Unlock()

	corrected := make(map[string]bool, len(corrections))
	for _, c := range corrections {
		corrected[c.Domain] = true
	}

	for i := range r.results {
		if corrected[r.results[i].Domain] {
			r.results[i].Status = dns.StatusTaken
		}
	}
}

// Finish writes the CSV header and all buffered rows, then flushes.
func (r *CSVRenderer) Finish(partial bool) {
	r.mu.Lock()
	defer r.mu.Unlock()

	cw := csv.NewWriter(r.w)
	_ = cw.Write([]string{"domain", "tld", "status"})
	for _, result := range r.results {
		_ = cw.Write([]string{result.Domain, result.TLD, result.Status.String()})
	}
	cw.Flush()

	if err := cw.Error(); err != nil {
		fmt.Fprintf(r.stderr, "warning: error writing CSV: %v\n", err)
	}

	if partial {
		_, _ = io.WriteString(r.stderr, "warning: interrupted, results are incomplete\n")
	}
}
