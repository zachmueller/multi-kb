package submit

import (
	"strings"
	"testing"
)

func TestEncodeCrockford_vectors(t *testing.T) {
	tests := []struct {
		name  string
		input []byte
		want  string
	}{
		{"all zeros", make([]byte, 10), "0000000000000000"},
		{"all 0xFF", bytes10(0xff), "ZZZZZZZZZZZZZZZZ"},
		{"0x00..0x09", []byte{0x00, 0x01, 0x02, 0x03, 0x04, 0x05, 0x06, 0x07, 0x08, 0x09}, "000G40R40M30E209"},
		{"DEADBEEF", []byte{0xde, 0xad, 0xbe, 0xef, 0xca, 0xfe, 0xba, 0xbe, 0x00, 0x42}, "VTPVXVYAZTXBW022"},
		{"HelloWorld", []byte("HelloWorld"), "91JPRV3FAXQQ4V34"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := EncodeCrockford(tt.input)
			if got != tt.want {
				t.Errorf("EncodeCrockford(%x) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

func TestGenerateUID(t *testing.T) {
	const validAlphabet = "0123456789ABCDEFGHJKMNPQRSTVWXYZ"
	const forbidden = "ILOU"

	for i := 0; i < 1000; i++ {
		uid, err := GenerateUID()
		if err != nil {
			t.Fatalf("GenerateUID() error: %v", err)
		}
		if len(uid) != 16 {
			t.Fatalf("GenerateUID() length %d, want 16", len(uid))
		}
		for _, ch := range uid {
			if !strings.ContainsRune(validAlphabet, ch) {
				t.Fatalf("GenerateUID() invalid char %q in %q", ch, uid)
			}
			if strings.ContainsRune(forbidden, ch) {
				t.Fatalf("GenerateUID() forbidden char %q in %q", ch, uid)
			}
		}
	}
}

func TestGenerateUID_uniqueness(t *testing.T) {
	seen := make(map[string]bool, 10000)
	for i := 0; i < 10000; i++ {
		uid, _ := GenerateUID()
		if seen[uid] {
			t.Fatalf("collision after %d UIDs: %q", i, uid)
		}
		seen[uid] = true
	}
}

func bytes10(b byte) []byte {
	out := make([]byte, 10)
	for i := range out {
		out[i] = b
	}
	return out
}
