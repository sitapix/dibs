package dns

import (
	"strings"
	"testing"
)

func TestBuildQuery_Header(t *testing.T) {
	q, err := buildQuery("example.com", qtypeA)
	if err != nil {
		t.Fatalf("buildQuery: %v", err)
	}
	if len(q) < 12 {
		t.Fatalf("query too short: %d bytes", len(q))
	}
	// ID must be 0 (RFC 8484 §4.1)
	if q[0] != 0 || q[1] != 0 {
		t.Errorf("ID = 0x%02x%02x, want 0x0000", q[0], q[1])
	}
	// RD flag must be set (byte 2 = 0x01, byte 3 = 0x00)
	if q[2] != 0x01 || q[3] != 0x00 {
		t.Errorf("flags = 0x%02x%02x, want 0x0100 (RD=1)", q[2], q[3])
	}
	// QDCOUNT = 1
	if q[4] != 0 || q[5] != 1 {
		t.Errorf("QDCOUNT = %d, want 1", int(q[4])<<8|int(q[5]))
	}
}

func TestBuildQuery_QuestionSection(t *testing.T) {
	q, err := buildQuery("example.com", qtypeA)
	if err != nil {
		t.Fatalf("buildQuery: %v", err)
	}
	// Post RFC 8467 padding, the total wire length is padded to 128 bytes.
	// The question section itself still starts at byte 12 with layout:
	// [7]example[3]com[0] [0x00 0x01] [0x00 0x01] (bytes 12..28).
	if len(q) != 128 {
		t.Fatalf("query length = %d, want 128 (padded)", len(q))
	}
	// QNAME starts at byte 12
	if q[12] != 7 {
		t.Errorf("first label length = %d, want 7", q[12])
	}
	if string(q[13:20]) != "example" {
		t.Errorf("first label = %q, want \"example\"", string(q[13:20]))
	}
	if q[20] != 3 {
		t.Errorf("second label length = %d, want 3", q[20])
	}
	if string(q[21:24]) != "com" {
		t.Errorf("second label = %q, want \"com\"", string(q[21:24]))
	}
	if q[24] != 0 {
		t.Errorf("root label terminator = %d, want 0", q[24])
	}
	// QTYPE = A (1)
	if q[25] != 0 || q[26] != 1 {
		t.Errorf("QTYPE = %d, want 1", int(q[25])<<8|int(q[26]))
	}
	// QCLASS = IN (1)
	if q[27] != 0 || q[28] != 1 {
		t.Errorf("QCLASS = %d, want 1", int(q[27])<<8|int(q[28]))
	}
}

func TestBuildQuery_SingleLabel(t *testing.T) {
	q, err := buildQuery("localhost", qtypeA)
	if err != nil {
		t.Fatalf("buildQuery: %v", err)
	}
	// Padded to 128-byte boundary (RFC 8467).
	if len(q) != 128 {
		t.Fatalf("query length = %d, want 128 (padded)", len(q))
	}
}

func TestExtractRcode_NXDOMAIN(t *testing.T) {
	resp := []byte{
		0x00, 0x00, // ID
		0x81,       // QR=1, RD=1
		0x83,       // RA=1, RCODE=3
		0x00, 0x01, // QDCOUNT
		0x00, 0x00, // ANCOUNT
		0x00, 0x00, // NSCOUNT
		0x00, 0x00, // ARCOUNT
	}
	if rc := extractRcode(resp); rc != 3 {
		t.Errorf("rcode = %d, want 3", rc)
	}
}

func TestExtractRcode_NOERROR(t *testing.T) {
	resp := []byte{
		0x00, 0x00,
		0x81, 0x80, // RA=1, RCODE=0
		0x00, 0x01,
		0x00, 0x01,
		0x00, 0x00,
		0x00, 0x00,
	}
	if rc := extractRcode(resp); rc != 0 {
		t.Errorf("rcode = %d, want 0", rc)
	}
}

func TestExtractRcode_TooShort(t *testing.T) {
	if rc := extractRcode([]byte{0x00, 0x00}); rc != -1 {
		t.Errorf("rcode = %d, want -1 for short response", rc)
	}
}

func TestExtractRcode_SERVFAIL(t *testing.T) {
	resp := []byte{
		0x00, 0x00,
		0x81, 0x82, // RA=1, RCODE=2
		0x00, 0x01,
		0x00, 0x00,
		0x00, 0x00,
		0x00, 0x00,
	}
	if rc := extractRcode(resp); rc != 2 {
		t.Errorf("rcode = %d, want 2", rc)
	}
}

func TestBuildQuery_LabelTooLong(t *testing.T) {
	long := strings.Repeat("a", MaxLabelLength+1) + ".com"
	_, err := buildQuery(long, qtypeA)
	if err == nil {
		t.Errorf("expected error for label exceeding %d bytes", MaxLabelLength)
	}
}

