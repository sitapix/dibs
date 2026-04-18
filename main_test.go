package main

import (
	"context"
	"io"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/sitapix/dibs/dns"
	"github.com/sitapix/dibs/output"
)

// cancelOnFirstLookup is a dns.Resolver test double that cancels the supplied
// context the first time Lookup is called and always returns an error result.
// It drives the retry loop into its sleep-or-cancel select so we can assert
// that runWorkerPool bails out on ctx.Done() instead of sleeping through the
// full retry budget.
type cancelOnFirstLookup struct {
	cancel context.CancelFunc
}

func (r *cancelOnFirstLookup) Lookup(_ context.Context, domain string) dns.Result {
	r.cancel()
	return dns.Result{Domain: domain, Status: dns.StatusError, Error: "boom"}
}

// isolateConfig points XDG_CONFIG_HOME at a temp directory so a developer's
// real ~/.config/dibs/config can never affect tests that go through run().
func isolateConfig(t *testing.T) {
	t.Helper()
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
}

func TestValidateDomain(t *testing.T) {
	tests := []struct {
		input string
		valid bool
	}{
		{"mybrand", true},
		{"my-brand", true},
		{"a", true},
		{"", false},
		{"-mybrand", false},
		{"mybrand-", false},
		{strings.Repeat("a", dns.MaxLabelLength+1), false}, // one over the limit
		{"my brand", false},
		{"my.brand", false},
		{"café", false}, // non-ASCII rejected; use punycode
		{"例え", false},   // CJK rejected
	}
	for _, tt := range tests {
		err := validateDomain(tt.input)
		if tt.valid && err != nil {
			t.Errorf("validateDomain(%q) = %v, want nil", tt.input, err)
		}
		if !tt.valid && err == nil {
			t.Errorf("validateDomain(%q) = nil, want error", tt.input)
		}
	}
}

func TestReorderArgs(t *testing.T) {
	tests := []struct {
		name string
		in   []string
		want []string
	}{
		{"flags before positional", []string{"--json", "mybrand"}, []string{"--json", "mybrand"}},
		{"flags after positional", []string{"mybrand", "--json"}, []string{"--json", "mybrand"}},
		{"mixed with value flag", []string{"mybrand", "--limit", "5", "--json"}, []string{"--limit", "5", "--json", "mybrand"}},
		{"equals syntax", []string{"mybrand", "--provider=google"}, []string{"--provider=google", "mybrand"}},
		{"no flags", []string{"mybrand"}, []string{"mybrand"}},
		{"no args", nil, nil},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := reorderArgs(tt.in)
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("reorderArgs(%v) = %v, want %v", tt.in, got, tt.want)
			}
		})
	}
}

func TestReorderArgsDoubleDash(t *testing.T) {
	got := reorderArgs([]string{"--json", "--", "-weird"})
	want := []string{"--json", "--", "-weird"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("reorderArgs(--json -- -weird) = %v, want %v", got, want)
	}
}

func TestJSONAndCSVMutuallyExclusive(t *testing.T) {
	isolateConfig(t)
	code := run([]string{"--json", "--csv", "test"}, io.Discard, io.Discard, strings.NewReader(""))
	if code != 1 {
		t.Errorf("expected exit code 1, got %d", code)
	}
}

// TestRejectsExtraPositionalArgs pins the fix for silently ignoring trailing
// args. `dibs verify lumen` used to check "verify" and drop "lumen" on the
// floor; now it errors out and, because "verify" is a known flag, suggests
// the --verify spelling.
func TestRejectsExtraPositionalArgs(t *testing.T) {
	tests := []struct {
		name      string
		args      []string
		wantInErr []string
	}{
		{
			name:      "flag-name typo suggests --flag",
			args:      []string{"verify", "lumen"},
			wantInErr: []string{"did you mean", `--verify "lumen"`},
		},
		{
			name:      "plain extra name lists both inputs",
			args:      []string{"foo", "bar"},
			wantInErr: []string{"at most one name", `"foo" "bar"`},
		},
		{
			name:      "arg with whitespace is quoted so suggestion round-trips",
			args:      []string{"verify", "foo bar"},
			wantInErr: []string{"did you mean", `--verify "foo bar"`},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			isolateConfig(t)
			var stderr strings.Builder
			code := run(tt.args, io.Discard, &stderr, strings.NewReader(""))
			if code != 1 {
				t.Fatalf("exit code = %d, want 1", code)
			}
			for _, needle := range tt.wantInErr {
				if !strings.Contains(stderr.String(), needle) {
					t.Errorf("stderr missing %q; got:\n%s", needle, stderr.String())
				}
			}
		})
	}
}

func TestBuildDomainList(t *testing.T) {
	tlds := []string{"com", "org", "net"}
	domains := buildDomainList("mybrand", tlds)
	if len(domains) != 3 {
		t.Fatalf("got %d domains, want 3", len(domains))
	}
	if domains[0] != "mybrand.com" {
		t.Errorf("domains[0] = %q, want mybrand.com", domains[0])
	}
}

