package main

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
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
		httpClient := newMockHttpClient(&http.Client{})
		service := mockTargetService()

		proxy := NewProxy(httpClient)
		w := newMockResponseWriter()

		expectedBody := "test request body"
		r := httptest.NewRequest(http.MethodGet, "/proxy/"+service.URL, strings.NewReader(expectedBody))
		r.Header.Set("Authorization", "Bearer token123")
		r.Header.Set("Content-Type", "application/json")
		r.Header.Set("X-Custom-Header", "custom-value")

		proxy.ServeHTTP(w, r)

		for key := range r.Header {
			if r.Header.Get(key) != httpClient.capturedRequest.Header.Get(key) {
				t.Errorf("proxy http client's request %s doesn't match with incoming request", key)
			}
		}

		capturedBody, err := io.ReadAll(httpClient.capturedRequest.Body)
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
