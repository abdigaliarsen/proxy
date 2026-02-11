package main

import (
	"bytes"
	"io"
	"net/http"
	"net/http/httptest"
)

const mockExpectedResponseBody = "<>expected response body</>"

type mockRoundTripper struct {
	base            http.RoundTripper
	capturedRequest *http.Request
}

func newMockRoundTripper() *mockRoundTripper {
	return &mockRoundTripper{
		base: http.DefaultTransport,
	}
}

func (c *mockRoundTripper) RoundTrip(req *http.Request) (*http.Response, error) {
	cloned := req.Clone(req.Context())

	if req.Body != nil {
		bodyBytes, _ := io.ReadAll(req.Body)
		req.Body.Close()

		req.Body = io.NopCloser(bytes.NewReader(bodyBytes))

		cloned.Body = io.NopCloser(bytes.NewReader(bodyBytes))
	}

	c.capturedRequest = cloned

	return c.base.RoundTrip(req)
}

type mockResponseWriter struct {
	*httptest.ResponseRecorder
	buffer *bytes.Buffer
	status int
}

func newMockResponseWriter() *mockResponseWriter {
	return &mockResponseWriter{
		ResponseRecorder: httptest.NewRecorder(),
		buffer:           bytes.NewBuffer(nil),
		status:           http.StatusTeapot,
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
			w.Write([]byte(mockExpectedResponseBody))
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

func mockNestedRedirectService() *httptest.Server {
	target := mockTargetService()
	redirect := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, target.URL, http.StatusMovedPermanently)
	}))

	finalServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, redirect.URL, http.StatusMovedPermanently)
	}))

	return finalServer
}
