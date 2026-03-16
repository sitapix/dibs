// Package tlds provides utilities for fetching, caching, parsing, and
// filtering IANA TLD lists.
package tlds

import (
	"bufio"
	"cmp"
	"errors"
	"io"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"time"
)

// top25 is a hardcoded list of 25 popular TLDs ordered by general popularity.
var top25 = []string{
	"com", "org", "net", "io", "co",
	"dev", "app", "ai", "xyz", "me",
	"tech", "ly", "gg", "tv", "fm",
	"sh", "cc", "us", "info", "pro",
	"cloud", "live", "space", "store", "online",
}

// Top25 returns a copy of the 25 most popular TLDs.
func Top25() []string {
	return slices.Clone(top25)
}

// Parse parses an IANA TLD list in its standard text format.
// Lines starting with '#' are treated as comments and skipped.
// Blank lines are skipped. All entries are lowercased.
func Parse(raw string) []string {
	var result []string
	scanner := bufio.NewScanner(strings.NewReader(raw))
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		result = append(result, strings.ToLower(line))
	}
	return result
}

// ParseCustom splits a comma-separated list of TLDs, trims whitespace from
// each entry, lowercases it, and discards empty tokens.
func ParseCustom(csv string) []string {
	parts := strings.Split(csv, ",")
	var result []string
	for _, p := range parts {
		p = strings.TrimSpace(p)
		p = strings.ToLower(p)
		if p != "" {
			result = append(result, p)
		}
	}
	return result
}

// FilterByLength returns the subset of tlds whose character length is within
// [minLen, maxLen]. A value of 0 for minLen or maxLen means no constraint on
// that bound.
func FilterByLength(tlds []string, minLen, maxLen int) []string {
	var result []string
	for _, t := range tlds {
		l := len([]rune(t))
		if minLen > 0 && l < minLen {
			continue
		}
		if maxLen > 0 && l > maxLen {
			continue
		}
		result = append(result, t)
	}
	return result
}

// Limit returns the first n TLDs from the slice. If n <= 0 or n >= len(tlds),
// all entries are returned.
func Limit(tlds []string, n int) []string {
	if n <= 0 || n >= len(tlds) {
		return slices.Clone(tlds)
	}
	return slices.Clone(tlds[:n])
}

// Sort returns a sorted copy of tlds according to mode:
//   - "alpha"  — alphabetical order
//   - "length" — ascending character length (stable, preserves original order
//     for ties)
//   - anything else — original order preserved
func Sort(tlds []string, mode string) []string {
	result := slices.Clone(tlds)

	switch mode {
	case "alpha":
		slices.Sort(result)
	case "length":
		slices.SortStableFunc(result, func(a, b string) int {
			return cmp.Compare(len([]rune(a)), len([]rune(b)))
		})
	}
	// Any other mode: return the copy with original order preserved.
	return result
}

// WriteCache writes data to the file at path, creating any missing parent
// directories as needed.
func WriteCache(path, data string) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	return os.WriteFile(path, []byte(data), 0o644)
}

// ReadCache reads a cache file at path and reports whether it is fresh.
// maxAge is the maximum acceptable file age; 0 means always stale.
// If the file does not exist, ("", false, nil) is returned.
func ReadCache(path string, maxAge time.Duration) (content string, fresh bool, err error) {
	f, err := os.Open(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return "", false, nil
		}
		return "", false, err
	}
	defer f.Close()

	info, err := f.Stat()
	if err != nil {
		return "", false, err
	}

	data, err := io.ReadAll(f)
	if err != nil {
		return "", false, err
	}

	isFresh := maxAge > 0 && time.Since(info.ModTime()) < maxAge
	return string(data), isFresh, nil
}
