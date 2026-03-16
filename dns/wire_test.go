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
	// After 12-byte header: [7]example[3]com[0] [0x00 0x01] [0x00 0x01]
	// Total = 12 + 1+7 + 1+3 + 1 + 2 + 2 = 29
	if len(q) != 29 {
		t.Fatalf("query length = %d, want 29", len(q))
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
	// 12 + 1+9 + 1 + 2 + 2 = 27
	if len(q) != 27 {
		t.Fatalf("query length = %d, want 27", len(q))
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
	long := strings.Repeat("a", 64) + ".com"
	_, err := buildQuery(long, qtypeA)
	if err == nil {
		t.Error("expected error for label exceeding 63 bytes")
	}
}

func TestBuildQuery_LabelAtLimit(t *testing.T) {
	label63 := strings.Repeat("a", 63) + ".com"
	_, err := buildQuery(label63, qtypeA)
	if err != nil {
		t.Errorf("63-byte label should be valid, got: %v", err)
	}
}
