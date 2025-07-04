package auth

import (
	"fmt"

	"golang.org/x/crypto/bcrypt" // "go get golang.org/x/crypto/bcrypt" //to install
)

func HashPassword(password string) (string, error) {
	// func GenerateFromPassword(password []byte, cost int) ([]byte, error)
	// bcrypt.DefaultCost is the default cost, otherwise needs to be specified
	// default is 10, 20 would be high, 5 would be low (but faster)
	// it acts as the size of exponent for hashing.
	hashedPassword, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return "", fmt.Errorf("error hashing pass: %w", err)
	}

	return string(hashedPassword), nil
}

func CheckPasswordHash(password, hash string) error {
	// func CompareHashAndPassword(hashedPassword, password []byte) error
	err := bcrypt.CompareHashAndPassword([]byte(hash), []byte(password))
	if err != nil {
		return fmt.Errorf("password does not match: %w", err)
	}
	return nil
}
