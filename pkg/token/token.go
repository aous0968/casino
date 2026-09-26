package token

import (
	"errors"
	"fmt"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
)

var (
	ErrInvalidToken = errors.New("token: invalid")
	ErrExpiredToken = errors.New("token: expired")
)

// Claims is the payload of an access token.
type Claims struct {
	jwt.RegisteredClaims
}

// ---------- Verifier ----------

type Verifier struct {
	secret []byte
	issuer string
}

func NewVerifier(secret []byte, issuer string) *Verifier {
	return &Verifier{secret: secret, issuer: issuer}
}

func (v *Verifier) Verify(raw string) (*Claims, error) {
	claims := &Claims{}

	_, err := jwt.ParseWithClaims(raw, claims, func(t *jwt.Token) (any, error) {
		if _, ok := t.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, fmt.Errorf("token: unexpected signing method %v", t.Header["alg"])
		}
		return v.secret, nil
	},
		jwt.WithIssuer(v.issuer),
		jwt.WithExpirationRequired(),
	)

	if err != nil {
		if errors.Is(err, jwt.ErrTokenExpired) {
			return nil, ErrExpiredToken
		}
		return nil, ErrInvalidToken
	}
	return claims, nil
}

// ---------- Signer ----------

type Signer struct {
	*Verifier
	accessTTL time.Duration
}

func NewSigner(secret []byte, issuer string, accessTTL time.Duration) *Signer {
	return &Signer{
		Verifier:  NewVerifier(secret, issuer),
		accessTTL: accessTTL,
	}
}

func (s *Signer) Sign(userID string) (string, error) {
	now := time.Now()
	claims := Claims{
		RegisteredClaims: jwt.RegisteredClaims{
			Subject:   userID,
			Issuer:    s.issuer,
			IssuedAt:  jwt.NewNumericDate(now),
			ExpiresAt: jwt.NewNumericDate(now.Add(s.accessTTL)),
			ID:        uuid.NewString(),
		},
	}
	tok := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	signed, err := tok.SignedString(s.secret)
	if err != nil {
		return "", fmt.Errorf("token: sign: %w", err)
	}
	return signed, nil
}