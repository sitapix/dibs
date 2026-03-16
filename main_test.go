package main

import (
	"io"
	"reflect"
	"strings"
	"testing"
)

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
		{strings.Repeat("a", 64), false},
		{"my brand", false},
		{"my.brand", false},
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
	code := run([]string{"--json", "--csv", "test"}, io.Discard, io.Discard, strings.NewReader(""))
	if code != 1 {
		t.Errorf("expected exit code 1, got %d", code)
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
