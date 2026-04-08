package config_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/sitapix/dibs/config"
)

// testProviders is the set of valid providers used across validation tests.
var testProviders = map[string]bool{
	"quad9": true, "mullvad": true, "nextdns": true, "adguard": true,
}

// ---------------------------------------------------------------------------
// helpers
// ---------------------------------------------------------------------------

func writeTempConfig(t *testing.T, content string) string {
	t.Helper()
	f, err := os.CreateTemp(t.TempDir(), "tld-config-*.conf")
	if err != nil {
		t.Fatalf("create temp file: %v", err)
	}
	if _, err := f.WriteString(content); err != nil {
		t.Fatalf("write temp file: %v", err)
	}
	f.Close()
	return f.Name()
}

// ---------------------------------------------------------------------------
// Default
// ---------------------------------------------------------------------------

func TestDefault(t *testing.T) {
	d := config.Default()

	if d.Parallel != 100 {
		t.Errorf("Parallel: got %d, want 100", d.Parallel)
	}
	if d.MaxParallel != 500 {
		t.Errorf("MaxParallel: got %d, want 500", d.MaxParallel)
	}
	if d.Timeout != 5 {
		t.Errorf("Timeout: got %d, want 5", d.Timeout)
	}
	if d.Retries != 1 {
		t.Errorf("Retries: got %d, want 1", d.Retries)
	}
	if d.Provider != "quad9" {
		t.Errorf("Provider: got %q, want \"quad9\"", d.Provider)
	}
}

// ---------------------------------------------------------------------------
// ParseFile — valid config
// ---------------------------------------------------------------------------

func TestParseFile_Valid(t *testing.T) {
	path := writeTempConfig(t, `parallel=10
timeout=3
retries=2
provider=nextdns
`)
	cfg, err := config.ParseFile(path)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.Parallel != 10 {
		t.Errorf("Parallel: got %d, want 10", cfg.Parallel)
	}
	if cfg.Timeout != 3 {
		t.Errorf("Timeout: got %d, want 3", cfg.Timeout)
	}
	if cfg.Retries != 2 {
		t.Errorf("Retries: got %d, want 2", cfg.Retries)
	}
	if cfg.Provider != "nextdns" {
		t.Errorf("Provider: got %q, want \"nextdns\"", cfg.Provider)
	}
}

// ---------------------------------------------------------------------------
// ParseFile — missing file (no error, zero-value Config)
// ---------------------------------------------------------------------------

func TestParseFile_MissingFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "does-not-exist.conf")
	cfg, err := config.ParseFile(path)
	if err != nil {
		t.Fatalf("expected no error for missing file, got: %v", err)
	}
	// zero-value: Parallel should be 0, Provider empty
	if cfg.Parallel != 0 {
		t.Errorf("Parallel: got %d, want 0", cfg.Parallel)
	}
	if cfg.Provider != "" {
		t.Errorf("Provider: got %q, want \"\"", cfg.Provider)
	}
}

// ---------------------------------------------------------------------------
// ParseFile — blank lines and comments are ignored
// ---------------------------------------------------------------------------

func TestParseFile_IgnoresBlankLinesAndComments(t *testing.T) {
	path := writeTempConfig(t, `# this is a comment
parallel=20

# another comment
timeout=7

retries=3
`)
	cfg, err := config.ParseFile(path)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.Parallel != 20 {
		t.Errorf("Parallel: got %d, want 20", cfg.Parallel)
	}
	if cfg.Timeout != 7 {
		t.Errorf("Timeout: got %d, want 7", cfg.Timeout)
	}
	if cfg.Retries != 3 {
		t.Errorf("Retries: got %d, want 3", cfg.Retries)
	}
}

// ---------------------------------------------------------------------------
// Merge
// ---------------------------------------------------------------------------

func TestMerge_FileOverridesBaseWhereNonZero(t *testing.T) {
	base := config.Default() // Parallel=100, Provider="quad9", Timeout=5
	file := config.Config{
		Parallel: 50,
		// Timeout left at zero — should not override base
		Provider: "nextdns",
	}

	merged := config.Merge(base, file)

	if merged.Parallel != 50 {
		t.Errorf("Parallel: got %d, want 50", merged.Parallel)
	}
	// Timeout was zero in file, so base value (5) should remain
	if merged.Timeout != 5 {
		t.Errorf("Timeout: got %d, want 5 (base preserved)", merged.Timeout)
	}
	if merged.Provider != "nextdns" {
		t.Errorf("Provider: got %q, want \"nextdns\"", merged.Provider)
	}
	// MaxParallel not set in file, base should be preserved
	if merged.MaxParallel != 500 {
		t.Errorf("MaxParallel: got %d, want 500", merged.MaxParallel)
	}
	// Retries not set in file, base should be preserved
	if merged.Retries != 1 {
		t.Errorf("Retries: got %d, want 1", merged.Retries)
	}
}

