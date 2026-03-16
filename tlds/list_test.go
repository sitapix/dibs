package tlds_test

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/sitapix/dibs/tlds"
)

// ---------------------------------------------------------------------------
// Top25
// ---------------------------------------------------------------------------

func TestTop25_ReturnsExactly25(t *testing.T) {
	got := tlds.Top25()
	if len(got) != 25 {
		t.Errorf("Top25: got %d entries, want 25", len(got))
	}
}

func TestTop25_FirstIscom(t *testing.T) {
	got := tlds.Top25()
	if len(got) == 0 {
		t.Fatal("Top25 returned empty slice")
	}
	if got[0] != "com" {
		t.Errorf("Top25[0]: got %q, want \"com\"", got[0])
	}
}

func TestTop25_ReturnsCopy(t *testing.T) {
	a := tlds.Top25()
	b := tlds.Top25()
	a[0] = "MUTATED"
	if b[0] == "MUTATED" {
		t.Error("Top25 returned reference to internal slice; mutations should not affect subsequent calls")
	}
}

// ---------------------------------------------------------------------------
// Parse
// ---------------------------------------------------------------------------

func TestParse_IANAFormat(t *testing.T) {
	// Simulate real IANA TLD list: header comment, uppercase TLDs,
	// internationalized (punycode) TLD.
	raw := `# Version 2023010100, Last Updated Mon Jan  1 00:07:01 2023 UTC
COM
NET
ORG
XN--FIQS8S
`
	got := tlds.Parse(raw)

	want := []string{"com", "net", "org", "xn--fiqs8s"}
	if len(got) != len(want) {
		t.Fatalf("Parse: got %d entries, want %d: %v", len(got), len(want), got)
	}
	for i, w := range want {
		if got[i] != w {
			t.Errorf("Parse[%d]: got %q, want %q", i, got[i], w)
		}
	}
}

func TestParse_SkipsBlankLines(t *testing.T) {
	raw := `# comment
COM

NET

ORG
`
	got := tlds.Parse(raw)
	if len(got) != 3 {
		t.Errorf("Parse: got %d entries, want 3: %v", len(got), got)
	}
}

func TestParse_EmptyInput(t *testing.T) {
	got := tlds.Parse("")
	if len(got) != 0 {
		t.Errorf("Parse(\"\") should return empty slice, got %v", got)
	}
}

func TestParse_LowercasesEntries(t *testing.T) {
	raw := "COM\nIO\n"
	got := tlds.Parse(raw)
	for _, g := range got {
		for _, r := range g {
			if r >= 'A' && r <= 'Z' {
				t.Errorf("Parse: entry %q is not fully lowercase", g)
			}
		}
	}
}

// ---------------------------------------------------------------------------
// ParseCustom
// ---------------------------------------------------------------------------

func TestParseCustom_Basic(t *testing.T) {
	got := tlds.ParseCustom("com,org,net")
	want := []string{"com", "org", "net"}
	if len(got) != len(want) {
		t.Fatalf("ParseCustom: got %d entries, want %d: %v", len(got), len(want), got)
	}
	for i, w := range want {
		if got[i] != w {
			t.Errorf("ParseCustom[%d]: got %q, want %q", i, got[i], w)
		}
	}
}

func TestParseCustom_TrimsSpaces(t *testing.T) {
	got := tlds.ParseCustom(" com , org , net ")
	want := []string{"com", "org", "net"}
	if len(got) != len(want) {
		t.Fatalf("ParseCustom: got %d entries, want %d: %v", len(got), len(want), got)
	}
	for i, w := range want {
		if got[i] != w {
			t.Errorf("ParseCustom[%d]: got %q, want %q", i, got[i], w)
		}
	}
}

func TestParseCustom_LowercasesEntries(t *testing.T) {
	got := tlds.ParseCustom("COM,ORG,IO")
	for _, g := range got {
		for _, r := range g {
			if r >= 'A' && r <= 'Z' {
				t.Errorf("ParseCustom: entry %q is not fully lowercase", g)
			}
		}
	}
}

func TestParseCustom_EmptyInput(t *testing.T) {
	got := tlds.ParseCustom("")
	// empty string split by comma gives one empty token — it should be filtered
	if len(got) != 0 {
		t.Errorf("ParseCustom(\"\") should return empty slice, got %v", got)
	}
}

func TestParseCustom_SingleEntry(t *testing.T) {
	got := tlds.ParseCustom("io")
	if len(got) != 1 || got[0] != "io" {
		t.Errorf("ParseCustom(\"io\") = %v, want [\"io\"]", got)
	}
}

// ---------------------------------------------------------------------------
// FilterByLength
// ---------------------------------------------------------------------------

