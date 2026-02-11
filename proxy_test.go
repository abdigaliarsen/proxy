package main

import (
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func TestProxyRedirect(t *testing.T) {
	testCases := []struct {
		name         string
		proxy        *Proxy
		service      *httptest.Server
		expectCode   int
		expectedBody string
	}{
		{
			name:         "request proxy",
			proxy:        NewProxy(&http.Client{}),
			service:      mockTargetService(),
			expectCode:   http.StatusOK,
			expectedBody: mockExpectedResponseBody,
		},
		{
			name:         "redirect service",
			proxy:        NewProxy(&http.Client{}),
			service:      mockRedirectService(),
			expectCode:   http.StatusOK,
			expectedBody: mockExpectedResponseBody,
		},
		{
			name:         "nested redirect service",
			proxy:        NewProxy(&http.Client{}),
			service:      mockNestedRedirectService(),
			expectCode:   http.StatusOK,
			expectedBody: mockExpectedResponseBody,
		},
		{
			name: "no proxy redirect",
			proxy: NewProxy(&http.Client{
				Timeout: 5 * time.Second,
				CheckRedirect: func(req *http.Request, via []*http.Request) error {
					return http.ErrUseLastResponse
				},
			}),
			service:      mockRedirectService(),
			expectCode:   http.StatusMovedPermanently,
			expectedBody: "Moved Permanently",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			w := newMockResponseWriter()
			r := httptest.NewRequest(http.MethodGet, "/proxy/"+tc.service.URL, nil)

			tc.proxy.ServeHTTP(w, r)

			if w.Code != tc.expectCode {
				t.Errorf("expected status code %d, got %d", tc.expectCode, w.Code)
			}

			if !strings.Contains(w.buffer.String(), tc.expectedBody) {
				t.Errorf("expected body %q, got %q", tc.expectedBody, w.Body.Bytes())
			}
		})
	}
}

func TestProxyHeaders(t *testing.T) {
	t.Run("set headers/body to proxy request", func(t *testing.T) {
		service := mockTargetService()
		transport := newMockRoundTripper()

		proxy := NewProxy(&http.Client{
			Transport: transport,
		})
		w := newMockResponseWriter()

		expectedBody := "test request body"
		r := httptest.NewRequest(http.MethodGet, "/proxy/"+service.URL, strings.NewReader(expectedBody))
		r.Header.Set("Authorization", "Bearer token123")
		r.Header.Set("Content-Type", "application/json")
		r.Header.Set("X-Custom-Header", "custom-value")

		proxy.ServeHTTP(w, r)

		for key := range r.Header {
			if r.Header.Get(key) != transport.capturedRequest.Header.Get(key) {
				t.Errorf("proxy http client's request %s doesn't match with incoming request", key)
			}
		}

		capturedBody, err := io.ReadAll(transport.capturedRequest.Body)
		if err != nil {
			t.Fatalf("failed to read captured request body: %v", err)
		}

		if string(capturedBody) != expectedBody {
			t.Errorf("proxy http client's request body doesn't match: expected %q, got %q", expectedBody, string(capturedBody))
		}

		if w.Code != http.StatusOK {
			t.Errorf("expected status code %d, got %d", http.StatusOK, w.Code)
		}

		if !strings.Contains(w.buffer.String(), mockExpectedResponseBody) {
			t.Errorf("expected body %q, got %q", mockExpectedResponseBody, w.buffer.String())
		}
	})
}

