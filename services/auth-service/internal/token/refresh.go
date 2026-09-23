package token

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"fmt"
)

const refreshTokenBytes = 32

// GenerateRefreshToken returns the raw token (to send to the client)
// and its hash (to store in the DB).
//
// The raw token is never persisted by the server.
func GenerateRefreshToken() (raw string, hash string, err error) {
	buf := make([]byte, refreshTokenBytes)
	if _, err := rand.Read(buf); err != nil {
		return "", "", fmt.Errorf("token: read random: %w", err)
	}

	raw = base64.RawURLEncoding.EncodeToString(buf)
	hash = HashRefreshToken(raw)
	return raw, hash, nil
}

// HashRefreshToken computes the storage hash of a raw refresh token.
func HashRefreshToken(raw string) string {
	sum := sha256.Sum256([]byte(raw))
	return hex.EncodeToString(sum[:])
}