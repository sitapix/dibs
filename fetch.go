package main

import (
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/sitapix/dibs/rdap"
	"github.com/sitapix/dibs/tlds"
)

const (
	ianaTLDURL       = "https://data.iana.org/TLD/tlds-alpha-by-domain.txt"
	cacheMaxAge      = 24 * time.Hour
	rdapBootstrapURL = "https://data.iana.org/rdap/dns.json"
)

// fetchAllTLDs fetches the full IANA TLD list, using caching.
func fetchAllTLDs(forceRefresh bool, stderr io.Writer) []string {
	cachePath := cacheFilePath()

	// Try cache first (unless forcing refresh).
	if !forceRefresh {
		content, fresh, err := tlds.ReadCache(cachePath, cacheMaxAge)
		if err == nil && fresh && content != "" {
			list := tlds.Parse(content)
			if len(list) > 0 {
				return list
			}
		}
	}

	// Fetch from IANA.
	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Get(ianaTLDURL)
	if err != nil {
		fmt.Fprintf(stderr, "Warning: could not fetch TLD list: %v\n", err)
		return fallbackTLDs(cachePath, stderr)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		fmt.Fprintf(stderr, "Warning: could not read TLD list response: %v\n", err)
		return fallbackTLDs(cachePath, stderr)
	}

	raw := string(body)
	list := tlds.Parse(raw)
	if len(list) == 0 {
		fmt.Fprintf(stderr, "Warning: fetched TLD list is empty\n")
		return fallbackTLDs(cachePath, stderr)
	}

	// Write to cache (ignore error).
	if err := tlds.WriteCache(cachePath, raw); err != nil {
		fmt.Fprintf(stderr, "Warning: could not write TLD cache: %v\n", err)
	}

	return list
}

// fallbackTLDs tries the stale cache, then falls back to Top25.
func fallbackTLDs(cachePath string, stderr io.Writer) []string {
	content, _, err := tlds.ReadCache(cachePath, 0)
	if err == nil && content != "" {
		list := tlds.Parse(content)
		if len(list) > 0 {
			fmt.Fprintf(stderr, "Using cached TLD list\n")
			return list
		}
	}
	fmt.Fprintf(stderr, "Falling back to Top 25 TLDs\n")
	return tlds.Top25()
}

func fetchRDAPBootstrap(forceRefresh bool, stderr io.Writer) rdap.Bootstrap {
	cachePath := rdapCacheFilePath()

	if !forceRefresh {
		data, fresh, err := rdap.ReadBootstrapCache(cachePath, cacheMaxAge)
		if err == nil && fresh && len(data) > 0 {
			bs, err := rdap.ParseBootstrap(data)
			if err == nil && len(bs) > 0 {
				return bs
			}
		}
	}

	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Get(rdapBootstrapURL)
	if err != nil {
		fmt.Fprintf(stderr, "Warning: could not fetch RDAP bootstrap: %v\n", err)
		return fallbackRDAPBootstrap(cachePath)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		fmt.Fprintf(stderr, "Warning: could not read RDAP bootstrap: %v\n", err)
		return fallbackRDAPBootstrap(cachePath)
	}

	bs, err := rdap.ParseBootstrap(body)
	if err != nil || len(bs) == 0 {
		fmt.Fprintf(stderr, "Warning: invalid RDAP bootstrap data\n")
		return fallbackRDAPBootstrap(cachePath)
	}

	if err := rdap.WriteBootstrapCache(cachePath, body); err != nil {
		fmt.Fprintf(stderr, "Warning: could not cache RDAP bootstrap: %v\n", err)
	}

	return bs
}

func fallbackRDAPBootstrap(cachePath string) rdap.Bootstrap {
	data, _, err := rdap.ReadBootstrapCache(cachePath, 0)
	if err == nil && len(data) > 0 {
		bs, err := rdap.ParseBootstrap(data)
		if err == nil {
			return bs
		}
	}
	return nil
}

func rdapCacheFilePath() string {
	return xdgPath("XDG_CACHE_HOME", ".cache", "rdap-bootstrap.json")
}
