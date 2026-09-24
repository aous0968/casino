package token

import (
	"errors"
	"testing"
	"time"
)

const testSecret = "0123456789012345678901234567890123456789"

func TestSignVerifyRoundTrip(t *testing.T) {
	s := NewSigner([]byte(testSecret), "test-issuer", time.Minute)

	raw, err := s.Sign("user-42")
	if err != nil {
		t.Fatal(err)
	}

	claims, err := s.Verify(raw)
	if err != nil {
		t.Fatal(err)
	}
	if claims.Subject != "user-42" {
		t.Fatalf("sub = %q, want user-42", claims.Subject)
	}
	if claims.Issuer != "test-issuer" {
		t.Fatalf("iss = %q", claims.Issuer)
	}
}

func TestVerifyRejectsWrongSecret(t *testing.T) {
	good := NewSigner([]byte(testSecret), "test-issuer", time.Minute)
	bad := NewSigner([]byte("9999999999999999999999999999999999999999"), "test-issuer", time.Minute)

	raw, _ := good.Sign("user-42")
	_, err := bad.Verify(raw)
	if !errors.Is(err, ErrInvalidToken) {
		t.Fatalf("want ErrInvalidToken, got %v", err)
	}
}

func TestVerifyRejectsWrongIssuer(t *testing.T) {
	a := NewSigner([]byte(testSecret), "issuer-a", time.Minute)
	b := NewSigner([]byte(testSecret), "issuer-b", time.Minute)

	raw, _ := a.Sign("user-42")
	_, err := b.Verify(raw)
	if !errors.Is(err, ErrInvalidToken) {
		t.Fatalf("want ErrInvalidToken, got %v", err)
	}
}

func TestVerifyRejectsExpired(t *testing.T) {
	s := NewSigner([]byte(testSecret), "test-issuer", -time.Second)

	raw, _ := s.Sign("user-42")
	_, err := s.Verify(raw)
	if !errors.Is(err, ErrExpiredToken) {
		t.Fatalf("want ErrExpiredToken, got %v", err)
	}
}

func TestVerifyRejectsTampered(t *testing.T) {
	s := NewSigner([]byte(testSecret), "test-issuer", time.Minute)
	raw, _ := s.Sign("user-42")

	// Flip the last character of the token.
	tampered := raw[:len(raw)-1]
	if raw[len(raw)-1] == 'a' {
		tampered += "b"
	} else {
		tampered += "a"
	}

	_, err := s.Verify(tampered)
	if !errors.Is(err, ErrInvalidToken) {
		t.Fatalf("want ErrInvalidToken, got %v", err)
	}
}