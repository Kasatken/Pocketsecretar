package security

import "testing"

func TestHashAndCheckPassword(t *testing.T) {
	hash, err := HashPassword("supersecret123")
	if err != nil {
		t.Fatalf("hash password: %v", err)
	}

	if !CheckPassword(hash, "supersecret123") {
		t.Fatal("expected password to match hash")
	}
	if CheckPassword(hash, "wrong-password") {
		t.Fatal("expected wrong password to fail")
	}
}
