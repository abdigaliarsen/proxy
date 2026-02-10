package main

import (
	"io"
	"net/http"
	"strings"
)

type HttpClient interface {
	Get(url string) (resp *http.Response, err error)
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

	resp, err := p.cli.Get(url)
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