func TestBuildQuery_LabelAtLimit(t *testing.T) {
	labelAtMax := strings.Repeat("a", MaxLabelLength) + ".com"
	_, err := buildQuery(labelAtMax, qtypeA)
	if err != nil {
		t.Errorf("%d-byte label should be valid, got: %v", MaxLabelLength, err)
	}
}

// TestBuildQuery_PaddingAlignment verifies that buildQuery pads every query
// to a 128-byte boundary regardless of input length (RFC 8467).
func TestBuildQuery_PaddingAlignment(t *testing.T) {
	cases := []struct {
		name    string
		domain  string
		wantLen int
	}{
		{"short", "a.b", 128},
		{"typical", "example.com", 128},
		{"long label", "localhost", 128},
		// 3 max-length labels → 3×(1+63) + 1 root = 193 bytes of name.
		// Header 12 + name 193 + qtype 2 + qclass 2 + OPT 15 = 224 → pads to 256.
		{"multi-max-label",
			strings.Repeat("a", MaxLabelLength) + "." + strings.Repeat("b", MaxLabelLength) + "." + strings.Repeat("c", MaxLabelLength),
			256},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			q, err := buildQuery(c.domain, qtypeA)
			if err != nil {
				t.Fatalf("buildQuery(%q): %v", c.domain, err)
			}
			if len(q) != c.wantLen {
				t.Errorf("len = %d, want %d", len(q), c.wantLen)
			}
			if len(q)%128 != 0 {
				t.Errorf("len = %d is not a multiple of 128", len(q))
			}
		})
	}
}

// TestBuildQuery_OPTRR verifies the OPT pseudo-RR (RFC 6891) is well-formed:
// ARCOUNT=1, NAME=root, TYPE=41 (OPT), CLASS=4096 (UDP payload size),
// TTL=0 (no extended RCODE, version 0, Z flags clear).
func TestBuildQuery_OPTRR(t *testing.T) {
	q, err := buildQuery("example.com", qtypeA)
	if err != nil {
		t.Fatalf("buildQuery: %v", err)
	}
	// ARCOUNT is at offset 10..11 in the header.
	if arcount := int(q[10])<<8 | int(q[11]); arcount != 1 {
		t.Errorf("ARCOUNT = %d, want 1", arcount)
	}
	// OPT RR begins right after the question section. For "example.com":
	// header(12) + name(13) + qtype(2) + qclass(2) = 29.
	const optOffset = 29
	if q[optOffset] != 0x00 {
		t.Errorf("OPT NAME = 0x%02x, want 0x00 (root)", q[optOffset])
	}
	if typ := int(q[optOffset+1])<<8 | int(q[optOffset+2]); typ != 41 {
		t.Errorf("OPT TYPE = %d, want 41", typ)
	}
	if class := int(q[optOffset+3])<<8 | int(q[optOffset+4]); class != 4096 {
		t.Errorf("OPT CLASS (UDP payload size) = %d, want 4096", class)
	}
	// TTL at optOffset+5..8 should be all zero (no extended RCODE, version 0, no flags).
	for i := 5; i <= 8; i++ {
		if q[optOffset+i] != 0 {
			t.Errorf("OPT TTL byte %d = 0x%02x, want 0x00", i-5, q[optOffset+i])
		}
	}
}

// TestBuildQuery_PaddingOption verifies the OPT RDATA contains a PADDING
// option (RFC 7830) with option code 12 and data consisting entirely of
// zero bytes, and that the whole thing fills the query to 128 bytes.
func TestBuildQuery_PaddingOption(t *testing.T) {
	q, err := buildQuery("example.com", qtypeA)
	if err != nil {
		t.Fatalf("buildQuery: %v", err)
	}
	// Layout: [header 12][question 17][OPT RR 11 + RDATA]
	const rdlenOffset = 29 + 9 // start of OPT + NAME(1)+TYPE(2)+CLASS(2)+TTL(4) = 9
	rdlen := int(q[rdlenOffset])<<8 | int(q[rdlenOffset+1])
	rdataOffset := rdlenOffset + 2

	if rdataOffset+rdlen != len(q) {
		t.Fatalf("OPT RDATA spans [%d..%d), query is %d bytes", rdataOffset, rdataOffset+rdlen, len(q))
	}

	// First 4 bytes of RDATA: PADDING option header.
	optCode := int(q[rdataOffset])<<8 | int(q[rdataOffset+1])
	if optCode != 12 {
		t.Errorf("EDNS option code = %d, want 12 (PADDING)", optCode)
	}
	optLen := int(q[rdataOffset+2])<<8 | int(q[rdataOffset+3])
	if optLen != rdlen-4 {
		t.Errorf("PADDING option length = %d, want %d (RDLEN - 4 option header)", optLen, rdlen-4)
	}

	// All padding bytes must be zero.
	for i := rdataOffset + 4; i < len(q); i++ {
		if q[i] != 0 {
			t.Errorf("padding byte at offset %d = 0x%02x, want 0x00", i, q[i])
			break
		}
	}

	// And the total must be 128.
	if len(q) != 128 {
		t.Errorf("total length = %d, want 128", len(q))
	}
}
