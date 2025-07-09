package auth

// testing GetAPIKey:

import (
	"net/http"
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

func TestGetBearerToken(t *testing.T) {

	testHeader := make(http.Header)
	testHeader.Set("Authorization", "")

	tokenString, err := GetBearerToken(testHeader) // test no bearer at all
	if err == nil || tokenString != "" {
		t.Errorf("expected error and emtpy string, got %v and %v instead.", tokenString, err)
		return
	}

	testHeader.Set("Authorization", "Bearer faketoken") // test small token
	if err == nil || tokenString != "" {
		t.Errorf("expected error and emtpy string, got %v and %v instead.", tokenString, err)
		return
	}

	//test long token
	testHeader.Set("Authorization", "Bearer faketokenthatistotallylongerandpossiblylongenough,butobviouslynotaCORRECT!!!TOKEN!!!BEARERbaererBaererTOKENTOKENtoken") // test small bearer
	if err == nil || tokenString != "" {
		t.Errorf("expected error and emtpy string, got %v and %v instead.", tokenString, err)
		return
	}

	//test bad bearer prefix
	testHeader.Set("Authorization", "BearerBearerBEARERbearer faketokenthatistotallylongerandpossiblylongenough,butobviouslynotaCORRECT!!!TOKEN!!!BEARERbaererBaererTOKENTOKENtoken") // test small bearer
	if err == nil || tokenString != "" {
		t.Errorf("expected error and emtpy string, got %v and %v instead.", tokenString, err)
		return
	}
}
