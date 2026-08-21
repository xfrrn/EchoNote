package main

import "testing"

func TestValidateTestDatabase(t *testing.T) {
	for _, value := range []string{
		"postgres://localhost/echonote_test",
		"postgresql://localhost/echonote_test_ci?sslmode=disable",
		"pgx5://localhost/echonote_browser_smoke",
	} {
		if err := validateTestDatabase(value); err != nil {
			t.Fatalf("%s: %v", value, err)
		}
	}
	for _, value := range []string{
		"postgres://localhost/echonote",
		"postgres://localhost/echonote_test/other",
		"mysql://localhost/echonote_test",
	} {
		if err := validateTestDatabase(value); err == nil {
			t.Fatalf("expected %s to be rejected", value)
		}
	}
}
