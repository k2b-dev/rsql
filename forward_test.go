package rsql

import (
	"bytes"
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"
)

func TestDatabaseRoundTripIsScopedAndStreamsBody(t *testing.T) {
	var received bytes.Buffer
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if request.URL.Path != "/v1/tenant/tables/items/rows" {
			t.Errorf("path = %q", request.URL.Path)
		}
		if request.URL.RawQuery != "status=eq.active&limit=10" {
			t.Errorf("query = %q", request.URL.RawQuery)
		}
		if got := request.Header.Get("Authorization"); got != "Bearer secret" {
			t.Errorf("authorization = %q", got)
		}
		if got := request.Header.Get("Cookie"); got != "" {
			t.Errorf("cookie was forwarded: %q", got)
		}
		if got := request.Header.Get("Prefer"); got != "return=representation" {
			t.Errorf("prefer = %q", got)
		}
		_, _ = io.Copy(&received, request.Body)
		writer.Header().Set("Content-Type", "application/json")
		writer.WriteHeader(http.StatusCreated)
		_, _ = io.WriteString(writer, `{"inserted":1}`)
	}))
	defer server.Close()

	client := mustClient(t, server.URL)
	database, err := client.Database("tenant")
	if err != nil {
		t.Fatal(err)
	}
	incoming := httptest.NewRequest(
		http.MethodPost,
		"http://gateway/database?status=eq.active&limit=10",
		strings.NewReader(`{"name":"Ada"}`),
	)
	incoming.Header.Set("Authorization", "Bearer caller-token")
	incoming.Header.Set("Cookie", "session=private")
	incoming.Header.Set("Prefer", "return=representation")

	response, err := database.RoundTrip(context.Background(), incoming, DatabaseRoute{
		Path: "/tables/items/rows",
	})
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusCreated {
		t.Fatalf("status = %d", response.StatusCode)
	}
	if received.String() != `{"name":"Ada"}` {
		t.Fatalf("body = %q", received.String())
	}
}

func TestDatabaseRoundTripDoesNotBufferRequestBody(t *testing.T) {
	firstChunkReceived := make(chan struct{})
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		buffer := make([]byte, 5)
		if _, err := io.ReadFull(request.Body, buffer); err != nil {
			t.Errorf("read first chunk: %v", err)
		}
		close(firstChunkReceived)
		_, _ = io.Copy(io.Discard, request.Body)
		writer.WriteHeader(http.StatusNoContent)
	}))
	defer server.Close()

	client := mustClient(t, server.URL)
	database, err := client.Database("tenant")
	if err != nil {
		t.Fatal(err)
	}
	reader, writer := io.Pipe()
	incoming := httptest.NewRequest(http.MethodPost, "/database/tables/items/rows", reader)
	incoming.ContentLength = -1

	responses := make(chan *http.Response, 1)
	errs := make(chan error, 1)
	go func() {
		response, err := database.RoundTrip(context.Background(), incoming, DatabaseRoute{
			Path: "/tables/items/rows",
		})
		responses <- response
		errs <- err
	}()

	if _, err := writer.Write([]byte("first")); err != nil {
		t.Fatal(err)
	}
	select {
	case <-firstChunkReceived:
	case <-time.After(time.Second):
		t.Fatal("upstream did not receive data before request body completed")
	}
	if _, err := writer.Write([]byte("-second")); err != nil {
		t.Fatal(err)
	}
	_ = writer.Close()
	if err := <-errs; err != nil {
		t.Fatal(err)
	}
	response := <-responses
	if response != nil {
		response.Body.Close()
	}
}

func TestDatabaseForwardCopiesErrorsUnchanged(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		writer.Header().Set("Content-Type", "application/json")
		writer.Header().Set("Connection", "X-Internal")
		writer.Header().Set("X-Internal", "secret")
		writer.WriteHeader(http.StatusConflict)
		_, _ = io.WriteString(writer, `{"error":"conflict","message":"duplicate"}`)
	}))
	defer server.Close()

	client := mustClient(t, server.URL)
	database, err := client.Database("tenant")
	if err != nil {
		t.Fatal(err)
	}
	incoming := httptest.NewRequest(http.MethodPost, "/database/query", strings.NewReader(`{}`))
	recorder := httptest.NewRecorder()
	if err := database.Forward(recorder, incoming, DatabaseRoute{Path: "/query"}); err != nil {
		t.Fatal(err)
	}
	if recorder.Code != http.StatusConflict || !strings.Contains(recorder.Body.String(), "duplicate") {
		t.Fatalf("response = %d %s", recorder.Code, recorder.Body.String())
	}
	if recorder.Header().Get("Connection") != "" || recorder.Header().Get("X-Internal") != "" {
		t.Fatalf("hop-by-hop headers leaked: %#v", recorder.Header())
	}
}

