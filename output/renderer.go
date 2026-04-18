package output

import "github.com/sitapix/dibs/dns"

// Renderer is the interface implemented by all output formats.
// Implementations must be safe for concurrent use: Render may be called
// from multiple goroutines simultaneously. Start is called once before
// any Render calls; BeginVerification, ApplyVerification, and Finish
// are called sequentially after all Render calls complete.
type Renderer interface {
	// Start is called once before any results, with the search query and total domain count.
	Start(query string, total int)
	// Render is called once per lookup result. Must be safe for concurrent use.
	Render(result dns.Result)
	// BeginVerification signals that RDAP verification is about to run on
	// `count` available domains. The terminal renderer uses this to clear
	// the in-place progress bar before the "Verifying..." banner prints,
	// so the two don't collide on the same line. No-op for buffered formats.
	BeginVerification(count int)
	// ApplyVerification applies RDAP verification corrections and stores stats.
	ApplyVerification(corrections []dns.Result, stats VerifyStats)
	// Finish is called after all results. partial is true when the run was interrupted.
	Finish(partial bool)
}
