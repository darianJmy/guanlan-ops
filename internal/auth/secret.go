package auth

import (
	"guanlan/internal/config"
	"guanlan/pkg/cryptoutil"
	"strings"
)

func EncryptSecret(plain string) (string, error) {
	return cryptoutil.Encrypt(strings.TrimSpace(plain), config.AppConfig.JWTSecret)
}
func DecryptSecret(ciphertext string) (string, error) {
	return cryptoutil.Decrypt(ciphertext, config.AppConfig.JWTSecret)
}
