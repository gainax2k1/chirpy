package main

import (
	"log"
	"net/http"
)

func main() {
	mux := http.NewServeMux()
	newServer := http.Server{
		Addr:    ":8080",
		Handler: mux,
	}

	mux.Handle("/", http.FileServer(http.Dir(".")))
	mux.Handle("/assets", http.FileServer(http.Dir("./assets")))

	err := newServer.ListenAndServe()
	if err != nil {
		log.Fatal(err)
	}

}
