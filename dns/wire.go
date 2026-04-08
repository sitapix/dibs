package dns

import (
	"fmt"
	"strings"
)

// DNS protocol constants (RFC 1035 §4.1.1, RFC 6891, RFC 7830, RFC 8467).
const (
	// MaxLabelLength is the maximum length of a single DNS label in octets
	// per RFC 1035 §2.3.4. Exported so user-input validators in other
	// packages can share this single source of truth.
	MaxLabelLength = 63

	qtypeA        uint16 = 1 // A record
	rcodeNOERROR  int    = 0
	rcodeNXDOMAIN int    = 3

	// EDNS(0) OPT pseudo-RR + PADDING option (RFC 6891, RFC 7830, RFC 8467).
	ednsUDPPayloadSize  = 4096 // requester's UDP payload size, signaled in OPT CLASS
	ednsOptionPadding   = 12   // PADDING option code (RFC 7830)
	paddingBlockSize    = 128  // pad total wire length to this multiple (RFC 8467)
	optRRFixedLen       = 11   // NAME(1) + TYPE(2) + CLASS(2) + TTL(4) + RDLEN(2)
	paddingOptHeaderLen = 4    // option code(2) + option length(2)
)

// buildQuery constructs a DNS wire-format query (RFC 1035 §4.1) with an
// EDNS(0) OPT pseudo-RR containing a PADDING option (RFC 6891, RFC 7830).
// The total wire length is padded to the nearest 128-byte multiple per
// RFC 8467 so passive TLS observers can't distinguish short-name queries
// from long-name queries by ciphertext length.
//
// ID is set to 0 per RFC 8484 §4.1. RD (recursion desired) is set.
// Returns an error if any label exceeds the 63-byte DNS limit (RFC 1035 §2.3.4).
func buildQuery(domain string, qtype uint16) ([]byte, error) {
	buf := []byte{
		0x00, 0x00, // ID (0 for DoH)
		0x01, 0x00, // Flags: RD=1
		0x00, 0x01, // QDCOUNT: 1
		0x00, 0x00, // ANCOUNT
		0x00, 0x00, // NSCOUNT
		0x00, 0x01, // ARCOUNT: 1 (for the OPT pseudo-RR below)
	}
	for _, label := range strings.Split(domain, ".") {
		if len(label) > MaxLabelLength {
			return nil, fmt.Errorf("dns: label %q exceeds %d-byte limit", label, MaxLabelLength)
		}
		buf = append(buf, byte(len(label)))
		buf = append(buf, label...)
	}
	buf = append(buf, 0x00)                             // root label
	buf = append(buf, byte(qtype>>8), byte(qtype&0xFF)) // QTYPE
	buf = append(buf, 0x00, 0x01)                       // QCLASS: IN

	// Compute padding so that (header + question + OPT + padding) lands on
	// a 128-byte boundary. Padding data length is (target - current) where
	// current already accounts for the OPT RR and padding option headers.
	current := len(buf) + optRRFixedLen + paddingOptHeaderLen
	target := ((current + paddingBlockSize - 1) / paddingBlockSize) * paddingBlockSize
	paddingDataLen := target - current
	rdlen := paddingOptHeaderLen + paddingDataLen

	// OPT pseudo-RR header (RFC 6891 §6.1.2).
	buf = append(buf, 0x00) // NAME: root domain
	buf = append(buf,
		0x00, 0x29, // TYPE: OPT (41)
		byte(ednsUDPPayloadSize>>8), byte(ednsUDPPayloadSize&0xFF), // CLASS: UDP payload size
		0x00, 0x00, 0x00, 0x00, // TTL: extended RCODE + version + Z flags (all zero)
		byte(rdlen>>8), byte(rdlen&0xFF), // RDLENGTH
	)
	// OPT RDATA: the PADDING option (RFC 7830).
	buf = append(buf,
		byte(ednsOptionPadding>>8), byte(ednsOptionPadding&0xFF), // Option code
		byte(paddingDataLen>>8), byte(paddingDataLen&0xFF), // Option length
	)
	buf = append(buf, make([]byte, paddingDataLen)...) // zero-filled padding
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
