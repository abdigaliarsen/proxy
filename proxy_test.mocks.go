package main

import (
	"bytes"
	"net/http"
	"net/http/httptest"
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
