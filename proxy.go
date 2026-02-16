package main

import (
	"fmt"
	"io"
	"math/rand"
	"net/http"
	"net/http/cookiejar"
	"strings"
	"sync"
)

const proxySessionCookie = "proxy-session-id"

type Proxy struct {
	cli        *http.Client
	cookieJars map[string]*cookiejar.Jar
	mu         sync.RWMutex
}

func NewProxy(httpClient *http.Client) *Proxy {
	return &Proxy{
		cli:        httpClient,
		cookieJars: make(map[string]*cookiejar.Jar),
	}
}

func (p *Proxy) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	proxyKeyPos := strings.Index(r.URL.Path, "/proxy/")
	if proxyKeyPos == -1 {
		http.Error(w, "invalid proxy path", http.StatusBadRequest)
		return
	}

	proxyUrl := r.URL.Path[proxyKeyPos+len("/proxy/"):]
	if r.URL.RawQuery != "" {
		proxyUrl += "?" + r.URL.RawQuery
	}

	req, err := http.NewRequestWithContext(r.Context(), r.Method, proxyUrl, r.Body)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	for key, header := range r.Header {
		req.Header[key] = header
	}

	session := p.getOrCreateSession(w, r)

	p.mu.RLock()
	jar := p.cookieJars[session]
	p.mu.RUnlock()

	if jar == nil {
		newJar, _ := cookiejar.New(&cookiejar.Options{})
		p.mu.Lock()
		if p.cookieJars[session] == nil {
			p.cookieJars[session] = newJar
			jar = newJar
		} else {
			jar = p.cookieJars[session]
		}
		p.mu.Unlock()
	}

	sessionClient := &http.Client{
		Transport:     p.cli.Transport,
		Jar:           jar,
		CheckRedirect: p.cli.CheckRedirect,
		Timeout:       p.cli.Timeout,
	}

	resp, err := sessionClient.Do(req)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	defer resp.Body.Close()

	for key, headers := range resp.Header {
		if strings.EqualFold(key, "Set-Cookie") {
			continue
		}

		for _, header := range headers {
			w.Header().Add(key, header)
		}
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.WriteHeader(resp.StatusCode)
	w.Write(body)
}

func (p *Proxy) getOrCreateSession(w http.ResponseWriter, r *http.Request) string {
	if cookie, err := r.Cookie(proxySessionCookie); err == nil {
		return cookie.Value
	}

	sessionID := fmt.Sprintf("%d", rand.Uint64())
	http.SetCookie(w, &http.Cookie{
		Name:     proxySessionCookie,
		Value:    sessionID,
		HttpOnly: true,
		Secure:   true,
		SameSite: http.SameSiteStrictMode,
		MaxAge:   3600,
	})

	return sessionID
}
