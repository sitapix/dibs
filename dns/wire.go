package dns

import (
	"fmt"
	"strings"
)

// DNS protocol constants (RFC 1035 §4.1.1).
const (
	qtypeA        uint16 = 1 // A record
	rcodeNOERROR  int    = 0
	rcodeNXDOMAIN int    = 3
)

// buildQuery constructs a DNS wire-format query (RFC 1035 §4.1).
// ID is set to 0 per RFC 8484 §4.1. RD (recursion desired) is set.
// Returns an error if any label exceeds the 63-byte DNS limit (RFC 1035 §2.3.4).
func buildQuery(domain string, qtype uint16) ([]byte, error) {
	buf := []byte{
		0x00, 0x00, // ID (0 for DoH)
		0x01, 0x00, // Flags: RD=1
		0x00, 0x01, // QDCOUNT: 1
		0x00, 0x00, // ANCOUNT
		0x00, 0x00, // NSCOUNT
		0x00, 0x00, // ARCOUNT
	}
	for _, label := range strings.Split(domain, ".") {
		if len(label) > 63 {
			return nil, fmt.Errorf("dns: label %q exceeds 63-byte limit", label)
		}
		buf = append(buf, byte(len(label)))
		buf = append(buf, label...)
	}
	buf = append(buf, 0x00)                             // root label
	buf = append(buf, byte(qtype>>8), byte(qtype&0xFF)) // QTYPE
	buf = append(buf, 0x00, 0x01)                       // QCLASS: IN
	return buf, nil
}

// extractRcode reads the RCODE from a DNS wire-format response header.
// Returns -1 if the response is shorter than the 12-byte header.
func extractRcode(resp []byte) int {
	if len(resp) < 12 {
		return -1
	}
	return int(resp[3] & 0x0F)
}
