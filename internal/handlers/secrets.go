package handlers

import authutil "guanlan/internal/auth"

func encryptSecret(plain string) (string, error)      { return authutil.EncryptSecret(plain) }
func decryptSecret(ciphertext string) (string, error) { return authutil.DecryptSecret(ciphertext) }