func TestProxyCookies(t *testing.T) {
	t.Run("keep session dependent cookies for multiple request", func(t *testing.T) {
		requestCount := 0
		var receivedCookies []*http.Cookie
		service := httptest.NewServer(
			http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				requestCount++
				receivedCookies = r.Cookies()

				if requestCount == 1 {
					http.SetCookie(w, &http.Cookie{
						Name:  "session-token",
						Value: "abc123",
						Path:  "/",
					})
					http.SetCookie(w, &http.Cookie{
						Name:  "user-id",
						Value: "user456",
						Path:  "/",
					})
					w.WriteHeader(http.StatusOK)
					w.Write([]byte("first request"))
				} else {
					w.WriteHeader(http.StatusOK)
					w.Write([]byte("subsequent request"))
				}
			}),
		)
		defer service.Close()

		proxy := NewProxy(&http.Client{})

		w1 := newMockResponseWriter()
		r1 := httptest.NewRequest(http.MethodGet, "/proxy/"+service.URL, nil)

		proxy.ServeHTTP(w1, r1)

		sessionCookie := extractSessionCookie(w1.ResponseRecorder)
		if sessionCookie == nil {
			t.Fatal("expected proxy session cookie to be set")
		}
		sessionID := sessionCookie.Value

		expectedCookies := map[string]string{
			"session-token": "abc123",
			"user-id":       "user456",
		}

		for i := 2; i <= 5; i++ {
			w := newMockResponseWriter()
			r := httptest.NewRequest(http.MethodGet, "/proxy/"+service.URL, nil)
			r.AddCookie(&http.Cookie{
				Name:  proxySessionCookie,
				Value: sessionID,
			})

			receivedCookies = nil
			proxy.ServeHTTP(w, r)

			if len(receivedCookies) == 0 {
				t.Errorf("request %d: expected cookies from first request to be sent", i)
				continue
			}

			receivedCookieMap := make(map[string]string)
			for _, cookie := range receivedCookies {
				receivedCookieMap[cookie.Name] = cookie.Value
			}

			for name, expectedValue := range expectedCookies {
				if actualValue, found := receivedCookieMap[name]; !found {
					t.Errorf("request %d: expected cookie %s to be sent", i, name)
				} else if actualValue != expectedValue {
					t.Errorf("request %d: expected cookie %s to have value %q, got %q", i, name, expectedValue, actualValue)
				}
			}

			if w.Code != http.StatusOK {
				t.Errorf("request %d: expected status code %d, got %d", i, http.StatusOK, w.Code)
			}
		}
	})
}

func extractSessionCookie(w *httptest.ResponseRecorder) *http.Cookie {
	for _, c := range w.Result().Cookies() {
		if c.Name == proxySessionCookie {
			return c
		}
	}
	return nil
}

func TestProxyConcurrentSameSession(t *testing.T) {
	t.Run("concurrent requests with same session", func(t *testing.T) {
		requestCount := atomic.Int64{}
		cookieReceivedCount := atomic.Int64{}

		service := httptest.NewServer(
			http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				count := requestCount.Add(1)
				cookies := r.Cookies()

				if len(cookies) > 0 {
					cookieReceivedCount.Add(1)
				}

				if count == 1 {
					http.SetCookie(w, &http.Cookie{
						Name:  "session-token",
						Value: "concurrent-test",
						Path:  "/",
					})
				}
				w.WriteHeader(http.StatusOK)
				w.Write([]byte("ok"))
			}),
		)
		defer service.Close()

		proxy := NewProxy(&http.Client{})

		w1 := newMockResponseWriter()
		r1 := httptest.NewRequest(http.MethodGet, "/proxy/"+service.URL, nil)
		proxy.ServeHTTP(w1, r1)

		sessionCookie := extractSessionCookie(w1.ResponseRecorder)
		if sessionCookie == nil {
			t.Fatal("expected proxy session cookie to be set")
		}
		sessionID := sessionCookie.Value

		const numRequests = 100
		var wg sync.WaitGroup
		wg.Add(numRequests)

		for i := 0; i < numRequests; i++ {
			go func() {
				defer wg.Done()
				w := newMockResponseWriter()
				r := httptest.NewRequest(http.MethodGet, "/proxy/"+service.URL, nil)
				r.AddCookie(&http.Cookie{
					Name:  proxySessionCookie,
					Value: sessionID,
				})
				proxy.ServeHTTP(w, r)

				if w.Code != http.StatusOK {
					t.Errorf("expected status code %d, got %d", http.StatusOK, w.Code)
				}
			}()
		}

		wg.Wait()

		if cookieReceivedCount.Load() == 0 {
			t.Error("expected cookies to be sent in concurrent requests")
		}
	})
}

