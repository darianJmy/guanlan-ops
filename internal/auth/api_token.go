package auth

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
)

const APITokenPrefix = "aiops_pat_"

func NewAPIToken() (string, string, string, error) {
	idBytes := make([]byte, 16)
	secretBytes := make([]byte, 32)
	if _, err := rand.Read(idBytes); err != nil {
		return "", "", "", err
	}
	if _, err := rand.Read(secretBytes); err != nil {
		return "", "", "", err
	}
	secret := APITokenPrefix + base64.RawURLEncoding.EncodeToString(secretBytes)
	return hex.EncodeToString(idBytes), secret, secret[:len(APITokenPrefix)+8], nil
}

func HashAPIToken(token string) string {
	sum := sha256.Sum256([]byte(token))
	return hex.EncodeToString(sum[:])
}