func TestParseFullDomain(t *testing.T) {
	tests := []struct {
		name      string
		input     string
		wantLabel string
		wantTLD   string
		wantErr   bool
	}{
		{"single-label tld", "vi.be", "vi", "be", false},
		{"multi-label tld", "foo.co.uk", "foo", "co.uk", false},
		{"dot-com", "google.com", "google", "com", false},
		{"uppercase normalized", "VI.BE", "vi", "be", false},
		{"whitespace trimmed", "  vi.be  ", "vi", "be", false},
		{"trailing fqdn dot", "vi.be.", "vi", "be", false},

		// Subdomain rejection: PSL detects the registrable domain.
		{"subdomain rejected", "mail.google.com", "", "", true},
		{"deep subdomain rejected", "a.b.c.d.co.uk", "", "", true},

		// Non-ICANN TLD rejection: would otherwise report misleading results.
		{"fake tld rejected", "this.tld", "", "", true},
		{"fake tld with extra labels", "test.this.tld", "", "", true},
		{"unknown tld rejected", "foo.weirdtld", "", "", true},
		{"psl private suffix rejected", "foo.github.io", "", "", true},

		// Malformed inputs.
		{"leading dot", ".be", "", "", true},
		{"trailing dot only", "vi.", "", "", true},
		{"double dot", "..be", "", "", true},
		{"empty", "", "", "", true},
		{"whitespace only", "   ", "", "", true},

		// Non-ASCII rejected (IDN deferred).
		{"unicode rejected", "café.fr", "", "", true},
		{"cjk rejected", "例え.jp", "", "", true},

		// Label validation failures post-split.
		{"label with space", "my brand.com", "", "", true},
		{"label starts with hyphen", "-foo.com", "", "", true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			label, tld, err := parseFullDomain(tt.input)
			if tt.wantErr {
				if err == nil {
					t.Errorf("parseFullDomain(%q) = (%q, %q, nil), want error", tt.input, label, tld)
				}
				return
			}
			if err != nil {
				t.Errorf("parseFullDomain(%q) = error %v, want (%q, %q, nil)", tt.input, err, tt.wantLabel, tt.wantTLD)
				return
			}
			if label != tt.wantLabel || tld != tt.wantTLD {
				t.Errorf("parseFullDomain(%q) = (%q, %q), want (%q, %q)", tt.input, label, tld, tt.wantLabel, tt.wantTLD)
			}
		})
	}
}

func TestSingleDomainConflicts(t *testing.T) {
	isolateConfig(t)
	// Each of these should error out before touching the network.
	cases := [][]string{
		{"--all", "vi.be"},
		{"--tlds", "com", "vi.be"},
		{"--limit", "5", "vi.be"},
		{"--sort", "alpha", "vi.be"},
		{"--min-length", "2", "vi.be"},
		{"--max-length", "5", "vi.be"},
	}
	for _, args := range cases {
		t.Run(strings.Join(args, " "), func(t *testing.T) {
			code := run(args, io.Discard, io.Discard, strings.NewReader(""))
			if code != 1 {
				t.Errorf("run(%v) = %d, want 1 (conflict error)", args, code)
			}
		})
	}
}

func TestSingleDomainRejectsSubdomains(t *testing.T) {
	isolateConfig(t)
	// "mail.google.com" has extra labels; should error instead of checking.
	code := run([]string{"mail.google.com"}, io.Discard, io.Discard, strings.NewReader(""))
	if code != 1 {
		t.Errorf("run(mail.google.com) = %d, want 1", code)
	}
}

// TestRunWorkerPoolRespectsContextCancellation verifies that once the context
// is cancelled, the retry loop exits its sleep immediately instead of waiting
// out retryDelay × remaining attempts. Without the select-on-ctx.Done() fix
// this test would take ~5s (10 retries × 500ms) instead of milliseconds.
func TestRunWorkerPoolRespectsContextCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	res := &cancelOnFirstLookup{cancel: cancel}
	renderer := output.NewJSONRenderer(io.Discard, io.Discard)

	const retries = 10
	budget := retryDelay / 2 // well under one full retryDelay sleep

	start := time.Now()
	_, partial := runWorkerPool(ctx, res, []string{"foo.com"}, 1, retries, renderer)
	elapsed := time.Since(start)

	if !partial {
		t.Error("expected partial=true for cancelled context")
	}
	if elapsed > budget {
		t.Errorf("runWorkerPool took %v on cancelled ctx; expected < %v", elapsed, budget)
	}
}

func TestSingleDomainConflictsReportsAll(t *testing.T) {
	isolateConfig(t)
	// Multiple conflicting flags should be reported in a single error so the
	// user can fix them all in one pass.
	var stderr strings.Builder
	code := run([]string{"--all", "--limit", "5", "--sort", "alpha", "vi.be"}, io.Discard, &stderr, strings.NewReader(""))
	if code != 1 {
		t.Errorf("expected exit code 1, got %d", code)
	}
	msg := stderr.String()
	for _, want := range []string{"--all", "--limit", "--sort"} {
		if !strings.Contains(msg, want) {
			t.Errorf("error message should mention %q, got: %s", want, msg)
		}
	}
}
