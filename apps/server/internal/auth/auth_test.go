package auth

import "testing"

func TestCredentialsAndSessionToken(t *testing.T) {
	display, normalized, err := NormalizeUsername("  Ａlice_1  ")
	if err != nil || display != "Alice_1" || normalized != "alice_1" {
		t.Fatalf("username display=%q normalized=%q err=%v", display, normalized, err)
	}
	if _, _, err := NormalizeUsername("bad name"); err == nil {
		t.Fatal("expected whitespace in username to be rejected")
	}

	hash, err := HashPassword("correct horse battery staple", bcryptTestCost)
	if err != nil {
		t.Fatal(err)
	}
	if !VerifyPassword(hash, "correct horse battery staple") || VerifyPassword(hash, "wrong password") {
		t.Fatal("password verification mismatch")
	}

	token, generatedHash, err := NewSessionToken()
	if err != nil {
		t.Fatal(err)
	}
	parsedHash, err := HashSessionToken(token)
	if err != nil || parsedHash != generatedHash {
		t.Fatalf("token hash mismatch err=%v", err)
	}
	if _, err := HashSessionToken(token + "="); err == nil {
		t.Fatal("expected non-canonical token to be rejected")
	}
}

const bcryptTestCost = 4
