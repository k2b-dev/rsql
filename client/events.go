package rsql

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
)

// EventsClient opens typed namespace event streams.
type EventsClient struct {
	database *DatabaseClient
}

// Subscribe opens an SSE subscription. The caller must close the stream.
func (e EventsClient) Subscribe(ctx context.Context, options SubscribeOptions) (*EventStream, error) {
	query := make(url.Values)
	if len(options.Tables) > 0 {
		for _, table := range options.Tables {
			if err := validateIdentifier("table", table); err != nil {
				return nil, err
			}
		}
		query.Set("tables", joinTables(options.Tables))
	}
	headers := make(http.Header)
	headers.Set("Accept", "text/event-stream")
	response, err := e.database.client.doStream(
		ctx,
		http.MethodGet,
		e.database.path("/subscribe"),
		query,
		nil,
		-1,
		headers,
	)
	if err != nil {
		return nil, err
	}
	scanner := bufio.NewScanner(response.Body)
	scanner.Buffer(make([]byte, 4096), 4<<20)
	return &EventStream{
		response: response,
		scanner:  scanner,
	}, nil
}

// EventStream is a pull-based SSE iterator without hidden goroutines.
type EventStream struct {
	response *StreamResponse
	scanner  *bufio.Scanner
	event    SSEEvent
	err      error
	closed   bool
}

// Next blocks until the next event is available or the stream ends.
func (s *EventStream) Next() bool {
	if s.closed || s.err != nil {
		return false
	}
	var data bytes.Buffer
	for s.scanner.Scan() {
		line := strings.TrimSuffix(s.scanner.Text(), "\r")
		if line == "" {
			if data.Len() == 0 {
				continue
			}
			payload := bytes.TrimSuffix(data.Bytes(), []byte{'\n'})
			if err := json.Unmarshal(payload, &s.event); err != nil {
				s.err = fmt.Errorf("decode SSE event: %w", err)
				return false
			}
			return true
		}
		if strings.HasPrefix(line, ":") {
			continue
		}
		if value, ok := strings.CutPrefix(line, "data:"); ok {
			data.WriteString(strings.TrimPrefix(value, " "))
			data.WriteByte('\n')
		}
	}
	if err := s.scanner.Err(); err != nil && !errors.Is(err, io.EOF) {
		s.err = fmt.Errorf("read SSE stream: %w", err)
	}
	return false
}

// Event returns the most recent event. It is valid until the next Next call.
func (s *EventStream) Event() SSEEvent {
	return s.event
}

// Header returns the subscription response headers.
func (s *EventStream) Header() http.Header {
	return s.response.Header.Clone()
}

// Err returns the terminal stream error, if any.
func (s *EventStream) Err() error {
	return s.err
}

// Close closes the underlying HTTP response body.
func (s *EventStream) Close() error {
	if s.closed {
		return nil
	}
	s.closed = true
	return s.response.Body.Close()
}
