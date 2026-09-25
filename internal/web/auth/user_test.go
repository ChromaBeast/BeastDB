package auth

import "testing"

func TestHashAndVerifyPassword(t *testing.T) {
	password := "SecretP@ssw0rd!"

	hash, err := HashPassword(password)
	if err != nil {
		t.Fatalf("HashPassword failed: %v", err)
	}

	ok, err := VerifyPassword(password, hash)
	if err != nil {
		t.Fatalf("VerifyPassword failed: %v", err)
	}
	if !ok {
		t.Fatalf("expected password verification to succeed")
	}

	// Wrong password check
	ok, err = VerifyPassword("WrongPassword", hash)
	if err != nil {
		t.Fatalf("VerifyPassword unexpected error on mismatch: %v", err)
	}
	if ok {
		t.Fatalf("expected password verification to fail for wrong password")
	}
}
