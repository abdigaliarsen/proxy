package main

import (
	"io"
	"net/http"
	"strings"
)

type HttpClient interface {
	Do(req *http.Request) (*http.Response, error)
}

type Proxy struct {
	cli HttpClient
}

func NewProxy(httpClient HttpClient) *Proxy {
	return &Proxy{
		cli: httpClient,
	}
}

func (p *Proxy) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	proxyKeyPos := strings.Index(r.URL.Path, "/proxy/")
	url := r.URL.Path[proxyKeyPos+len("/proxy/"):]

	req, err := http.NewRequestWithContext(r.Context(), r.Method, url, r.Body)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	for key, header := range r.Header {
		req.Header[key] = header
	}

	resp, err := p.cli.Do(req)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.WriteHeader(resp.StatusCode)
	if _, err = io.Copy(w, resp.Body); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
}