func TestProxyConcurrentDifferentSessions(t *testing.T) {
	t.Run("concurrent requests with different sessions", func(t *testing.T) {
		sessionCookies := make(map[string]int)
		var mu sync.Mutex

		service := httptest.NewServer(
			http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				cookies := r.Cookies()
				for _, cookie := range cookies {
					if cookie.Name == "session-data" {
						mu.Lock()
						sessionCookies[cookie.Value]++
						mu.Unlock()
					}
				}

				http.SetCookie(w, &http.Cookie{
					Name:  "session-data",
					Value: r.URL.Query().Get("session"),
					Path:  "/",
				})
				w.WriteHeader(http.StatusOK)
				w.Write([]byte("ok"))
			}),
		)
		defer service.Close()

		proxy := NewProxy(&http.Client{})

		const numSessions = 50
		const requestsPerSession = 10
		var wg sync.WaitGroup
		wg.Add(numSessions * requestsPerSession)

		for sessionNum := 0; sessionNum < numSessions; sessionNum++ {
			go func(sessionNum int) {
				w1 := newMockResponseWriter()
				r1 := httptest.NewRequest(http.MethodGet, "/proxy/"+service.URL+"?session="+fmt.Sprintf("%d", sessionNum), nil)
				proxy.ServeHTTP(w1, r1)

				sessionCookie := extractSessionCookie(w1.ResponseRecorder)
				if sessionCookie == nil {
					t.Errorf("session %d: expected proxy session cookie to be set", sessionNum)
					return
				}
				sessionID := sessionCookie.Value

				for i := 0; i < requestsPerSession; i++ {
					go func() {
						defer wg.Done()
						w := newMockResponseWriter()
						r := httptest.NewRequest(http.MethodGet, "/proxy/"+service.URL+"?session="+fmt.Sprintf("%d", sessionNum), nil)
						r.AddCookie(&http.Cookie{
							Name:  proxySessionCookie,
							Value: sessionID,
						})
						proxy.ServeHTTP(w, r)

						if w.Code != http.StatusOK {
							t.Errorf("session %d: expected status code %d, got %d", sessionNum, http.StatusOK, w.Code)
						}
					}()
				}
			}(sessionNum)
		}

		wg.Wait()

		mu.Lock()
		if len(sessionCookies) == 0 {
			t.Error("expected cookies from different sessions to be isolated")
		}
		mu.Unlock()
	})
}

func TestProxyMemoryLeakManySessions(t *testing.T) {
	t.Run("memory leak test - many sessions", func(t *testing.T) {
		service := httptest.NewServer(
			http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(http.StatusOK)
				w.Write([]byte("ok"))
			}),
		)
		defer service.Close()

		proxy := NewProxy(&http.Client{})

		const numSessions = 1000
		sessionIDs := make([]string, 0, numSessions)

		for i := 0; i < numSessions; i++ {
			w := newMockResponseWriter()
			r := httptest.NewRequest(http.MethodGet, "/proxy/"+service.URL, nil)
			proxy.ServeHTTP(w, r)

			sessionCookie := extractSessionCookie(w.ResponseRecorder)
			if sessionCookie == nil {
				t.Fatalf("session %d: expected proxy session cookie to be set", i)
			}
			sessionIDs = append(sessionIDs, sessionCookie.Value)

			if w.Code != http.StatusOK {
				t.Errorf("session %d: expected status code %d, got %d", i, http.StatusOK, w.Code)
			}
		}

		proxy.mu.RLock()
		cookieJarCount := len(proxy.cookieJars)
		proxy.mu.RUnlock()

		if cookieJarCount != numSessions {
			t.Errorf("expected %d cookie jars, got %d", numSessions, cookieJarCount)
		}

		uniqueSessions := make(map[string]bool)
		for _, id := range sessionIDs {
			uniqueSessions[id] = true
		}

		if len(uniqueSessions) != numSessions {
			t.Errorf("expected %d unique sessions, got %d", numSessions, len(uniqueSessions))
		}

		for i := 0; i < 100; i++ {
			w := newMockResponseWriter()
			r := httptest.NewRequest(http.MethodGet, "/proxy/"+service.URL, nil)
			r.AddCookie(&http.Cookie{
				Name:  proxySessionCookie,
				Value: sessionIDs[i],
			})
			proxy.ServeHTTP(w, r)

			if w.Code != http.StatusOK {
				t.Errorf("reuse session %d: expected status code %d, got %d", i, http.StatusOK, w.Code)
			}
		}

		proxy.mu.RLock()
		finalCookieJarCount := len(proxy.cookieJars)
		proxy.mu.RUnlock()

		if finalCookieJarCount != numSessions {
			t.Errorf("after reuse: expected %d cookie jars, got %d", numSessions, finalCookieJarCount)
		}
	})
}
