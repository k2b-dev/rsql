package rsql

import (
	"context"
	"errors"
	"fmt"
	"io"
	"mime"
	"net/http"
	"strings"
)

// DatabaseRoute identifies a namespace-relative data-plane route.
type DatabaseRoute struct {
	Path     string
	RawQuery string
}

var forwardedRequestHeaders = [...]string{
	"Accept",
	"Accept-Encoding",
	"Cache-Control",
	"Content-Encoding",
	"Content-Type",
	"Last-Event-ID",
	"Prefer",
	"Range",
	"X-Request-ID",
}

var hopByHopHeaders = map[string]struct{}{
	"Connection":          {},
	"Keep-Alive":          {},
	"Proxy-Authenticate":  {},
	"Proxy-Authorization": {},
	"Proxy-Connection":    {},
	"Te":                  {},
	"Trailer":             {},
	"Transfer-Encoding":   {},
	"Upgrade":             {},
}

// RoundTrip forwards an incoming request to this database without buffering.
//
// Unlike typed methods, non-2xx responses are returned unchanged. The caller
// owns and must close the response body.
func (d *DatabaseClient) RoundTrip(
	ctx context.Context,
	incoming *http.Request,
	route DatabaseRoute,
) (*http.Response, error) {
	if incoming == nil {
		return nil, errors.New("incoming request is required")
	}
	if err := validateDatabaseRoute(route.Path); err != nil {
		return nil, err
	}
	rawQuery := route.RawQuery
	if rawQuery == "" {
		rawQuery = incoming.URL.RawQuery
	}

	requestURL := *d.client.baseURL
	requestURL.Path = strings.TrimRight(d.client.baseURL.Path, "/") + d.path(route.Path)
	requestURL.RawQuery = rawQuery
	requestURL.RawPath = ""

	upstream, err := http.NewRequestWithContext(ctx, incoming.Method, requestURL.String(), incoming.Body)
	if err != nil {
		return nil, fmt.Errorf("create forwarded request: %w", err)
	}
	upstream.ContentLength = incoming.ContentLength
	upstream.Header.Set("Authorization", "Bearer "+d.client.token)
	for _, name := range forwardedRequestHeaders {
		for _, value := range incoming.Header.Values(name) {
			upstream.Header.Add(name, value)
		}
	}

	response, err := d.client.httpClient.Do(upstream)
	if err != nil {
		return nil, fmt.Errorf("forward request: %w", err)
	}
	return response, nil
}

// Forward streams an incoming request and its upstream response.
func (d *DatabaseClient) Forward(
	writer http.ResponseWriter,
	incoming *http.Request,
	route DatabaseRoute,
) error {
	response, err := d.RoundTrip(incoming.Context(), incoming, route)
	if err != nil {
		return err
	}
	defer response.Body.Close()

	copyResponseHeaders(writer.Header(), response.Header)
	writer.WriteHeader(response.StatusCode)
	if incoming.Method == http.MethodHead {
		return nil
	}
	if isEventStream(response.Header) {
		return copyEventStream(writer, response.Body)
	}
	buffer := make([]byte, 32<<10)
	if _, err := io.CopyBuffer(writer, response.Body, buffer); err != nil {
		return fmt.Errorf("stream forwarded response: %w", err)
	}
	return nil
}

func validateDatabaseRoute(route string) error {
	if route == "" || route[0] != '/' {
		return errors.New("database route must start with /")
	}
	if strings.ContainsAny(route, "?#\\") {
		return errors.New("database route must contain only a path")
	}
	segments := strings.Split(strings.Trim(route, "/"), "/")
	if len(segments) == 0 || segments[0] == "" {
		return errors.New("database route is empty")
	}
	for _, segment := range segments {
		if segment == "." || segment == ".." {
			return errors.New("database route traversal is not allowed")
		}
	}
	switch segments[0] {
	case "tables":
		return nil
	case "query":
		if len(segments) == 1 || (len(segments) == 2 && (segments[1] == "exec" || segments[1] == "transaction")) {
			return nil
		}
		return errors.New("database route has unexpected query path segments")
	case "subscribe", "changelog", "overview":
		if len(segments) != 1 {
			return errors.New("database route has unexpected path segments")
		}
		return nil
	default:
		return fmt.Errorf("database route %q is not a data-plane route", route)
	}
}

func copyResponseHeaders(target, source http.Header) {
	connectionHeaders := make(map[string]struct{})
	for _, value := range source.Values("Connection") {
		for _, name := range strings.Split(value, ",") {
			connectionHeaders[http.CanonicalHeaderKey(strings.TrimSpace(name))] = struct{}{}
		}
	}
	for name, values := range source {
		canonical := http.CanonicalHeaderKey(name)
		if _, blocked := hopByHopHeaders[canonical]; blocked {
			continue
		}
		if _, blocked := connectionHeaders[canonical]; blocked {
			continue
		}
		for _, value := range values {
			target.Add(canonical, value)
		}
	}
}

func isEventStream(header http.Header) bool {
	contentType := header.Get("Content-Type")
	mediaType, _, err := mime.ParseMediaType(contentType)
	return err == nil && mediaType == "text/event-stream"
}

func copyEventStream(writer http.ResponseWriter, source io.Reader) error {
	controller := http.NewResponseController(writer)
	if err := controller.Flush(); err != nil {
		return fmt.Errorf("flush event stream headers: %w", err)
	}
	buffer := make([]byte, 16<<10)
	for {
		read, readErr := source.Read(buffer)
		if read > 0 {
			if _, err := writer.Write(buffer[:read]); err != nil {
				return fmt.Errorf("write event stream: %w", err)
			}
			if err := controller.Flush(); err != nil {
				return fmt.Errorf("flush event stream: %w", err)
			}
		}
		if readErr != nil {
			if errors.Is(readErr, io.EOF) {
				return nil
			}
			return fmt.Errorf("read event stream: %w", readErr)
		}
	}
}
