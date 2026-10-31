package auth

import (
	"errors"
	"guanlan/internal/config"
	"strings"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

type Claims struct {
	UserID   uint   `json:"user_id"`
	Username string `json:"username"`
	jwt.RegisteredClaims
}

func GenerateToken(userID uint, username string) (string, error) {
	secret := strings.TrimSpace(config.AppConfig.JWTSecret)
	if secret == "" {
		return "", errors.New("jwt secret is empty")
	}
	claims := Claims{
		UserID:   userID,
		Username: username,
		RegisteredClaims: jwt.RegisteredClaims{
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(24 * time.Hour)),
			IssuedAt:  jwt.NewNumericDate(time.Now()),
		},
	}
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	return token.SignedString([]byte(secret))
}

func ParseToken(tokenString string) (*Claims, error) {
	secret := strings.TrimSpace(config.AppConfig.JWTSecret)
	if secret == "" {
		return nil, errors.New("jwt secret is empty")
	}
	tokenString = strings.TrimSpace(tokenString)
	if tokenString == "" {
		return nil, jwt.ErrTokenMalformed
	}
	token, err := jwt.ParseWithClaims(tokenString, &Claims{}, func(t *jwt.Token) (interface{}, error) {
		return []byte(secret), nil
	}, jwt.WithValidMethods([]string{jwt.SigningMethodHS256.Alg()}), jwt.WithExpirationRequired())
	if err != nil {
		return nil, err
	}
	if claims, ok := token.Claims.(*Claims); ok && token.Valid && claims.UserID > 0 {
		return claims, nil
	}
	return nil, jwt.ErrSignatureInvalid
}
