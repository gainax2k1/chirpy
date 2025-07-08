package auth

// testing GetAPIKey:

import (
	"testing"
)

// Test functions must take one argument of type *testing.T
// - provides ways to print, skip, and fail test
//    *testing.T

func TestHashPassword(t *testing.T) {
	emptyString := ""
	returnString, err := HashPassword(emptyString)
	if returnString != emptyString {
		t.Errorf("expected string: %v, got: %v", emptyString, returnString)
	}
	if err == nil {
		t.Errorf("expected error, got none")
	}

	regPassword := "password"
	returnString, err = HashPassword(regPassword)
	if returnString == emptyString {
		t.Errorf("hashed password empty")
	}
	if err != nil {
		t.Errorf("expected no error, got: %v", err)
	}
}
