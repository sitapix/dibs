package dns

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"strings"
	"sync/atomic"
	"time"
)

// DomainStatus represents the availability status of a domain.
type DomainStatus int

const (
	StatusAvailable DomainStatus = iota
	StatusTaken
	StatusError
)

// String returns the human-readable name of the status.
func (s DomainStatus) String() string {
	switch s {
	case StatusAvailable:
		return "available"
	case StatusTaken:
		return "taken"
	default:
		return "error"
	}
}

// Result holds the outcome of a domain lookup.
type Result struct {
	Domain string
	TLD    string
	Status DomainStatus
	Error  string
}

// Provider represents a DNS-over-HTTPS provider.
type Provider struct {
	Name string
	URL  string
}

// providers is the set of built-in DoH providers.
var providers = map[string]Provider{
	"quad9":      {Name: "quad9", URL: "https://dns.quad9.net/dns-query"},
	"mullvad":    {Name: "mullvad", URL: "https://dns.mullvad.net/dns-query"},
	"cloudflare": {Name: "cloudflare", URL: "https://cloudflare-dns.com/dns-query"},
	"google":     {Name: "google", URL: "https://dns.google/dns-query"},
}

// GetProvider returns a provider by name and whether it exists.
func GetProvider(name string) (Provider, bool) {
	p, ok := providers[name]
	return p, ok
}

// ProviderNames returns the names of all built-in providers.
func ProviderNames() []string {
	names := make([]string, 0, len(providers))
	for name := range providers {
		names = append(names, name)
	}
	return names
}

// AllProviders returns all built-in providers in a deterministic order.
func AllProviders() []Provider {
	return []Provider{
		providers["quad9"],
		providers["mullvad"],
		providers["cloudflare"],
		providers["google"],
	}
}

// Resolver is the interface implemented by all resolver types.
type Resolver interface {
	Lookup(ctx context.Context, domain string) Result
}

// DoHResolver queries DNS via DNS-over-HTTPS (RFC 8484 wire format).
type DoHResolver struct {
	providers []Provider
	rotate    bool
	counter   atomic.Uint64
	client    *http.Client
}

// NewDoHResolver constructs a DoHResolver.
// When rotate is true, successive calls cycle through providers in round-robin order.
func NewDoHResolver(providers []Provider, rotate bool, timeout time.Duration) *DoHResolver {
	return &DoHResolver{
		providers: providers,
		rotate:    rotate,
		client: &http.Client{
			Timeout: timeout,
		},
	}
}

// Lookup performs a DoH A-record lookup for domain using RFC 8484 wire format.
func (r *DoHResolver) Lookup(ctx context.Context, domain string) Result {
	provider := r.pickProvider()

	query, err := buildQuery(domain, qtypeA)
	if err != nil {
		return errorResult(domain, err)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, provider.URL, bytes.NewReader(query))
	if err != nil {
		return errorResult(domain, err)
	}
	req.Header.Set("Content-Type", "application/dns-message")
	req.Header.Set("Accept", "application/dns-message")

	resp, err := r.client.Do(req)
	if err != nil {
		return errorResult(domain, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		_, _ = io.Copy(io.Discard, resp.Body)
		return errorResult(domain, fmt.Errorf("DoH HTTP %d", resp.StatusCode))
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return errorResult(domain, err)
	}

	rcode := extractRcode(body)
	return interpretRcode(domain, rcode)
}

// pickProvider returns the provider to use for the next lookup.
func (r *DoHResolver) pickProvider() Provider {
	if !r.rotate || len(r.providers) == 1 {
		return r.providers[0]
	}
	idx := r.counter.Add(1) - 1
	return r.providers[idx%uint64(len(r.providers))]
}

// interpretRcode converts a DNS RCODE into a Result.
func interpretRcode(domain string, rcode int) Result {
	tld := extractTLD(domain)
	switch rcode {
	case rcodeNXDOMAIN:
		return Result{Domain: domain, TLD: tld, Status: StatusAvailable}
	case rcodeNOERROR:
		return Result{Domain: domain, TLD: tld, Status: StatusTaken}
	default:
		return Result{
			Domain: domain,
			TLD:    tld,
			Status: StatusError,
			Error:  fmt.Sprintf("DNS error (RCODE %d)", rcode),
		}
	}
}

// SystemResolver uses the operating system's DNS resolver.
type SystemResolver struct {
	resolver *net.Resolver
	timeout  time.Duration
}

// NewSystemResolver constructs a SystemResolver.
func NewSystemResolver(timeout time.Duration) *SystemResolver {
	return &SystemResolver{
		resolver: net.DefaultResolver,
		timeout:  timeout,
	}
}

// Lookup performs a system DNS lookup for domain.
func (r *SystemResolver) Lookup(ctx context.Context, domain string) Result {
	tld := extractTLD(domain)

	ctx, cancel := context.WithTimeout(ctx, r.timeout)
	defer cancel()

	_, err := r.resolver.LookupHost(ctx, domain)
	if err != nil {
		var dnsErr *net.DNSError
		if errors.As(err, &dnsErr) && dnsErr.IsNotFound {
			return Result{Domain: domain, TLD: tld, Status: StatusAvailable}
		}
		return errorResult(domain, err)
	}

	return Result{Domain: domain, TLD: tld, Status: StatusTaken}
}

// errorResult builds a Result with StatusError for the given domain and error.
func errorResult(domain string, err error) Result {
	return Result{Domain: domain, TLD: extractTLD(domain), Status: StatusError, Error: err.Error()}
}

// extractTLD returns the last label of a domain name.
func extractTLD(domain string) string {
	if i := strings.LastIndex(domain, "."); i >= 0 {
		return domain[i+1:]
	}
	return domain
}