func TestFilterByLength_MinAndMax(t *testing.T) {
	input := []string{"io", "com", "tech", "store"}
	// min=3, max=4 → com(3), tech(4)
	got := tlds.FilterByLength(input, 3, 4)
	want := []string{"com", "tech"}
	if len(got) != len(want) {
		t.Fatalf("FilterByLength(3,4): got %v, want %v", got, want)
	}
	for i, w := range want {
		if got[i] != w {
			t.Errorf("FilterByLength[%d]: got %q, want %q", i, got[i], w)
		}
	}
}

func TestFilterByLength_MinOnly(t *testing.T) {
	input := []string{"io", "com", "tech"}
	// min=3, max=0 (no upper bound) → com(3), tech(4)
	got := tlds.FilterByLength(input, 3, 0)
	want := []string{"com", "tech"}
	if len(got) != len(want) {
		t.Fatalf("FilterByLength(3,0): got %v, want %v", got, want)
	}
	for i, w := range want {
		if got[i] != w {
			t.Errorf("[%d]: got %q, want %q", i, got[i], w)
		}
	}
}

func TestFilterByLength_MaxOnly(t *testing.T) {
	input := []string{"io", "com", "tech"}
	// min=0, max=2 (no lower bound) → io(2)
	got := tlds.FilterByLength(input, 0, 2)
	want := []string{"io"}
	if len(got) != len(want) {
		t.Fatalf("FilterByLength(0,2): got %v, want %v", got, want)
	}
	if got[0] != "io" {
		t.Errorf("got %q, want \"io\"", got[0])
	}
}

func TestFilterByLength_NoConstraint(t *testing.T) {
	input := []string{"io", "com", "tech"}
	got := tlds.FilterByLength(input, 0, 0)
	if len(got) != len(input) {
		t.Errorf("FilterByLength(0,0): got %v, want all %v", got, input)
	}
}

func TestFilterByLength_NilInput(t *testing.T) {
	got := tlds.FilterByLength(nil, 2, 5)
	if len(got) != 0 {
		t.Errorf("FilterByLength(nil,...): expected empty, got %v", got)
	}
}

// ---------------------------------------------------------------------------
// Limit
// ---------------------------------------------------------------------------

func TestLimit_ReturnsFirstN(t *testing.T) {
	input := []string{"com", "org", "net", "io", "co"}
	got := tlds.Limit(input, 3)
	want := []string{"com", "org", "net"}
	if len(got) != len(want) {
		t.Fatalf("Limit(3): got %v, want %v", got, want)
	}
	for i, w := range want {
		if got[i] != w {
			t.Errorf("[%d]: got %q, want %q", i, got[i], w)
		}
	}
}

func TestLimit_NZeroReturnsAll(t *testing.T) {
	input := []string{"com", "org", "net"}
	got := tlds.Limit(input, 0)
	if len(got) != len(input) {
		t.Errorf("Limit(0): got %v, want all %v", got, input)
	}
}

func TestLimit_NNegativeReturnsAll(t *testing.T) {
	input := []string{"com", "org", "net"}
	got := tlds.Limit(input, -5)
	if len(got) != len(input) {
		t.Errorf("Limit(-5): got %v, want all %v", got, input)
	}
}

func TestLimit_NGreaterThanLenReturnsAll(t *testing.T) {
	input := []string{"com", "org", "net"}
	got := tlds.Limit(input, 100)
	if len(got) != len(input) {
		t.Errorf("Limit(100) on 3-element slice: got %v, want all %v", got, input)
	}
}

func TestLimit_NEqualsLenReturnsAll(t *testing.T) {
	input := []string{"com", "org", "net"}
	got := tlds.Limit(input, 3)
	if len(got) != 3 {
		t.Errorf("Limit(3) on 3-element slice: got %v, want all", got)
	}
}

// ---------------------------------------------------------------------------
// Sort
// ---------------------------------------------------------------------------

func TestSort_Alpha(t *testing.T) {
	input := []string{"org", "com", "io", "app"}
	got := tlds.Sort(input, "alpha")
	want := []string{"app", "com", "io", "org"}
	if len(got) != len(want) {
		t.Fatalf("Sort(alpha): got %v, want %v", got, want)
	}
	for i, w := range want {
		if got[i] != w {
			t.Errorf("[%d]: got %q, want %q", i, got[i], w)
		}
	}
}

func TestSort_AlphaDoesNotMutateInput(t *testing.T) {
	input := []string{"org", "com", "io"}
	_ = tlds.Sort(input, "alpha")
	if input[0] != "org" {
		t.Error("Sort mutated the input slice")
	}
}

