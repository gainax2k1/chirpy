package main

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"strings"
	"sync/atomic"
	"time"

	"github.com/gainax2k1/chirpy/internal/auth"
	"github.com/gainax2k1/chirpy/internal/database"
	"github.com/google/uuid"

	"github.com/joho/godotenv"
	_ "github.com/lib/pq"
)

type apiConfig struct {
	db             *database.Queries
	fileserverHits atomic.Int32
	/*
		The atomic.Int32 type is a really cool standard-library type that allows us
		to safely increment and read an integer value across multiple goroutines
		(HTTP requests).
	*/
	platform string
	secret   string
	polkaKey string
}

type User struct {
	ID           uuid.UUID `json:"id"`
	CreatedAt    time.Time `json:"created_at"`
	UpdatedAt    time.Time `json:"updated_at"`
	Email        string    `json:"email"`
	Token        string    `json:"token"`
	RefreshToken string    `json:"refresh_token"`
	IsChirpyRed  bool      `json:"is_chirpy_red"`
}

type Chirp struct {
	ID        uuid.UUID `json:"id"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
	Body      string    `json:"body"`
	UserID    uuid.UUID `json:"user_id"`
}

type CreateUserRequest struct {
	Email    string `json:"email"`
	Password string `json:"password"`
}

type CreateChirp struct {
	Body    string    `json:"body"`
	User_ID uuid.UUID `json:"user_id"`
}

type errResponse struct {
	Error string `json:"error"`
}

type tokenResponse struct {
	Token string `json:"token"`
}

type userData struct {
	User_ID uuid.UUID `json:"user_id"`
}

type userEvent struct {
	Event string   `json:"event"`
	Data  userData `json:"data"`
}

/*
{
  "event": "user.upgraded",
  "data": {
    "user_id": "3311741c-680c-4546-99f3-fc9efac2036c"
  }
}
*/

func main() {
	err := godotenv.Load()
	if err != nil {
		log.Fatal("Error loading .env file")
	}

	dbURL := os.Getenv("DB_URL")
	platform := os.Getenv("PLATFORM")
	secret := os.Getenv("SECRET")
	polkaKey := os.Getenv("POLKA_KEY")
	db, err := sql.Open("postgres", dbURL)
	if err != nil {
		fmt.Println("error opening sql: ", err)
		os.Exit(1)

	}
	defer db.Close()

	dbQueries := database.New(db)

	cfg := &apiConfig{
		db:       dbQueries,
		platform: platform,
		secret:   secret,
		polkaKey: polkaKey,
	}

	// This creates a "multiplexer"—a router for incoming HTTP requests.
	// It decides which handler should process requests for different URL paths.
	mux := http.NewServeMux()

	// Actually makes the server that listens on port 8080 and uses the mux that was just created.
	newServer := http.Server{
		Addr:    ":8080",
		Handler: mux,
	}

	// Tells tbe mux that any request starting with "/" should be handled by a fileserver serving from
	// the current directory.
	//  This allows files like "index.html" (and other static files) to be served for most requests.
	// first version:
	// mux.Handle("/", http.FileServer(http.Dir(".")))
	// after adding readiness():

	mux.Handle("/app/", cfg.middlewareMetricsInc(http.StripPrefix("/app/", http.FileServer(http.Dir(".")))))
	mux.Handle("/assets/", http.StripPrefix("/assets/", http.FileServer(http.Dir("./assets"))))

	mux.HandleFunc("POST /admin/reset", cfg.middlewareMetricsHandlerReset)
	mux.HandleFunc("GET /api/healthz", readiness) // correct!
	mux.HandleFunc("GET /admin/metrics", cfg.middlewareMetricsStats)
	mux.HandleFunc("POST /api/chirps", cfg.createChirps)
	mux.HandleFunc("DELETE /api/chirps/{chirpID}", cfg.deleteChirp)
	mux.HandleFunc("GET /api/chirps", cfg.getChirps)
	mux.HandleFunc("GET /api/chirp/{chirpID}", cfg.getChirp)

	mux.HandleFunc("POST /api/users", cfg.createUser)
	mux.HandleFunc("POST /api/login", cfg.loginUser)
	mux.HandleFunc("POST /api/refresh", cfg.refresh)
	mux.HandleFunc("POST /api/revoke", cfg.revoke)
	mux.HandleFunc("PUT /api/users", cfg.updateUser)
	mux.HandleFunc("POST /api/polka/webhooks", cfg.userEvents) // so far, upgrading to "chirpy red"

	//mux.HandleFunc("POST /admin/reset", cfg.middlewareMetricsReset) //old reset that reset the page view counter
	//mux.HandleFunc("POST /api/validate_chirp", cfg.middlewareMetricsValidate) // old seperate validate case

	// starts your server and keeps it running, handling incoming HTTP requests as per your routing rules.
	err = newServer.ListenAndServe()
	if err != nil {
		log.Fatal(err)
	}

}

// "http.ResponseWriter" has methods like Header().Set() to set headers, WriteHeader() to set
// the status code, and Write() to write the body of the response.
// The server creates this for you for each incoming request.
// "*http.Request" includes things like the HTTP method (GET, POST, etc.),
// the URL path, headers, and the request body (if there is one).
// The server also creates this for you for each incoming request.

func readiness(w http.ResponseWriter, req *http.Request) {
	w.Header().Set("Content-Type", "text/plain; charset=utf-8") // normal header
	w.WriteHeader(200)                                          // status code
	w.Write([]byte("OK"))                                       // << expects []byte, so type convert to have "OK" (for now)

}

func (cfg *apiConfig) middlewareMetricsHandlerReset(w http.ResponseWriter, req *http.Request) { // **** UNDER CONSTRUCTION! ****
	if cfg.platform != "dev" {
		respondWithError(w, 403, "Forbidden")
		return
	}

	err := cfg.db.Reset(context.Background())
	if err != nil {
		respondWithError(w, 400, "Bad Request")
	}

	fmt.Printf("Database successfully reset.")
}

func (cfg *apiConfig) middlewareMetricsInc(next http.Handler) http.Handler {
	/*
		THIS WONT'T WORK! it only runs ONCE at startup!
		- cfg.fileserverHits.Add(1)
		- return next
	*/
	//correct code: we return our modified hanlder at startup.
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		cfg.fileserverHits.Add(1) // This line runs on EVERY request, "baked into" the handler
		next.ServeHTTP(w, r)      //continues the call
	})

	/* EXPLANATION:
	At startup:
		- You call apiCfg.middlewareMetricsInc(fileServer)
		- Your middleware function runs, receives fileServer as the next parameter
		- Your middleware creates a new function that will increment + call next
		- Your middleware returns that new function (*wrapped as a handler*)
		- Mux stores that returned handler

	*/
}

func (cfg *apiConfig) middlewareMetricsStats(w http.ResponseWriter, req *http.Request) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8") // normal header
	w.WriteHeader(200)                                         // status code
	returnHits := fmt.Sprintf("<html><body><h1>Welcome, Chirpy Admin</h1><p>Chirpy has been visited %d times!</p></body></html>", cfg.fileserverHits.Load())
	w.Write([]byte(returnHits)) // << expects []byte, so type convert to have "OK" (for now)

}

/*
	OLD RESET - reset page hit-counter

	func (cfg *apiConfig) middlewareMetricsReset(w http.ResponseWriter, req *http.Request) {
		w.Header().Set("Content-Type", "text/plain; charset=utf-8") // normal header
		w.WriteHeader(200)                                          // status code
		cfg.fileserverHits.Store(0)
		returnHits := fmt.Sprint("Hits reset: ", cfg.fileserverHits.Load())
		w.Write([]byte(returnHits)) // << expects []byte, so type convert to have "OK" (for now)

}
*/

func (cfg *apiConfig) createUser(w http.ResponseWriter, req *http.Request) {

	decoder := json.NewDecoder(req.Body)
	newUserParams := CreateUserRequest{}

	err := decoder.Decode(&newUserParams)
	if err != nil {
		respondWithError(w, 500, "Error decoding params")
		return
	}

	newUserParams.Password, err = auth.HashPassword(newUserParams.Password)
	if err != nil {
		respondWithError(w, 500, "error creating password")
		return
	}

	var createUserParams database.CreateUserParams
	createUserParams.Email = newUserParams.Email
	createUserParams.HashedPassword = newUserParams.Password
	newUserRecord, err := cfg.db.CreateUser(context.Background(), createUserParams)
	if err != nil {
		//error creating new user
		respondWithError(w, 500, "error creating user")
		return
	}

	mainUser := User{ // converting to ensure security (not exposing sql field names, allows not returning specific values, like potential password, etc)
		ID:          newUserRecord.ID,
		CreatedAt:   newUserRecord.CreatedAt,
		UpdatedAt:   newUserRecord.UpdatedAt,
		Email:       newUserRecord.Email,
		IsChirpyRed: newUserRecord.IsChirpyRed,
	}

	jsonWriter(w, 201, mainUser)
}

func (cfg *apiConfig) loginUser(w http.ResponseWriter, req *http.Request) {
	decoder := json.NewDecoder(req.Body)
	userLoginParams := CreateUserRequest{} // struct with email and password
	err := decoder.Decode(&userLoginParams)
	if err != nil {
		respondWithError(w, 500, "Error decoding params")
		return
	}

	dbUserRecord, err := cfg.db.GetUserByEmail(context.Background(), userLoginParams.Email)
	if err != nil {
		respondWithError(w, 401, "Unauthorize (getuserbyemail failed)")
		return
	}

	err = auth.CheckPasswordHash(userLoginParams.Password, dbUserRecord.HashedPassword)
	if err != nil {
		respondWithError(w, 401, "Unauthorized (checkpasswordhash failed)")
		return
	}

	token, err := auth.MakeJWT(dbUserRecord.ID, cfg.secret)
	if err != nil {
		respondWithError(w, 401, "Unauthorized")
		return
	}
	refreshToken, err := auth.MakeRefreshToken()
	if err != nil {
		respondWithError(w, 500, "error creating refresh token")
		return
	}

	newRefreshTokenParams := database.CreateRefreshTokenParams{
		Token:     refreshToken,
		ExpiresAt: time.Now().Add(time.Hour * 24 * 60), // 60 days
		UserID:    dbUserRecord.ID,
	}

	_, err = cfg.db.CreateRefreshToken(context.Background(), newRefreshTokenParams)
	if err != nil {
		// Let's print the actual error from the database!
		fmt.Printf("Database error creating refresh token: %v\n", err)       // <--- Add this print statement!
		respondWithError(w, 500, "error creating refresh token in database") // Keep this for user response
		return
	}

	mainUser := User{ // converting to ensure security (not exposing sql field names, allows not returning specific values, like potential password, etc)
		ID:           dbUserRecord.ID,
		CreatedAt:    dbUserRecord.CreatedAt,
		UpdatedAt:    dbUserRecord.UpdatedAt,
		Email:        dbUserRecord.Email,
		Token:        token,
		RefreshToken: refreshToken,
		IsChirpyRed:  dbUserRecord.IsChirpyRed,
	}

	jsonWriter(w, 200, mainUser)
}

func (cfg *apiConfig) updateUser(w http.ResponseWriter, req *http.Request) {
	decoder := json.NewDecoder(req.Body)
	updateUserParams := CreateUserRequest{}

	err := decoder.Decode(&updateUserParams)
	if err != nil {
		respondWithError(w, 500, "Error decoding params")
		return
	}

	// At this point, I have the user params (password and email)
	// -- i need to verify token here
	// getting user's token from header
	token, err := auth.GetBearerToken(req.Header)
	if err != nil {
		respondWithError(w, 401, "Unauthorized")
		return
	}

	userIDVerified, err := auth.ValidateJWT(token, cfg.secret)
	if err != nil {
		respondWithError(w, 401, "Unauthorized")
		return
	}

	// at this point, i have the UUID for the token in  header.

	// by now, user will need to have been verified correctly
	updateUserParams.Password, err = auth.HashPassword(updateUserParams.Password)
	if err != nil {
		respondWithError(w, 500, "error creating password")

		return
	}

	var updateDBUserParams database.UpdateUserParams
	updateDBUserParams.Email = updateUserParams.Email
	updateDBUserParams.HashedPassword = updateUserParams.Password
	updateDBUserParams.ID = userIDVerified

	updatedUserRecord, err := cfg.db.UpdateUser(context.Background(), updateDBUserParams)

	if err != nil {
		//error creating new user
		respondWithError(w, 500, "error creating user")
		return
	}

	updatedUserInfo := User{
		ID:          updatedUserRecord.ID,
		CreatedAt:   updatedUserRecord.CreatedAt,
		UpdatedAt:   updatedUserRecord.UpdatedAt,
		Email:       updatedUserRecord.Email,
		IsChirpyRed: updatedUserRecord.IsChirpyRed,
	}

	jsonWriter(w, 200, updatedUserInfo)
}

func (cfg *apiConfig) userEvents(w http.ResponseWriter, req *http.Request) {
	decoder := json.NewDecoder(req.Body)
	params := userEvent{}

	err := decoder.Decode(&params)
	if err != nil {
		respondWithError(w, 500, "Error decoding params")
		return
	}

	if params.Event != "user.upgraded" {
		w.WriteHeader(204)
		return
	}
	apiKey, err := auth.GetAPIKey(req.Header)
	if err != nil || apiKey != cfg.polkaKey {
		respondWithError(w, 401, "Unauthorized")
		return
	}

	_, err = cfg.db.UpgradeUser(context.Background(), params.Data.User_ID)
	if err != nil {
		// if err == sql.ErrNoRows {} for explicitely checking user not found (row not found)
		w.WriteHeader(204)
	}
}

func (cfg *apiConfig) refresh(w http.ResponseWriter, req *http.Request) {
	token, err := auth.GetBearerToken(req.Header) //getting refresh token from user
	if err != nil {
		respondWithError(w, 401, "unable to refresh token")
		return
	}

	refreshTokenRecord, err := cfg.db.GetUserByRefreshToken(context.Background(), token)
	if err != nil {
		respondWithError(w, 401, "unable to refresh token")
		return
	}

	if refreshTokenRecord.ExpiresAt.Compare(time.Now()) < 0 {
		//revoke call () //not implimented yet
		respondWithError(w, 401, "unable to refresh token")
		return
	}
	if refreshTokenRecord.RevokedAt.Valid {
		//revoke call () //not implimented yet
		respondWithError(w, 401, "unable to refresh token")
		return
	}

	jwtToken, err := auth.MakeJWT(refreshTokenRecord.UserID, cfg.secret)
	if err != nil {
		respondWithError(w, 401, "unable to refresh token")
		return
	}

	returnJWTToken := tokenResponse{
		Token: jwtToken,
	}

	jsonWriter(w, 200, returnJWTToken)
}
func (cfg *apiConfig) revoke(w http.ResponseWriter, req *http.Request) {
	token, err := auth.GetBearerToken(req.Header) //getting refresh token from user
	if err != nil {
		respondWithError(w, 401, "unable to refresh token")
		return
	}
	cfg.db.RevokeRefreshToken(context.Background(), token)

	w.WriteHeader(204)
}

func (cfg *apiConfig) createChirps(w http.ResponseWriter, req *http.Request) {
	decoder := json.NewDecoder(req.Body)
	params := CreateChirp{}

	err := decoder.Decode(&params)
	if err != nil {
		respondWithError(w, 500, "Error decoding params")
		return
	}

	token, err := auth.GetBearerToken(req.Header)
	if err != nil {
		respondWithError(w, 401, "Unauthorized")
		return
	}
	userIDVerified, err := auth.ValidateJWT(token, cfg.secret)
	if err != nil {
		respondWithError(w, 401, "Unauthorized")
		return
	}

	characterCount := len(params.Body)

	// ENCODE JSON RESPONSE BODY:

	if characterCount > 140 { //invalid case
		respondWithError(w, 400, "Chirp is too long")
		return
	}
	// At this point, CHIRP is good to go:
	var chirpParams database.CreateChirpParams
	chirpParams.Body = filterProfanity(params.Body) // not sure if we're still filtering, but this would be teh place to do so
	chirpParams.UserID = userIDVerified

	dbChirp, err := cfg.db.CreateChirp(context.Background(), chirpParams)
	if err != nil {
		respondWithError(w, 500, "error creating chirp")
		return
	}

	mainChirp := Chirp{ // converting to ensure security (not exposing sql field names, allows not returning specific values, like potential password, etc)
		ID:        dbChirp.ID,
		CreatedAt: dbChirp.CreatedAt,
		UpdatedAt: dbChirp.UpdatedAt,
		Body:      dbChirp.Body,
		UserID:    dbChirp.UserID,
	}

	jsonWriter(w, 201, mainChirp)
}

func (cfg *apiConfig) getChirp(w http.ResponseWriter, req *http.Request) {
	chirpIDString := req.PathValue("chirpID") // pulls the chirp id from the path string as a STRING
	fmt.Println(chirpIDString)

	chirpUUID, err := uuid.Parse(chirpIDString) // converts the string into a UUID
	if err != nil {
		respondWithError(w, 500, "UUID error")
		return
	}

	dbChirp, err := cfg.db.GetChirpByChirpUUID(context.Background(), chirpUUID)
	if err != nil {
		respondWithError(w, 404, "chirp not found")
		return
	}

	mainChirp := Chirp{ // converting to ensure security (not exposing sql field names, allows not returning specific values, like potential password, etc)
		ID:        dbChirp.ID,
		CreatedAt: dbChirp.CreatedAt,
		UpdatedAt: dbChirp.UpdatedAt,
		Body:      dbChirp.Body,
		UserID:    dbChirp.UserID,
	}

	jsonWriter(w, 200, mainChirp)
}

func (cfg *apiConfig) deleteChirp(w http.ResponseWriter, req *http.Request) {
	// delete requests don't contain a request body!
	// getting user's token from header
	token, err := auth.GetBearerToken(req.Header)
	if err != nil {
		respondWithError(w, 401, "Unauthorized")
		return
	}

	userIDVerified, err := auth.ValidateJWT(token, cfg.secret)
	if err != nil {
		respondWithError(w, 401, "Unauthorized")
		return
	}

	chirpIDString := req.PathValue("chirpID") // pulls the chirp id from the path string as a STRING
	fmt.Println(chirpIDString)

	chirpUUID, err := uuid.Parse(chirpIDString) // converts the string into a UUID
	if err != nil {
		respondWithError(w, 500, "UUID error")
		return
	}
	dbChirp, err := cfg.db.GetChirpByChirpUUID(context.Background(), chirpUUID)
	if err != nil {
		respondWithError(w, 404, "chirp not found")
		return
	}
	if dbChirp.UserID != userIDVerified {
		respondWithError(w, 403, "UserID does not match Chirp author userID")
		return
	}

	// verified user (by token) is confirmed creator of chirp, proced to attemp to remove

	err = cfg.db.DeleteChirpByChirpUUID(context.Background(), chirpUUID)
	if err != nil {
		respondWithError(w, 500, "err deleting chirp")
		return
	}

	w.WriteHeader(204)
}

func (cfg *apiConfig) getChirps(w http.ResponseWriter, req *http.Request) {

	authorID := req.URL.Query().Get("author_id")
	// s is a string that contains the value of the author_id query parameter
	// if it exists, or an empty string if it doesn't
	var chirpsSlice []database.Chirp

	if len(authorID) == 0 {
		chirpsSlice, err := cfg.db.GetChirps(context.Background())
		if err != nil {
			respondWithError(w, 500, "error retrieving chirps")
			return
		}
		if len(chirpsSlice) == 0 {
			respondWithError(w, 500, "no chirps found")
		}
	} else {
		authorUUID, err := uuid.Parse(authorID) // converts the string into a UUID
		if err != nil {
			respondWithError(w, 500, "UUID error")
			return
		}
		chirpsSlice, err = cfg.db.GetChirpsByAuthorID(context.Background(), authorUUID)
		if err != nil {
			respondWithError(w, 500, "error retrieving chirps")
			return
		}

	}

	var chirpsMainSlice []Chirp

	for _, chirp := range chirpsSlice {

		chirpsMainSlice = append(chirpsMainSlice, Chirp{
			ID:        chirp.ID,
			CreatedAt: chirp.CreatedAt,
			UpdatedAt: chirp.UpdatedAt,
			Body:      chirp.Body,
			UserID:    chirp.UserID,
		})

	}
	jsonWriter(w, 200, chirpsMainSlice)
}

func filterProfanity(body string) string {
	profanity := []string{"kerfuffle", "sharbert", "fornax"}
	replaceString := "****"

	wordSlice := strings.Split(body, " ")

	for i, word := range wordSlice {
		for _, profane := range profanity {
			if strings.ToLower(word) == profane {
				wordSlice[i] = replaceString // NEED TO USE INDEX! Otherwise, word is a *copy* of the value
			}
		}
	}

	return strings.Join(wordSlice, " ")
}

func respondWithError(w http.ResponseWriter, code int, msg string) {

	resp := errResponse{Error: msg}
	jsonWriter(w, code, resp)
}

func jsonWriter(w http.ResponseWriter, code int, payload interface{}) {

	jsonBytes, err := json.Marshal(payload)

	if err != nil {
		fmt.Printf("error marshalling response: %v\n", err)
		http.Error(w, "Internal server error", http.StatusInternalServerError) // auto handles setting header to 500 and body to error
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	w.Write(jsonBytes)
}
