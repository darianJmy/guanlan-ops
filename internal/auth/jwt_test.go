package auth

import (
	"guanlan/internal/config"
	"strings"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

func TestTokenRoundTripUsesConfiguredSecret(t *testing.T) {
	previous := config.AppConfig
	t.Cleanup(func() { config.AppConfig = previous })
	config.AppConfig.JWTSecret = "  test-secret  "

	raw, err := GenerateToken(42, "admin")
	if err != nil {
		t.Fatal(err)
	}
	claims, err := ParseToken("  " + raw + "  ")
	if err != nil {
		t.Fatal(err)
	}
	if claims.UserID != 42 || claims.Username != "admin" {
		t.Fatalf("unexpected claims: %+v", claims)
	}
}

func TestParseTokenRequiresUserAndExpiration(t *testing.T) {
	previous := config.AppConfig
	t.Cleanup(func() { config.AppConfig = previous })
	config.AppConfig.JWTSecret = "test-secret"
	for _, claims := range []Claims{
		{UserID: 1},
		{RegisteredClaims: jwt.RegisteredClaims{ExpiresAt: jwt.NewNumericDate(time.Now().Add(time.Hour))}},
	} {
		raw, err := jwt.NewWithClaims(jwt.SigningMethodHS256, claims).SignedString([]byte(config.AppConfig.JWTSecret))
		if err != nil {
			t.Fatal(err)
		}
		if _, err := ParseToken(raw); err == nil {
			t.Fatal("accepted incomplete identity claims")
		}
	}
}

func TestParseTokenRejectsEmptySecret(t *testing.T) {
	previous := config.AppConfig
	t.Cleanup(func() { config.AppConfig = previous })
	config.AppConfig.JWTSecret = strings.Repeat(" ", 3)

	if _, err := GenerateToken(1, "admin"); err == nil {
		t.Fatal("expected empty secret error")
	}
}
