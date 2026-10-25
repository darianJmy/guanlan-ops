package auth

import (
	"strings"
	"testing"
)

func TestNewAPITokenReturnsDistinctHashedCredentials(t *testing.T) {
	id1, token1, prefix1, err := NewAPIToken()
	if err != nil {
		t.Fatal(err)
	}
	id2, token2, _, err := NewAPIToken()
	if err != nil {
		t.Fatal(err)
	}
	if len(id1) != 32 || len(id2) != 32 || id1 == id2 {
		t.Fatalf("unexpected public IDs: %q %q", id1, id2)
	}
	if !strings.HasPrefix(token1, APITokenPrefix) || token1 == token2 {
		t.Fatalf("unexpected API tokens")
	}
	if prefix1 != token1[:len(APITokenPrefix)+8] {
		t.Fatalf("prefix = %q, want token display prefix", prefix1)
	}
	if HashAPIToken(token1) == token1 || len(HashAPIToken(token1)) != 64 {
		t.Fatalf("token hash is not a SHA-256 hex digest")
	}
}
