package submit

import (
	"crypto/rand"
	"fmt"
)

const crockfordAlphabet = "0123456789ABCDEFGHJKMNPQRSTVWXYZ"

// EncodeCrockford encodes a byte slice to Crockford base32.
// Uses bit-buffer algorithm: accumulates 8 bits per byte, extracts 5-bit groups MSB-first.
func EncodeCrockford(data []byte) string {
	var bits, value int
	var output []byte

	for _, b := range data {
		value = (value << 8) | int(b)
		bits += 8
		for bits >= 5 {
			bits -= 5
			output = append(output, crockfordAlphabet[(value>>bits)&0x1f])
		}
	}
	if bits > 0 {
		output = append(output, crockfordAlphabet[(value<<(5-bits))&0x1f])
	}
	return string(output)
}

// GenerateUID returns a new 16-character Crockford base32 UID using 10 random bytes.
func GenerateUID() (string, error) {
	buf := make([]byte, 10)
	if _, err := rand.Read(buf); err != nil {
		return "", fmt.Errorf("uid: cannot read random bytes: %w", err)
	}
	return EncodeCrockford(buf), nil
}
