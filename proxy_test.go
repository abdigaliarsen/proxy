package main

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

type mockResponseWriter struct {
	*httptest.ResponseRecorder
	buffer *bytes.Buffer
	status int
}

func newMockResponseWriter() *mockResponseWriter {
	return &mockResponseWriter{
		ResponseRecorder: httptest.NewRecorder(),
		buffer:           new(bytes.Buffer),
		status:           http.StatusOK,
	}
}

func (m *mockResponseWriter) Write(b []byte) (int, error) {
	m.buffer.Write(b)
	return m.ResponseRecorder.Write(b)
}

func (m *mockResponseWriter) WriteHeader(code int) {
	m.status = code
	m.ResponseRecorder.WriteHeader(code)
}

func mockTargetService() *httptest.Server {
	return httptest.NewServer(
		http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusOK)
			w.Write([]byte("expected body"))
		}),
	)
}

func mockRedirectService() *httptest.Server {
	targetService := mockTargetService()

	return httptest.NewServer(
		http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			http.Redirect(w, r, targetService.URL, http.StatusMovedPermanently)
		}),
	)
}

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
