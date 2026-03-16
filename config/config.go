// Package config provides configuration loading, merging, and validation for
// the dibs CLI tool. Configuration is stored as a simple KEY=VALUE text file.
package config

import (
	"bufio"
	"cmp"
	"errors"
	"fmt"
	"os"
	"strconv"
	"strings"
)

// Config holds all runtime configuration for dibs.
type Config struct {
	// Concurrency / networking
	Parallel    int
	MaxParallel int
	Timeout     int
	Retries     int
	Provider    string
	DohURL      string

	// Output / behaviour flags
	All     bool
	Limit   int
	Quiet   bool
	JSON    bool
	CSV     bool
	Rotate  bool
	NoDOH   bool
	NoColor bool
	Refresh bool
	Verify  bool

	// Input / filtering
	File      string
	TLDs      string
	MinLength int
	MaxLength int
	Sort      string
}

// Default returns a Config populated with sensible defaults.
func Default() Config {
	return Config{
		Parallel:    100,
		MaxParallel: 500,
		Timeout:     5,
		Retries:     1,
		Provider:    "quad9",
	}
}

// ParseFile reads a KEY=VALUE configuration file at path and returns the
// parsed Config. Lines beginning with '#' and blank lines are ignored.
// If the file does not exist, a zero-value Config is returned without error.
func ParseFile(path string) (Config, error) {
	f, err := os.Open(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return Config{}, nil
		}
		return Config{}, fmt.Errorf("config: open %q: %w", path, err)
	}
	defer f.Close()

	var cfg Config
	scanner := bufio.NewScanner(f)
	lineNum := 0
	for scanner.Scan() {
		lineNum++
		line := strings.TrimSpace(scanner.Text())

		// Skip blank lines and comments.
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}

		key, value, ok := strings.Cut(line, "=")
		if !ok {
			return Config{}, fmt.Errorf("config: line %d: expected KEY=VALUE, got %q", lineNum, line)
		}
		key = strings.TrimSpace(key)
		value = strings.TrimSpace(value)

		switch key {
		case "parallel", "timeout", "retries":
			n, err := strconv.Atoi(value)
			if err != nil {
				return Config{}, fmt.Errorf("config: line %d: %s must be an integer: %w", lineNum, key, err)
			}
			switch key {
			case "parallel":
				cfg.Parallel = n
			case "timeout":
				cfg.Timeout = n
			case "retries":
				cfg.Retries = n
			}
		case "provider":
			cfg.Provider = value
		default:
			return Config{}, fmt.Errorf("config: line %d: unknown key %q", lineNum, key)
		}
	}

	if err := scanner.Err(); err != nil {
		return Config{}, fmt.Errorf("config: read %q: %w", path, err)
	}

	return cfg, nil
}

// Merge combines base and file configs. For each field, the file value is used
// when it is non-zero/non-empty; otherwise the base value is kept.
func Merge(base, file Config) Config {
	return Config{
		Parallel:    cmp.Or(file.Parallel, base.Parallel),
		MaxParallel: cmp.Or(file.MaxParallel, base.MaxParallel),
		Timeout:     cmp.Or(file.Timeout, base.Timeout),
		Retries:     cmp.Or(file.Retries, base.Retries),
		Provider:    cmp.Or(file.Provider, base.Provider),
		DohURL:      cmp.Or(file.DohURL, base.DohURL),
		All:         file.All || base.All,
		Limit:       cmp.Or(file.Limit, base.Limit),
		Quiet:       file.Quiet || base.Quiet,
		JSON:        file.JSON || base.JSON,
		CSV:         file.CSV || base.CSV,
		Rotate:      file.Rotate || base.Rotate,
		NoDOH:       file.NoDOH || base.NoDOH,
		NoColor:     file.NoColor || base.NoColor,
		Refresh:     file.Refresh || base.Refresh,
		Verify:      file.Verify || base.Verify,
		File:        cmp.Or(file.File, base.File),
		TLDs:        cmp.Or(file.TLDs, base.TLDs),
		MinLength:   cmp.Or(file.MinLength, base.MinLength),
		MaxLength:   cmp.Or(file.MaxLength, base.MaxLength),
		Sort:        cmp.Or(file.Sort, base.Sort),
	}
}

// Validate checks that cfg contains values that are safe to use at runtime.
// validProviders is the set of recognized DNS provider names.
func Validate(cfg Config, validProviders map[string]bool) error {
	if cfg.Parallel < 1 || cfg.Parallel > cfg.MaxParallel {
		return fmt.Errorf("config: parallel must be between 1 and %d, got %d", cfg.MaxParallel, cfg.Parallel)
	}
	if cfg.Timeout < 0 {
		return fmt.Errorf("config: timeout must be non-negative, got %d", cfg.Timeout)
	}
	if !validProviders[cfg.Provider] {
		return fmt.Errorf("config: unknown provider %q", cfg.Provider)
	}
	return nil
}
