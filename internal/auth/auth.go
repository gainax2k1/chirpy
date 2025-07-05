package auth

import (
	"fmt"
	"time"

	"github.com/golang-jwt/jwt/v5" // go get -u github.com/golang-jwt/jwt/v5
	"github.com/google/uuid"
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

func MakeJWT(userID uuid.UUID, tokenSecret string, expiresIn time.Duration) (string, error) {
	var newClaims jwt.RegisteredClaims
	newClaims.Issuer = "chirpy"

	var timeNow jwt.NumericDate
	timeNow.Time = time.Now().UTC()
	newClaims.IssuedAt = &timeNow

	var expireTime jwt.NumericDate
	expireTime.Time = time.Now().Add(expiresIn).UTC()
	newClaims.ExpiresAt = &expireTime

	newClaims.Subject = userID.String()

	newToken := jwt.NewWithClaims(jwt.SigningMethodHS256, newClaims)
	jwtString, err := newToken.SignedString(tokenSecret)
	if err != nil {
		return "", fmt.Errorf("error signing token: %w", err)
	}
	return jwtString, nil
}

func ValidateJWT(tokenString, tokenSecret string) (uuid.UUID, error) {

	var registeredClaims jwt.RegisteredClaims

	_, err := jwt.ParseWithClaims(tokenString, &registeredClaims,
		func(token *jwt.Token) (interface{}, error) {
			if token.Method != jwt.SigningMethodHS256 {
				return nil, fmt.Errorf("wrong jwt signature")
			}
			return []byte(tokenSecret), nil
		})

	if err != nil {
		return uuid.UUID{}, fmt.Errorf("error validating: %w", err)
	}

	userUUIDString, err := registeredClaims.GetSubject()
	if err != nil {
		return uuid.UUID{}, fmt.Errorf("error getting userUUID: %w", err)
	}

	userUUIDUUID, err := uuid.Parse(userUUIDString)
	if err != nil {
		return uuid.UUID{}, fmt.Errorf("error parsing userUUID: %w", err)
	}
	return userUUIDUUID, nil
}