func TestSort_Length(t *testing.T) {
	input := []string{"tech", "io", "com", "online"}
	got := tlds.Sort(input, "length")
	// Expected order by ascending length: io(2), com(3), tech(4), online(6)
	// Stable sort: ties preserve original order.
	if len(got) != 4 {
		t.Fatalf("Sort(length): got %v", got)
	}
	if got[0] != "io" {
		t.Errorf("Sort(length)[0]: got %q, want \"io\"", got[0])
	}
	if got[1] != "com" {
		t.Errorf("Sort(length)[1]: got %q, want \"com\"", got[1])
	}
	if got[2] != "tech" {
		t.Errorf("Sort(length)[2]: got %q, want \"tech\"", got[2])
	}
	if got[3] != "online" {
		t.Errorf("Sort(length)[3]: got %q, want \"online\"", got[3])
	}
}

func TestSort_LengthStable(t *testing.T) {
	// Two 3-letter TLDs: "com" then "org" — stable sort must preserve order.
	input := []string{"com", "org", "io"}
	got := tlds.Sort(input, "length")
	// io(2), com(3), org(3)
	if len(got) < 3 {
		t.Fatalf("Sort(length): got %v", got)
	}
	if got[1] != "com" || got[2] != "org" {
		t.Errorf("Sort(length) not stable for equal-length entries: got %v", got)
	}
}

func TestSort_NonePreservesOrder(t *testing.T) {
	input := []string{"org", "com", "io"}
	got := tlds.Sort(input, "none")
	for i, w := range input {
		if got[i] != w {
			t.Errorf("Sort(none)[%d]: got %q, want %q (order should be preserved)", i, got[i], w)
		}
	}
}

func TestSort_UnknownModePreservesOrder(t *testing.T) {
	input := []string{"org", "com", "io"}
	got := tlds.Sort(input, "random-unknown-mode")
	for i, w := range input {
		if got[i] != w {
			t.Errorf("Sort(unknown)[%d]: got %q, want %q (order should be preserved)", i, got[i], w)
		}
	}
}

// ---------------------------------------------------------------------------
// WriteCache / ReadCache
// ---------------------------------------------------------------------------

func TestCache_WriteThenReadFresh(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "subdir", "tlds.cache")
	data := "COM\nNET\nORG\n"

	if err := tlds.WriteCache(path, data); err != nil {
		t.Fatalf("WriteCache: %v", err)
	}

	content, fresh, err := tlds.ReadCache(path, time.Hour)
	if err != nil {
		t.Fatalf("ReadCache: %v", err)
	}
	if !fresh {
		t.Error("ReadCache: expected fresh=true for newly written cache")
	}
	if content != data {
		t.Errorf("ReadCache: got %q, want %q", content, data)
	}
}

func TestCache_Expired_MaxAgeZero(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "tlds.cache")
	data := "COM\nNET\n"

	if err := tlds.WriteCache(path, data); err != nil {
		t.Fatalf("WriteCache: %v", err)
	}

	// maxAge=0 means always expired
	content, fresh, err := tlds.ReadCache(path, 0)
	if err != nil {
		t.Fatalf("ReadCache: %v", err)
	}
	if fresh {
		t.Error("ReadCache: expected fresh=false when maxAge=0")
	}
	// Content should still be returned even when stale
	if content != data {
		t.Errorf("ReadCache: content mismatch: got %q, want %q", content, data)
	}
}

func TestCache_MissingFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "nonexistent.cache")

	content, fresh, err := tlds.ReadCache(path, time.Hour)
	if err != nil {
		t.Fatalf("ReadCache on missing file: expected no error, got %v", err)
	}
	if fresh {
		t.Error("ReadCache on missing file: expected fresh=false")
	}
	if content != "" {
		t.Errorf("ReadCache on missing file: expected empty content, got %q", content)
	}
}

func TestCache_CreatesParentDirectories(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "a", "b", "c", "tlds.cache")

	if err := tlds.WriteCache(path, "test"); err != nil {
		t.Fatalf("WriteCache with nested dirs: %v", err)
	}

	if _, err := os.Stat(path); err != nil {
		t.Errorf("file not created at %s: %v", path, err)
	}
}

func TestCache_ExpiredByAge(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "tlds.cache")

	if err := tlds.WriteCache(path, "data"); err != nil {
		t.Fatalf("WriteCache: %v", err)
	}

	// Back-date the file modification time by 2 hours.
	pastTime := time.Now().Add(-2 * time.Hour)
	if err := os.Chtimes(path, pastTime, pastTime); err != nil {
		t.Fatalf("Chtimes: %v", err)
	}

	// maxAge=3600 (1 hour) — file is 2 hours old, should be stale.
	_, fresh, err := tlds.ReadCache(path, time.Hour)
	if err != nil {
		t.Fatalf("ReadCache: %v", err)
	}
	if fresh {
		t.Error("ReadCache: expected fresh=false for file older than maxAge")
	}
}
