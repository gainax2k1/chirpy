package main

import (
	"log"
	"net/http"
)

func main() {
	newMux := http.NewServeMux()
	newServer := http.Server{
		Addr:    ":8080",
		Handler: newMux,
	}
	err := newServer.ListenAndServe()
	if err != nil {
		log.Fatal(err)
	}
}