func TestDatabaseForwardFlushesSSE(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		writer.Header().Set("Content-Type", "text/event-stream")
		flusher := writer.(http.Flusher)
		_, _ = io.WriteString(writer, ": connected\n\n")
		flusher.Flush()
		time.Sleep(10 * time.Millisecond)
		_, _ = io.WriteString(writer, "data: {\"table\":\"items\",\"action\":\"insert\"}\n\n")
		flusher.Flush()
	}))
	defer server.Close()

	client := mustClient(t, server.URL)
	database, err := client.Database("tenant")
	if err != nil {
		t.Fatal(err)
	}
	incoming := httptest.NewRequest(http.MethodGet, "/database/subscribe", nil)
	writer := newFlushWriter()
	if err := database.Forward(writer, incoming, DatabaseRoute{Path: "/subscribe"}); err != nil {
		t.Fatal(err)
	}
	if writer.flushes < 2 {
		t.Fatalf("flushes = %d, want at least 2", writer.flushes)
	}
	if !strings.Contains(writer.body.String(), `"action":"insert"`) {
		t.Fatalf("body = %q", writer.body.String())
	}
}

func TestDatabaseRouteRejectsControlPlaneAndTraversal(t *testing.T) {
	client := mustClient(t, "http://example.com")
	database, err := client.Database("tenant")
	if err != nil {
		t.Fatal(err)
	}
	for _, route := range []string{
		"",
		"tables/items",
		"/namespaces",
		"/v1/namespaces",
		"/../namespaces",
		"/tables/../namespaces",
		"/query/extra",
		"/tables?x=1",
	} {
		request := httptest.NewRequest(http.MethodGet, "/", nil)
		if _, err := database.RoundTrip(context.Background(), request, DatabaseRoute{Path: route}); err == nil {
			t.Errorf("route %q was accepted", route)
		}
	}
}

func TestDatabaseForwardCancellation(t *testing.T) {
	started := make(chan struct{})
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		close(started)
		<-request.Context().Done()
	}))
	defer server.Close()

	client := mustClient(t, server.URL)
	database, err := client.Database("tenant")
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	incoming := httptest.NewRequest(http.MethodGet, "/database/subscribe", nil).WithContext(ctx)
	errs := make(chan error, 1)
	go func() {
		errs <- database.Forward(httptest.NewRecorder(), incoming, DatabaseRoute{Path: "/subscribe"})
	}()
	<-started
	cancel()
	if err := <-errs; err == nil {
		t.Fatal("expected cancellation error")
	}
}

type flushWriter struct {
	mu      sync.Mutex
	header  http.Header
	body    bytes.Buffer
	status  int
	flushes int
}

func newFlushWriter() *flushWriter {
	return &flushWriter{header: make(http.Header)}
}

func (w *flushWriter) Header() http.Header {
	return w.header
}

func (w *flushWriter) WriteHeader(status int) {
	w.status = status
}

func (w *flushWriter) Write(payload []byte) (int, error) {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.body.Write(payload)
}

func (w *flushWriter) Flush() {
	w.mu.Lock()
	w.flushes++
	w.mu.Unlock()
}

func TestSQLForwardRoutes(t *testing.T) {
	for _, route := range []string{"/query/exec", "/query/transaction"} {
		if err := validateDatabaseRoute(route); err != nil {
			t.Fatalf("%s: %v", route, err)
		}
	}
	for _, route := range []string{"/query/exec/extra", "/query/transaction/../exec", "/query/other"} {
		if err := validateDatabaseRoute(route); err == nil {
			t.Fatalf("unexpected route: %s", route)
		}
	}
}
