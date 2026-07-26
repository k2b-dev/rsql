package rsql

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
)

func TestNewValidatesConfig(t *testing.T) {
	tests := []Config{
		{},
		{BaseURL: "http://example.com"},
		{BaseURL: "ftp://example.com", Token: "token"},
		{BaseURL: "http://example.com?x=1", Token: "token"},
	}
	for _, config := range tests {
		if _, err := New(config); err == nil {
			t.Fatalf("New(%#v) succeeded", config)
		}
	}
	if _, err := New(Config{BaseURL: "http://example.com/base/", Token: "token"}); err != nil {
		t.Fatal(err)
	}
}

func TestAPIErrorAndAuthentication(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if got := request.Header.Get("Authorization"); got != "Bearer secret" {
			t.Errorf("authorization = %q", got)
		}
		writer.Header().Set("X-Test", "value")
		writer.WriteHeader(http.StatusInsufficientStorage)
		_, _ = io.WriteString(writer, `{"error":"quota_exceeded","message":"full"}`)
	}))
	defer server.Close()

	client := mustClient(t, server.URL)
	_, err := client.Admin().Namespaces.List(context.Background())
	var apiErr *APIError
	if !errors.As(err, &apiErr) {
		t.Fatalf("error = %v, want APIError", err)
	}
	if apiErr.Status != http.StatusInsufficientStorage || apiErr.Code != "quota_exceeded" ||
		apiErr.Message != "full" || apiErr.Header.Get("X-Test") != "value" {
		t.Fatalf("unexpected API error: %#v", apiErr)
	}
	if got := apiErr.Error(); got != "rsql API error 507 quota_exceeded: full" {
		t.Fatalf("error string = %q", got)
	}
}

func TestAPIErrorFallsBackToHTTPStatus(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		writer.WriteHeader(http.StatusBadGateway)
		_, _ = io.WriteString(writer, "not JSON")
	}))
	defer server.Close()

	client := mustClient(t, server.URL)
	_, err := client.Admin().Namespaces.List(context.Background())
	var apiErr *APIError
	if !errors.As(err, &apiErr) {
		t.Fatalf("error = %v, want APIError", err)
	}
	if apiErr.Code != "unknown_error" || apiErr.Message != "502 Bad Gateway" {
		t.Fatalf("unexpected fallback error: %#v", apiErr)
	}
}

func TestNamespaceCreateUsesDefaultConfig(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		var definition NamespaceDefinition
		if err := json.NewDecoder(request.Body).Decode(&definition); err != nil {
			t.Fatal(err)
		}
		if definition.Config == nil || definition.Config.JournalMode != "wal" ||
			definition.Config.ForeignKeys == nil || !*definition.Config.ForeignKeys {
			t.Fatalf("default config = %#v", definition.Config)
		}
		writer.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(writer, `{"name":"tenant"}`)
	}))
	defer server.Close()

	client := mustClient(t, server.URL)
	if _, err := client.Admin().Namespaces.Create(
		context.Background(),
		NamespaceDefinition{Name: "tenant"},
	); err != nil {
		t.Fatal(err)
	}
}

func TestPublicValidationRejectsInvalidNames(t *testing.T) {
	client := mustClient(t, "http://example.com")
	if _, err := client.Database("../tenant"); err == nil {
		t.Fatal("invalid namespace was accepted")
	}
	database, err := client.Database("tenant")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := database.Table("invalid-table"); err == nil {
		t.Fatal("invalid table was accepted")
	}
	if err := database.Tables.Create(
		context.Background(),
		TableCreateRequest{Name: "invalid-table"},
	); err == nil {
		t.Fatal("invalid table create was accepted")
	}
}

func TestClientIsSafeForConcurrentUse(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if request.URL.Path != "/v1/tenant/overview" {
			t.Errorf("path = %q", request.URL.Path)
		}
		writer.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(writer, `{"health":{"status":"healthy"}}`)
	}))
	defer server.Close()

	client := mustClient(t, server.URL)
	database, err := client.Database("tenant")
	if err != nil {
		t.Fatal(err)
	}

	var wait sync.WaitGroup
	errs := make(chan error, 100)
	for range 100 {
		wait.Add(1)
		go func() {
			defer wait.Done()
			overview, err := database.Overview.Get(context.Background(), Window24Hour)
			if err == nil && overview.Health.Status != "healthy" {
				err = errors.New("unexpected overview")
			}
			errs <- err
		}()
	}
	wait.Wait()
	close(errs)
	for err := range errs {
		if err != nil {
			t.Fatal(err)
		}
	}
}

func TestContextCancellation(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		<-request.Context().Done()
	}))
	defer server.Close()

	client := mustClient(t, server.URL)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err := client.Admin().Namespaces.List(ctx)
	if err == nil || !strings.Contains(err.Error(), "context canceled") {
		t.Fatalf("error = %v", err)
	}
}

func mustClient(t *testing.T, baseURL string) *Client {
	t.Helper()
	client, err := New(Config{BaseURL: baseURL, Token: "secret"})
	if err != nil {
		t.Fatal(err)
	}
	return client
}
