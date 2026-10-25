package auth

import "testing"

func TestSecretRoundTrip(t *testing.T) {
	ciphertext, err := EncryptSecret("  example-secret  ")
	if err != nil {
		t.Fatalf("encrypt secret: %v", err)
	}
	if ciphertext == "" || ciphertext == "example-secret" {
		t.Fatalf("secret was not encrypted")
	}
	plain, err := DecryptSecret(ciphertext)
	if err != nil {
		t.Fatalf("decrypt secret: %v", err)
	}
	if plain != "example-secret" {
		t.Fatalf("plain secret = %q", plain)
	}
}