func TestMerge_VerifyFlag(t *testing.T) {
	base := config.Default()
	file := config.Config{Verify: true}
	merged := config.Merge(base, file)
	if !merged.Verify {
		t.Error("expected Verify=true after merge")
	}
}

func TestMerge_BaseRetainedWhenFileEmpty(t *testing.T) {
	base := config.Default()
	file := config.Config{} // all zero-values

	merged := config.Merge(base, file)

	if merged.Parallel != base.Parallel {
		t.Errorf("Parallel: got %d, want %d", merged.Parallel, base.Parallel)
	}
	if merged.Provider != base.Provider {
		t.Errorf("Provider: got %q, want %q", merged.Provider, base.Provider)
	}
}

// ---------------------------------------------------------------------------
// Validate
// ---------------------------------------------------------------------------

func TestValidate_DefaultConfigIsValid(t *testing.T) {
	if err := config.Validate(config.Default(), testProviders); err != nil {
		t.Errorf("Default config should be valid, got: %v", err)
	}
}

func TestValidate_ParallelZeroErrors(t *testing.T) {
	cfg := config.Default()
	cfg.Parallel = 0
	if err := config.Validate(cfg, testProviders); err == nil {
		t.Error("expected error for parallel=0, got nil")
	}
}

func TestValidate_ParallelTooHighErrors(t *testing.T) {
	cfg := config.Default()
	cfg.Parallel = 600
	if err := config.Validate(cfg, testProviders); err == nil {
		t.Error("expected error for parallel=600, got nil")
	}
}

func TestValidate_ParallelAtMax(t *testing.T) {
	cfg := config.Default()
	cfg.Parallel = 500
	if err := config.Validate(cfg, testProviders); err != nil {
		t.Errorf("parallel=500 (max) should be valid, got: %v", err)
	}
}

func TestValidate_ParallelAtMin(t *testing.T) {
	cfg := config.Default()
	cfg.Parallel = 1
	if err := config.Validate(cfg, testProviders); err != nil {
		t.Errorf("parallel=1 (min) should be valid, got: %v", err)
	}
}

func TestValidate_NegativeTimeoutErrors(t *testing.T) {
	cfg := config.Default()
	cfg.Timeout = -1
	if err := config.Validate(cfg, testProviders); err == nil {
		t.Error("expected error for timeout=-1, got nil")
	}
}

func TestValidate_ZeroTimeoutErrors(t *testing.T) {
	// Breaking change from older behavior: timeout=0 used to be accepted but
	// made http.Client hang forever. Now rejected at validation time.
	cfg := config.Default()
	cfg.Timeout = 0
	if err := config.Validate(cfg, testProviders); err == nil {
		t.Error("expected error for timeout=0 (hang-forever trap), got nil")
	}
}

func TestValidate_InvalidProviderErrors(t *testing.T) {
	cfg := config.Default()
	cfg.Provider = "invalidprovider"
	if err := config.Validate(cfg, testProviders); err == nil {
		t.Error("expected error for invalid provider, got nil")
	}
}

func TestParseFile_UnknownKeyErrors(t *testing.T) {
	path := writeTempConfig(t, "paralell=50\n")
	_, err := config.ParseFile(path)
	if err == nil {
		t.Error("expected error for unknown key, got nil")
	}
}

func TestValidate_Quad9ProviderValid(t *testing.T) {
	cfg := config.Default()
	cfg.Provider = "quad9"
	if err := config.Validate(cfg, testProviders); err != nil {
		t.Errorf("quad9 should be valid provider, got: %v", err)
	}
}

func TestValidate_MullvadProviderValid(t *testing.T) {
	cfg := config.Default()
	cfg.Provider = "mullvad"
	if err := config.Validate(cfg, testProviders); err != nil {
		t.Errorf("mullvad should be valid provider, got: %v", err)
	}
}

