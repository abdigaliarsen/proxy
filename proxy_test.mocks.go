package main

import (
	"bytes"
	"io"
	"net/http"
	"net/http/httptest"
)

const mockExpectedResponseBody = "<>expected response body</>"

type mockHttpClient struct {
	HttpClient
	capturedRequest *http.Request
}

func newMockHttpClient(client HttpClient) *mockHttpClient {
	return &mockHttpClient{
		HttpClient: client,
	}
}

func (c *mockHttpClient) Do(req *http.Request) (*http.Response, error) {
	cloned := req.Clone(req.Context())

	if req.Body != nil {
		bodyBytes, _ := io.ReadAll(req.Body)
		req.Body.Close()

		req.Body = io.NopCloser(bytes.NewReader(bodyBytes))

		cloned.Body = io.NopCloser(bytes.NewReader(bodyBytes))
	}

	c.capturedRequest = cloned
	return c.HttpClient.Do(req)
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
	targetService := mockTargetService()
	redirectService1 := httptest.NewServer(
		http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			http.Redirect(w, r, targetService.URL, http.StatusMovedPermanently)
		}),
	)
	redirectService2 := httptest.NewServer(
		http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			http.Redirect(w, r, redirectService1.URL, http.StatusMovedPermanently)
		}),
	)
	return httptest.NewServer(
		http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			http.Redirect(w, r, redirectService2.URL, http.StatusMovedPermanently)
		}),
	)
}
