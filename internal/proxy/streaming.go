package proxy

import (
	"bufio"
	"bytes"
	"fmt"
	"io"
	"log/slog"
	"net/http"
)

// SSEStreamer handles Server-Sent Events (SSE) streaming passthrough from
// upstream LLM providers to downstream clients. It forwards each SSE event
// in real time while accumulating the full response body for trace storage.
type SSEStreamer struct {
	logger *slog.Logger
}

// NewSSEStreamer creates a new SSE streaming handler.
func NewSSEStreamer(logger *slog.Logger) *SSEStreamer {
	if logger == nil {
		logger = slog.Default()
	}
	return &SSEStreamer{
		logger: logger.With("component", "proxy.SSEStreamer"),
	}
}

// StreamSSE reads the upstream SSE response line by line, forwarding each
// event to the client in real time. It returns the accumulated complete
// response body for trace storage.
//
// The caller is responsible for setting appropriate SSE headers on the
// ResponseWriter before calling this method. This method will flush after
// each complete SSE event (blank line delimiter).
//
// Returns the accumulated body bytes and any error encountered during streaming.
func (s *SSEStreamer) StreamSSE(w http.ResponseWriter, upstreamResp *http.Response) ([]byte, error) {
	// Verify the ResponseWriter supports flushing.
	flusher, ok := w.(http.Flusher)
	if !ok {
		return nil, fmt.Errorf("response writer does not support flushing")
	}

	// Set SSE headers.
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("X-Accel-Buffering", "no") // Disable nginx buffering

	// Copy through any relevant upstream headers.
	for _, header := range []string{"X-Request-Id", "X-Ratelimit-Remaining-Tokens", "X-Ratelimit-Remaining-Requests"} {
		if val := upstreamResp.Header.Get(header); val != "" {
			w.Header().Set(header, val)
		}
	}

	w.WriteHeader(upstreamResp.StatusCode)

	var accumulated bytes.Buffer
	scanner := bufio.NewScanner(upstreamResp.Body)

	// Increase scanner buffer for large SSE events (e.g., long completions).
	const maxTokenSize = 1024 * 1024 // 1 MB
	scanner.Buffer(make([]byte, 0, 64*1024), maxTokenSize)

	var eventBuf bytes.Buffer
	eventCount := 0

	for scanner.Scan() {
		line := scanner.Bytes()

		// Write the line to the accumulated buffer for trace storage.
		accumulated.Write(line)
		accumulated.WriteByte('\n')

		// Write the line to the client.
		if _, err := w.Write(line); err != nil {
			s.logger.Debug("client disconnected during SSE stream",
				"events_sent", eventCount,
				"error", err,
			)
			// Drain the rest of the upstream response to avoid connection issues.
			io.Copy(io.Discard, upstreamResp.Body)
			return accumulated.Bytes(), fmt.Errorf("client write failed: %w", err)
		}
		if _, err := w.Write([]byte("\n")); err != nil {
			io.Copy(io.Discard, upstreamResp.Body)
			return accumulated.Bytes(), fmt.Errorf("client write failed: %w", err)
		}

		eventBuf.Write(line)
		eventBuf.WriteByte('\n')

		// SSE events are delimited by blank lines. When we see one, flush
		// the buffered data to the client immediately.
		if len(line) == 0 {
			flusher.Flush()
			eventCount++
			eventBuf.Reset()
		}
	}

	if err := scanner.Err(); err != nil {
		s.logger.Error("SSE stream scanner error",
			"events_sent", eventCount,
			"error", err,
		)
		return accumulated.Bytes(), fmt.Errorf("SSE stream read error: %w", err)
	}

	// Final flush in case the stream ended without a trailing blank line.
	flusher.Flush()

	s.logger.Debug("SSE stream completed",
		"events_sent", eventCount,
		"total_bytes", accumulated.Len(),
	)

	return accumulated.Bytes(), nil
}

// IsSSEResponse checks whether an HTTP response is a Server-Sent Events stream
// by inspecting the Content-Type header.
func IsSSEResponse(resp *http.Response) bool {
	ct := resp.Header.Get("Content-Type")
	return ct == "text/event-stream" ||
		// Some providers include charset or other parameters.
		len(ct) > 17 && ct[:17] == "text/event-stream"
}
