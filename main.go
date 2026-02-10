package main

import (
	"net/http"
	"time"
)

func main() {
	proxy := NewProxy(&http.Client{
		Timeout: 10 * time.Second,
	})

	http.Handle("/proxy/", proxy)
	if err := http.ListenAndServe(":8080", nil); err != nil {
		return
	}
}
