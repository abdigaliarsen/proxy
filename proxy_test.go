package main

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestProxy(t *testing.T) {
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
			expectedBody: "expected body",
		},
		{
			name:         "redirect service",
			proxy:        NewProxy(&http.Client{}),
			service:      mockRedirectService(),
			expectCode:   http.StatusOK,
			expectedBody: "expected body",
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
