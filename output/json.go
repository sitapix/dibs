package output

import (
	"encoding/json"
	"fmt"
	"io"
	"sync"

	"github.com/sitapix/dibs/dns"
)

// domainEntry is a single entry in the available/taken arrays.
type domainEntry struct {
	Domain string `json:"domain"`
	TLD    string `json:"tld"`
}

// errorEntry is a single entry in the errors array.
type errorEntry struct {
	Domain string `json:"domain"`
	TLD    string `json:"tld"`
	Error  string `json:"error"`
}

// verifyInfo holds RDAP verification metadata written to the JSON output.
type verifyInfo struct {
	Verified   int `json:"verified"`
	Unverified int `json:"unverified"`
	Corrected  int `json:"corrected"`
}

// jsonDocument is the top-level JSON object written on Finish.
type jsonDocument struct {
	Query     string        `json:"query"`
	Checked   int           `json:"checked"`
	Partial   bool          `json:"partial"`
	Available []domainEntry `json:"available"`
	Taken     []domainEntry `json:"taken"`
	Errors    []errorEntry  `json:"errors"`
	Verify    *verifyInfo   `json:"verify,omitempty"`
}

// JSONRenderer collects results and writes a single JSON document on Finish.
type JSONRenderer struct {
	w      io.Writer
	stderr io.Writer
	mu     sync.Mutex
	doc    jsonDocument
}

// NewJSONRenderer creates a JSONRenderer that writes JSON to w and warnings to stderr.
func NewJSONRenderer(w, stderr io.Writer) *JSONRenderer {
	return &JSONRenderer{
		w:      w,
		stderr: stderr,
		doc: jsonDocument{
			// Initialise slices so they serialise as [] rather than null.
			Available: []domainEntry{},
			Taken:     []domainEntry{},
			Errors:    []errorEntry{},
		},
	}
}

// Start records the query string.
func (r *JSONRenderer) Start(query string, _ int) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.doc.Query = query
}

// Render accumulates a single lookup result.
func (r *JSONRenderer) Render(result dns.Result) {
	r.mu.Lock()
	defer r.mu.Unlock()

	r.doc.Checked++
	switch result.Status {
	case dns.StatusAvailable:
		r.doc.Available = append(r.doc.Available, domainEntry{Domain: result.Domain, TLD: result.TLD})
	case dns.StatusTaken:
		r.doc.Taken = append(r.doc.Taken, domainEntry{Domain: result.Domain, TLD: result.TLD})
	default:
		r.doc.Errors = append(r.doc.Errors, errorEntry{Domain: result.Domain, TLD: result.TLD, Error: result.Error})
	}
}

// BeginVerification: JSON is buffered; any "verifying..." banner would only
// appear in the error stream and is skipped for machine-parseable output.
func (r *JSONRenderer) BeginVerification(_ int) {}

// ApplyVerification moves corrected domains from Available to Taken and stores stats.
func (r *JSONRenderer) ApplyVerification(corrections []dns.Result, stats VerifyStats) {
	r.mu.Lock()
	defer r.mu.Unlock()

	corrected := make(map[string]bool, len(corrections))
	for _, c := range corrections {
		corrected[c.Domain] = true
	}

	var remaining []domainEntry
	for _, d := range r.doc.Available {
		if corrected[d.Domain] {
			r.doc.Taken = append(r.doc.Taken, d)
		} else {
			remaining = append(remaining, d)
		}
	}
	if remaining == nil {
		remaining = []domainEntry{}
	}
	r.doc.Available = remaining

	r.doc.Verify = &verifyInfo{
		Verified:   stats.Verified,
		Unverified: stats.Unverified,
		Corrected:  len(corrections),
	}
}

// Finish writes the collected results as a pretty-printed JSON document.
func (r *JSONRenderer) Finish(partial bool) {
	r.mu.Lock()
	defer r.mu.Unlock()

	r.doc.Partial = partial

	enc := json.NewEncoder(r.w)
	enc.SetIndent("", "  ")
	if err := enc.Encode(r.doc); err != nil {
		fmt.Fprintf(r.stderr, "warning: error writing JSON output: %v\n", err)
	}
}
