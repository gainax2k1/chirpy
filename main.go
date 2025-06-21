package main

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"sync/atomic"
)

type apiConfig struct {
	fileserverHits atomic.Int32
	/*
		The atomic.Int32 type is a really cool standard-library type that allows us
		to safely increment and read an integer value across multiple goroutines
		(HTTP requests).
	*/
}

func main() {
	// This creates a "multiplexer"—a router for incoming HTTP requests.
	// It decides which handler should process requests for different URL paths.
	mux := http.NewServeMux()
	var cfg apiConfig

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
	err := newServer.ListenAndServe()
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
		cfg.fileserverHits.Add(1) // should increment by 1 safely
		return next
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

func (cfg *apiConfig) middlewareMetricsValidate(w http.ResponseWriter, req *http.Request) { // ******
	// all copied, don't trust below

	// DECODE JSON REQUEST BODY:

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

	decoder := json.NewDecoder(req.Body)
	params := parameters{}

	err := decoder.Decode(&params)
	if err != nil {

		log.Printf("Error decoding parameters: %s", err)

		// Create an error response and send it back
		errorResp := errResponse{Error: "Invalid JSON"}
		jsonBytes, marshalErr := json.Marshal(errorResp)
		if marshalErr != nil {
			// If we can't even marshal the error, just send a plain 500
			w.WriteHeader(500)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(400) // or 500, depending on what you prefer
		w.Write(jsonBytes)
		return
	}

	// params is a struct with data populated successfully

	characterCount := len(params.Body)

	fmt.Printf("Character count using len: %v", characterCount)
	// ABove  this is correct/working

	// ENCODE JSON RESPONSE BODY:

	if characterCount > 140 { //invalid case
		resp := errResponse{Error: "Chirp is too long"}
		jsonBytes, err := json.Marshal(resp)
		if err != nil {
			fmt.Printf("error marshalling over 140: %v\n", err)
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(400)
		w.Write(jsonBytes)
		return
	}

	resp := response{Valid: true}
	jsonBytes, err := json.Marshal(resp)
	if err != nil {
		fmt.Printf("eror marshalling valid: %v\n", err)
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(200)
	w.Write(jsonBytes)
	return

}
