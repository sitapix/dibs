package output

import "github.com/sitapix/dibs/dns"

// Renderer is the interface implemented by all output formats.
// Implementations must be safe for concurrent use: Render may be called
// from multiple goroutines simultaneously. Start is called once before
// any Render calls, and ApplyVerification and Finish are called
// sequentially after all Render calls complete.
type Renderer interface {
	// Start is called once before any results, with the search query and total TLD count.
	Start(query string, total int)
	// Render is called once per lookup result. Must be safe for concurrent use.
	Render(result dns.Result)
	// ApplyVerification applies RDAP verification corrections and stores stats.
	ApplyVerification(corrections []dns.Result, stats VerifyStats)
	// Finish is called after all results. partial is true when the run was interrupted.
	Finish(partial bool)
}
