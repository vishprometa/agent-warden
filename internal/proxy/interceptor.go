package proxy

import (
	"bytes"
	"fmt"
	"io"
	"net/http"
)

// CaptureRequestBody reads the entire request body and then restores it
// so that downstream handlers (including the reverse proxy) can read it again.
// Returns the captured body bytes and any read error.
func CaptureRequestBody(r *http.Request) ([]byte, error) {
	if r.Body == nil {
		return nil, nil
	}

	body, err := io.ReadAll(r.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read request body: %w", err)
	}
	r.Body.Close()

	// Restore the body so the reverse proxy can forward it.
	r.Body = io.NopCloser(bytes.NewReader(body))
	// Preserve the original content length.
	r.ContentLength = int64(len(body))

	return body, nil
}

// responseRecorder wraps an http.ResponseWriter to capture the response status
// code and body while still writing through to the underlying writer.
type responseRecorder struct {
	http.ResponseWriter
	statusCode int
	body       bytes.Buffer
	wroteHeader bool
}

// newResponseRecorder creates a new response recorder wrapping the given writer.
func newResponseRecorder(w http.ResponseWriter) *responseRecorder {
	return &responseRecorder{
		ResponseWriter: w,
		statusCode:     http.StatusOK,
	}
}

// WriteHeader captures the status code and delegates to the underlying writer.
func (r *responseRecorder) WriteHeader(code int) {
	if r.wroteHeader {
		return
	}
	r.wroteHeader = true
	r.statusCode = code
	r.ResponseWriter.WriteHeader(code)
}

// Write captures the response body and delegates to the underlying writer.
func (r *responseRecorder) Write(b []byte) (int, error) {
	// Capture a copy of the response body.
	r.body.Write(b)
	return r.ResponseWriter.Write(b)
}

// Flush delegates to the underlying writer if it supports flushing.
// This is required for SSE streaming to work correctly.
func (r *responseRecorder) Flush() {
	if flusher, ok := r.ResponseWriter.(http.Flusher); ok {
		flusher.Flush()
	}
}

// StatusCode returns the captured HTTP status code.
func (r *responseRecorder) StatusCode() int {
	return r.statusCode
}

// Body returns the captured response body.
func (r *responseRecorder) Body() []byte {
	return r.body.Bytes()
}

// Unwrap returns the underlying ResponseWriter for interface assertion.
func (r *responseRecorder) Unwrap() http.ResponseWriter {
	return r.ResponseWriter
}
