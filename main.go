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
}

type User struct {
	ID        uuid.UUID `json:"id"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
	Email     string    `json:"email"`
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

func main() {
	err := godotenv.Load()
	if err != nil {
		log.Fatal("Error loading .env file")
	}

	dbURL := os.Getenv("DB_URL")
	platform := os.Getenv("PLATFORM")
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

	// similar to above, but for URLs that start with "/assets"
	// -- my initial, not-quite-there implimentation:
	// mux.Handle("/assets", http.FileServer(http.Dir("./assets")))
	// -- this ONLY catches urls ending with ".../assets", and anything else like "../assets/chirp.png"
	//    WON'T be caught. The following is the robust version that handles it correctly

	// suggested, more robust implimentation:
	mux.Handle("/assets/", http.StripPrefix("/assets/", http.FileServer(http.Dir("./assets"))))

	// old: mux.HandleFunc("/healthz", readiness(http.ResponseWriter, *http.Request)) WRONG!
	// new:
	mux.HandleFunc("POST /admin/reset", cfg.middlewareMetricsHandlerReset)
	mux.HandleFunc("GET /api/healthz", readiness) // correct!
	mux.HandleFunc("GET /admin/metrics", cfg.middlewareMetricsStats)
	//mux.HandleFunc("POST /admin/reset", cfg.middlewareMetricsReset) //old reset that reset the page view counter
	//mux.HandleFunc("POST /api/validate_chirp", cfg.middlewareMetricsValidate) // old seperate validate case
	mux.HandleFunc("POST /api/chirps", cfg.middlewareMetricsCreateChirps)
	mux.HandleFunc("GET /api/chirps", cfg.middlewareMetricsGetChirps)
	mux.HandleFunc("POST /api/users", cfg.middlewareMetricsCreateUser)
	mux.HandleFunc("GET /api/chirps/{chirpID}", cfg.middlewareMetricsGetChirp)
	mux.HandleFunc("POST /api/login", cfg.middlewareMetricsLoginUser)

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
		// 403 Forbidden
		respondWithError(w, 403, "Forbidden")
		return
	}
	err := cfg.db.Reset(context.Background())
	if err != nil {
		respondWithError(w, 400, "Bad Request")
	}
	fmt.Printf("Database successfully reset.")
	//return
}

func (cfg *apiConfig) middlewareMetricsInc(next http.Handler) http.Handler {
	/*
		THIS DOESN'T WORK! it only runs ONCE at startup!
		- cfg.fileserverHits.Add(1) // should increment by 1 safely
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

func (cfg *apiConfig) middlewareMetricsCreateUser(w http.ResponseWriter, req *http.Request) {
	// DECODE JSON REQUEST BODY:

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
		ID:        newUserRecord.ID,
		CreatedAt: newUserRecord.CreatedAt,
		UpdatedAt: newUserRecord.UpdatedAt,
		Email:     newUserRecord.Email,
	}

	jsonWriter(w, 201, mainUser)
	//return
}

func (cfg *apiConfig) middlewareMetricsLoginUser(w http.ResponseWriter, req *http.Request) {

	// DECODE JSON REQUEST BODY:

	decoder := json.NewDecoder(req.Body)
	userLoginParams := CreateUserRequest{} // struct with email and password

	err := decoder.Decode(&userLoginParams)
	if err != nil {
		respondWithError(w, 500, "Error decoding params")
		return
	}
	dbUserRecord, err := cfg.db.GetUserByEmail(context.Background(), userLoginParams.Email)
	if err != nil {
		respondWithError(w, 401, "Unauthorized")
		return
	}
	err = auth.CheckPasswordHash(userLoginParams.Password, dbUserRecord.HashedPassword)
	if err != nil {
		respondWithError(w, 401, "Unauthorized")
		return
	}

	mainUser := User{ // converting to ensure security (not exposing sql field names, allows not returning specific values, like potential password, etc)
		ID:        dbUserRecord.ID,
		CreatedAt: dbUserRecord.CreatedAt,
		UpdatedAt: dbUserRecord.UpdatedAt,
		Email:     dbUserRecord.Email,
	}

	jsonWriter(w, 200, mainUser)
	//return

}

func (cfg *apiConfig) middlewareMetricsCreateChirps(w http.ResponseWriter, req *http.Request) {

	// DECODE JSON REQUEST BODY:

	decoder := json.NewDecoder(req.Body)
	params := CreateChirp{}

	err := decoder.Decode(&params)
	if err != nil {
		respondWithError(w, 500, "Error decoding params")
		return
	}

	// params is a struct with data populated successfully

	characterCount := len(params.Body)

	fmt.Printf("Character count using len: %v", characterCount)

	// ENCODE JSON RESPONSE BODY:

	if characterCount > 140 { //invalid case
		respondWithError(w, 400, "Chirp is too long")
		return
	}
	// At this point, CHIRP is good to go:
	var chirpParams database.CreateChirpParams
	chirpParams.Body = filterProfanity(params.Body) // not sure if we're still filtering, but this would be teh place to do so
	chirpParams.UserID = params.User_ID

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
	//return
}

func (cfg *apiConfig) middlewareMetricsGetChirp(w http.ResponseWriter, req *http.Request) {
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

func (cfg *apiConfig) middlewareMetricsGetChirps(w http.ResponseWriter, req *http.Request) {
	chirpsSlice, err := cfg.db.GetChirps(context.Background())
	if err != nil {
		respondWithError(w, 500, "error retrieving chirps")
		return
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
