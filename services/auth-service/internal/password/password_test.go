package password

import (
	"strings"
	"testing"
)

func TestHashVerifyRoundTrip(t *testing.T) {
	hash, err := Hash("hunter2hunter2")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(hash, "$argon2id$") {
		t.Fatalf("hash should start with $argon2id$, got %q", hash)
	}

	ok, err := Verify("hunter2hunter2", hash)
	if err != nil {
		t.Fatal(err)
	}
	if !ok {
		t.Fatal("correct password did not verify")
	}

	ok, err = Verify("wrong-password", hash)
	if err != nil {
		t.Fatal(err)
	}
	if ok {
		t.Fatal("wrong password verified")
	}
}

func TestHashProducesUniqueSalts(t *testing.T) {
	h1, _ := Hash("same-password")
	h2, _ := Hash("same-password")
	if h1 == h2 {
		t.Fatal("two hashes of the same password are identical (salt not working)")
	}
}

func TestVerifyRejectsMalformedHash(t *testing.T) {
	cases := []string{
		"",
		"not-a-hash",
		"$argon2id$",
		"$bcrypt$v=19$m=65536,t=3,p=4$abc$def",
		"$argon2id$v=999$m=65536,t=3,p=4$abc$def",
	}
	for _, c := range cases {
		t.Run(c, func(t *testing.T) {
			_, err := Verify("anything", c)
			if err == nil {
				t.Fatalf("expected error for %q", c)
			}
		})
	}
}