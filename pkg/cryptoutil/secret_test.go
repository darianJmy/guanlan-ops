package cryptoutil

import (
	"encoding/base64"
	"testing"
)

func TestEncryptionPreservesInputAndAuthenticates(t *testing.T) {
	plain := "  中文 secret\n"
	a, err := Encrypt(plain, "key-one")
	if err != nil {
		t.Fatal(err)
	}
	b, err := Encrypt(plain, "key-one")
	if err != nil {
		t.Fatal(err)
	}
	if a == b {
		t.Fatal("encryption reused nonce")
	}
	got, err := Decrypt(a, "key-one")
	if err != nil || got != plain {
		t.Fatalf("round trip: %q %v", got, err)
	}
	if _, err := Decrypt(a, "key-two"); err == nil {
		t.Fatal("wrong key accepted")
	}
	raw, _ := base64.StdEncoding.DecodeString(a)
	raw[len(raw)-1] ^= 1
	if _, err := Decrypt(base64.StdEncoding.EncodeToString(raw), "key-one"); err == nil {
		t.Fatal("tampering accepted")
	}
	for _, s := range []string{"invalid!", "", "YQ=="} {
		if _, err := Decrypt(s, "key-one"); err == nil {
			t.Fatalf("invalid ciphertext accepted: %q", s)
		}
	}
}
