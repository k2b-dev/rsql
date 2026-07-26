package rsql

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestEventStreamParsesMultilineAndReportsInvalidJSON(t *testing.T) {
	responses := []string{
		": connected\n\nevent: insert\ndata: {\"table\":\"items\",\ndata: \"action\":\"insert\"}\n\n",
		"data: not-json\n\n",
	}
	index := 0
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		writer.Header().Set("Content-Type", "text/event-stream")
		_, _ = io.WriteString(writer, responses[index])
		index++
	}))
	defer server.Close()

	client := mustClient(t, server.URL)
	database, err := client.Database("tenant")
	if err != nil {
		t.Fatal(err)
	}
	stream, err := database.Events.Subscribe(context.Background(), SubscribeOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if !stream.Next() || stream.Event().Table != "items" {
		t.Fatalf("event = %#v, error = %v", stream.Event(), stream.Err())
	}
	if stream.Next() || stream.Err() != nil {
		t.Fatalf("unexpected terminal state: %v", stream.Err())
	}
	_ = stream.Close()

	invalid, err := database.Events.Subscribe(context.Background(), SubscribeOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if invalid.Next() || invalid.Err() == nil {
		t.Fatal("expected invalid JSON error")
	}
	_ = invalid.Close()
}
