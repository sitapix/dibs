package tlds

import "golang.org/x/net/publicsuffix"

// Suffix returns the public suffix of a domain (e.g. "co.uk" for "foo.co.uk")
// from the Public Suffix List embedded via x/net/publicsuffix.
//
// The icann return is true only when the suffix comes from the ICANN section
// of the PSL. It is false for unknown TLDs (PSL's default rule treats the
// last label as the suffix) and for PSL private suffixes like "github.io"
// (public for cookie scoping, not registrable via a registrar).
//
// Callers that validate user input MUST check icann. Callers that only need
// the suffix for display or RDAP routing can discard it.
func Suffix(domain string) (suffix string, icann bool) {
	return publicsuffix.PublicSuffix(domain)
}

// RegistrableDomain returns the registrable portion of a domain (eTLD+1).
// For "foo.co.uk" it returns "foo.co.uk"; for "mail.google.com", "google.com".
// Returns an error for inputs with no registrable form (empty labels,
// suffix-only inputs).
func RegistrableDomain(domain string) (string, error) {
	return publicsuffix.EffectiveTLDPlusOne(domain)
}
