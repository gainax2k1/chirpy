package auth

import (
	"fmt"
	"net/http"
	"strings"
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
	if len(password) == 0 {
		return "", fmt.Errorf("error hashing password of zero length ")
	}
	hashedPassword, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return "", fmt.Errorf("error hashing pass: %w", err)
	}

	return string(hashedPassword), nil
}

func CheckPasswordHash(password, hash string) error {
	err := bcrypt.CompareHashAndPassword([]byte(hash), []byte(password))
	if err != nil {
		return fmt.Errorf("password does not match: %w", err)
	}
	return nil
}

func MakeJWT(userID uuid.UUID, tokenSecret string, expiresIn time.Duration) (string, error) {

	//Create a variable to hold the "claims"—the standard fields about the token and user.
	var newClaims jwt.RegisteredClaims

	// Set the Issuer field, declaring who made this token — here, the "chirpy" app.
	newClaims.Issuer = "chirpy"

	// Set the IssuedAt field to the current UTC time (when the token was created).
	var timeNow jwt.NumericDate
	timeNow.Time = time.Now().UTC()
	newClaims.IssuedAt = &timeNow

	// Set the ExpiresAt field to a future moment (current time plus however long you want it to last).
	var expireTime jwt.NumericDate
	expireTime.Time = time.Now().Add(expiresIn).UTC()
	newClaims.ExpiresAt = &expireTime

	// Store the user’s ID (converted to string) in the Subject field.
	// This identifies the user the token is about.
	newClaims.Subject = userID.String()

	//Create a new token and tell the JWT library to sign it
	// using HMAC SHA256, including your claims from above.
	newToken := jwt.NewWithClaims(jwt.SigningMethodHS256, newClaims)

	//Sign (cryptographically seal) the token using your secret.
	// This produces a "JWT string"—just a base64 string you can hand out.
	jwtString, err := newToken.SignedString([]byte(tokenSecret)) // secret needs to be []byte, not string
	//jwtString, err := newToken.SignedString(tokenSecret) //WRONG!
	if err != nil {
		return "", fmt.Errorf("error signing token: %w", err)
	}
	return jwtString, nil
}

func ValidateJWT(tokenString, tokenSecret string) (uuid.UUID, error) {
	// Prepare a place to extract the claims from the incoming token.
	var registeredClaims jwt.RegisteredClaims

	//  Parse the token string using the JWT library and try to populate registeredClaims.
	// - The callback checks the signature method and passes in your secret so
	// the JWT library can verify the authenticity.
	// - The library checks if the signature is valid, if it’s expired, etc.

	_, err := jwt.ParseWithClaims(tokenString, &registeredClaims,
		func(token *jwt.Token) (interface{}, error) { //anonymous function "key function".  Its job is
			//  to tell the JWT library which cryptographic key (or secret) should be used to
			// verify the token's signature.
			if token.Method != jwt.SigningMethodHS256 {
				return nil, fmt.Errorf("wrong jwt signature")
			}
			return []byte(tokenSecret), nil
		})

	if err != nil {
		return uuid.UUID{}, fmt.Errorf("error validating: %w", err)
	}

	// Extract the Subject (should be the user's UUID as a string),
	//  parse it back into a uuid.UUID, and return it if all is well.
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

func GetBearerToken(headers http.Header) (string, error) {
	// Auth information will come into our server in the Authorization header:
	// Bearer TOKEN_STRING
	/*
		This function should look for the Authorization header in the headers parameter and
		return the TOKEN_STRING if it exists (stripping off the Bearer prefix and whitespace).
		If the header doesn't exist, return an error.
	*/

	tokenStringHeader := headers.Get("Authorization") // get the first item with this key
	if tokenStringHeader == "" {
		return "", fmt.Errorf("unable to retrieve authorization header")
	}

	tokenStringHeader = strings.TrimSpace(tokenStringHeader) // trim leading whitespace

	if !strings.HasPrefix(tokenStringHeader, "Bearer ") { // checks for proper header leader.
		//  Note, "space" after Bearer to make sure that's all that's there,
		//  not something like "BearerToken" which would be invalid
		return "", fmt.Errorf("invalid authorization header")

	}

	token := strings.TrimPrefix(tokenStringHeader, "Bearer")

	token = strings.TrimSpace(token)
	if len(token) < 30 {
		return "", fmt.Errorf("invalid token")
	}
	return token, nil
}
