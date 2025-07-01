package main

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"strings"
	"sync/atomic"

	"github.com/gainax2k1/chirpy/internal/database"

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

}

func main() {
	err := godotenv.Load()
	if err != nil {
		log.Fatal("Error loading .env file")
	}

	dbURL := os.Getenv("DB_URL")
	db, err := sql.Open("postgres", dbURL)
	if err != nil {
		fmt.Println("error opening sql: ", err)
		os.Exit(1)

	}
	defer db.Close()

	dbQueries := database.New(db)

	cfg := &apiConfig{
		db: dbQueries,
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
	mux.HandleFunc("GET /api/healthz", readiness) // correct!
	mux.HandleFunc("GET /admin/metrics", cfg.middlewareMetricsStats)
	mux.HandleFunc("POST /admin/reset", cfg.middlewareMetricsReset)
	mux.HandleFunc("POST /api/validate_chirp", cfg.middlewareMetricsValidate)

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

func (cfg *apiConfig) middlewareMetricsReset(w http.ResponseWriter, req *http.Request) {
	w.Header().Set("Content-Type", "text/plain; charset=utf-8") // normal header
	w.WriteHeader(200)                                          // status code
	cfg.fileserverHits.Store(0)
	returnHits := fmt.Sprint("Hits reset: ", cfg.fileserverHits.Load())
	w.Write([]byte(returnHits)) // << expects []byte, so type convert to have "OK" (for now)

}

type parameters struct {
	// these tags indicate how the keys in the JSON should be mapped to the struct fields
	// the struct fields must be exported (start with a capital letter) if you want them parsed
	Body string `json:"body"`
}

type response struct {
	Valid bool `json:"valid"`
}

type errResponse struct {
	Error string `json:"error"`
}

type cleanResponse struct {
	Clean string `json:"cleaned_body"`
}

func (cfg *apiConfig) middlewareMetricsValidate(w http.ResponseWriter, req *http.Request) { // ******

	// DECODE JSON REQUEST BODY:

	decoder := json.NewDecoder(req.Body)
	params := parameters{}

	err := decoder.Decode(&params)
	if err != nil {
		respondWithError(w, 500, "Error decoding params")
		return
	}

	// params is a struct with data populated successfully

	characterCount := len(params.Body)

	fmt.Printf("Character count using len: %v", characterCount)
	// ABove  this is correct/working

	// ENCODE JSON RESPONSE BODY:

	if characterCount > 140 { //invalid case
		respondWithError(w, 400, "Chirp is too long")
		return
	}

	respondClean(w, filterProfanity(params.Body))
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

func respondWithValid(w http.ResponseWriter) {
	resp := response{Valid: true}
	jsonWriter(w, 200, resp)
}

func respondClean(w http.ResponseWriter, cleanBody string) {
	resp := cleanResponse{Clean: cleanBody}
	jsonWriter(w, 200, resp)
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
