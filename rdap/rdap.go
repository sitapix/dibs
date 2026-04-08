// Package rdap provides RDAP domain lookups and IANA bootstrap loading.
package rdap

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// Status represents the result of an RDAP domain lookup.
type Status int

const (
	Registered  Status = iota // 200: domain exists in registry
	NotFound                  // 404: domain not registered
	Unavailable               // no RDAP server for this TLD
	Error                     // network/server error
)

// Result holds the outcome of an RDAP lookup.
type Result struct {
	Domain string
	TLD    string
	Status Status
	Error  string
}

// Bootstrap maps lowercase TLDs to their RDAP base URLs.
type Bootstrap map[string]string

// bootstrapJSON matches the IANA RDAP bootstrap format.
type bootstrapJSON struct {
	Version  string       `json:"version"`
	Services [][][]string `json:"services"`
}

// ParseBootstrap parses IANA RDAP bootstrap JSON into a TLD→URL map.
func ParseBootstrap(data []byte) (Bootstrap, error) {
	var doc bootstrapJSON
	if err := json.Unmarshal(data, &doc); err != nil {
		return nil, fmt.Errorf("rdap: parse bootstrap: %w", err)
	}

	bs := make(Bootstrap)
	for _, svc := range doc.Services {
		if len(svc) < 2 || len(svc[1]) == 0 {
			continue
		}
		url := svc[1][0]
		for _, tld := range svc[0] {
			bs[strings.ToLower(tld)] = url
		}
	}
	return bs, nil
}

// Client performs RDAP domain lookups.
type Client struct {
	bootstrap Bootstrap
	http      *http.Client
}

// NewClient creates an RDAP client with the given bootstrap data and timeout.
func NewClient(bootstrap Bootstrap, timeout time.Duration) *Client {
	return &Client{
		bootstrap: bootstrap,
		http:      &http.Client{Timeout: timeout},
	}
}

// Lookup checks whether a domain is registered via RDAP.
func (c *Client) Lookup(ctx context.Context, domain, tld string) Result {
	base, ok := c.bootstrap[strings.ToLower(tld)]
	if !ok {
		return Result{Domain: domain, TLD: tld, Status: Unavailable}
	}

	// Ensure base URL ends with /
	if !strings.HasSuffix(base, "/") {
		base += "/"
	}

	url := base + "domain/" + domain
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return Result{Domain: domain, TLD: tld, Status: Error, Error: err.Error()}
	}
	req.Header.Set("Accept", "application/rdap+json")

	resp, err := c.http.Do(req)
	if err != nil {
		return Result{Domain: domain, TLD: tld, Status: Error, Error: err.Error()}
	}
	defer resp.Body.Close()
	_, _ = io.Copy(io.Discard, resp.Body)

	switch resp.StatusCode {
	case http.StatusOK:
		return Result{Domain: domain, TLD: tld, Status: Registered}
	case http.StatusNotFound:
		return Result{Domain: domain, TLD: tld, Status: NotFound}
	default:
		return Result{Domain: domain, TLD: tld, Status: Error,
			Error: fmt.Sprintf("RDAP HTTP %d", resp.StatusCode)}
	}
}

// WriteBootstrapCache writes bootstrap data to a cache file.
func WriteBootstrapCache(path string, data []byte) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	return os.WriteFile(path, data, 0o644)
}

// ReadBootstrapCache reads a cached bootstrap file.
// Returns (nil, false, nil) if the file does not exist.
func ReadBootstrapCache(path string, maxAge time.Duration) ([]byte, bool, error) {
	f, err := os.Open(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, false, nil
		}
		return nil, false, err
	}
	defer f.Close()

	info, err := f.Stat()
	if err != nil {
		return nil, false, err
	}

	data, err := io.ReadAll(f)
	if err != nil {
		return nil, false, err
	}

	fresh := maxAge > 0 && time.Since(info.ModTime()) < maxAge
	return data, fresh, nil
}
