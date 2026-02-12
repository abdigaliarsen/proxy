package main

import (
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
)

func main() {
	proxy := NewProxy(&http.Client{
		Timeout: 10 * time.Second,
	})

	router := chi.NewRouter()
	router.Handle("/proxy/*", proxy)

	if err := http.ListenAndServe(":8080", router); err != nil {
		return
	}
}