func TestValidate_NextDNSProviderValid(t *testing.T) {
	cfg := config.Default()
	cfg.Provider = "nextdns"
	if err := config.Validate(cfg, testProviders); err != nil {
		t.Errorf("nextdns should be valid provider, got: %v", err)
	}
}

func TestValidate_AdGuardProviderValid(t *testing.T) {
	cfg := config.Default()
	cfg.Provider = "adguard"
	if err := config.Validate(cfg, testProviders); err != nil {
		t.Errorf("adguard should be valid provider, got: %v", err)
	}
}

// ---------------------------------------------------------------------------
// Validate — numeric bounds added in the production-grade pass
// ---------------------------------------------------------------------------

func TestValidate_NegativeRetriesErrors(t *testing.T) {
	cfg := config.Default()
	cfg.Retries = -1
	if err := config.Validate(cfg, testProviders); err == nil {
		t.Error("expected error for retries=-1, got nil")
	}
}

func TestValidate_ZeroRetriesIsValid(t *testing.T) {
	cfg := config.Default()
	cfg.Retries = 0
	if err := config.Validate(cfg, testProviders); err != nil {
		t.Errorf("retries=0 should be valid (means no retries), got: %v", err)
	}
}

func TestValidate_NegativeLimitErrors(t *testing.T) {
	cfg := config.Default()
	cfg.Limit = -5
	if err := config.Validate(cfg, testProviders); err == nil {
		t.Error("expected error for limit=-5, got nil")
	}
}

func TestValidate_NegativeMinLengthErrors(t *testing.T) {
	cfg := config.Default()
	cfg.MinLength = -1
	if err := config.Validate(cfg, testProviders); err == nil {
		t.Error("expected error for min-length=-1, got nil")
	}
}

func TestValidate_NegativeMaxLengthErrors(t *testing.T) {
	cfg := config.Default()
	cfg.MaxLength = -1
	if err := config.Validate(cfg, testProviders); err == nil {
		t.Error("expected error for max-length=-1, got nil")
	}
}

func TestValidate_MinLengthGreaterThanMaxLengthErrors(t *testing.T) {
	cfg := config.Default()
	cfg.MinLength = 5
	cfg.MaxLength = 3
	if err := config.Validate(cfg, testProviders); err == nil {
		t.Error("expected error for min-length=5 > max-length=3, got nil")
	}
}

func TestValidate_MinLengthEqualsMaxLengthIsValid(t *testing.T) {
	cfg := config.Default()
	cfg.MinLength = 3
	cfg.MaxLength = 3
	if err := config.Validate(cfg, testProviders); err != nil {
		t.Errorf("min-length=max-length should be valid (exact length filter), got: %v", err)
	}
}

func TestValidate_OnlyMinLengthIsValid(t *testing.T) {
	cfg := config.Default()
	cfg.MinLength = 4
	// MaxLength left at 0 (no upper bound)
	if err := config.Validate(cfg, testProviders); err != nil {
		t.Errorf("min-length alone should be valid, got: %v", err)
	}
}

func TestValidate_OnlyMaxLengthIsValid(t *testing.T) {
	cfg := config.Default()
	cfg.MaxLength = 4
	// MinLength left at 0 (no lower bound)
	if err := config.Validate(cfg, testProviders); err != nil {
		t.Errorf("max-length alone should be valid, got: %v", err)
	}
}

func TestValidate_SortEmptyIsValid(t *testing.T) {
	cfg := config.Default()
	cfg.Sort = ""
	if err := config.Validate(cfg, testProviders); err != nil {
		t.Errorf("empty sort should be valid, got: %v", err)
	}
}

func TestValidate_SortAlphaIsValid(t *testing.T) {
	cfg := config.Default()
	cfg.Sort = "alpha"
	if err := config.Validate(cfg, testProviders); err != nil {
		t.Errorf("sort=alpha should be valid, got: %v", err)
	}
}

func TestValidate_SortLengthIsValid(t *testing.T) {
	cfg := config.Default()
	cfg.Sort = "length"
	if err := config.Validate(cfg, testProviders); err != nil {
		t.Errorf("sort=length should be valid, got: %v", err)
	}
}

func TestValidate_SortInvalidErrors(t *testing.T) {
	cfg := config.Default()
	cfg.Sort = "popularity"
	if err := config.Validate(cfg, testProviders); err == nil {
		t.Error("expected error for unrecognized sort value, got nil")
	}
}
