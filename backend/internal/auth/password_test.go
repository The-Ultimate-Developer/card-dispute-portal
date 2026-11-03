package auth

import (
	"strings"
	"testing"
)

func TestHashAndVerify(t *testing.T) {
	hash, err := HashPassword("correct horse battery staple")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(hash, "$argon2id$v=19$m=19456,t=2,p=1$") {
		t.Fatalf("unexpected format: %s", hash)
	}

	ok, err := VerifyPassword(hash, "correct horse battery staple")
	if err != nil || !ok {
		t.Fatalf("correct password rejected: ok=%v err=%v", ok, err)
	}
	ok, err = VerifyPassword(hash, "wrong password")
	if err != nil || ok {
		t.Fatalf("wrong password accepted: ok=%v err=%v", ok, err)
	}
}

func TestHashesAreSalted(t *testing.T) {
	a, _ := HashPassword("same")
	b, _ := HashPassword("same")
	if a == b {
		t.Fatal("two hashes of the same password must differ")
	}
}

func TestVerifyRejectsMalformedHashes(t *testing.T) {
	for _, h := range []string{
		"",
		"plaintext",
		"$bcrypt$v=19$m=1,t=1,p=1$c2FsdA$aGFzaA",
		"$argon2id$v=18$m=19456,t=2,p=1$c2FsdA$aGFzaA",
		"$argon2id$v=19$m=x,t=2,p=1$c2FsdA$aGFzaA",
		"$argon2id$v=19$m=19456,t=2,p=1$!!!$aGFzaA",
	} {
		if _, err := VerifyPassword(h, "x"); err == nil {
			t.Errorf("expected error for %q", h)
		}
	}
}
