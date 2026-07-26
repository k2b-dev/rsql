package rsql

import (
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

// Config configures a Client.
type Config struct {
	BaseURL    string
	Token      string
	HTTPClient *http.Client
}

// Client is a concurrency-safe rsql API client.
type Client struct {
	baseURL    *url.URL
	token      string
	httpClient *http.Client
}

// New creates a client with explicit connection and authentication settings.
func New(config Config) (*Client, error) {
	if strings.TrimSpace(config.BaseURL) == "" {
		return nil, errors.New("base URL is required")
	}
	if config.Token == "" {
		return nil, errors.New("token is required")
	}
	baseURL, err := url.Parse(strings.TrimRight(config.BaseURL, "/"))
	if err != nil {
		return nil, fmt.Errorf("parse base URL: %w", err)
	}
	if baseURL.Scheme != "http" && baseURL.Scheme != "https" {
		return nil, errors.New("base URL must use http or https")
	}
	if baseURL.Host == "" || baseURL.RawQuery != "" || baseURL.Fragment != "" {
		return nil, errors.New("base URL must contain an origin without query or fragment")
	}

	httpClient := config.HTTPClient
	if httpClient == nil {
		transport := http.DefaultTransport.(*http.Transport).Clone()
		// Passthrough responses must not be transparently transformed.
		transport.DisableCompression = true
		httpClient = &http.Client{Transport: transport}
	}
	return &Client{baseURL: baseURL, token: config.Token, httpClient: httpClient}, nil
}

// Admin returns the explicit control-plane API.
func (c *Client) Admin() AdminClient {
	return AdminClient{Namespaces: NamespaceAdmin{client: c}}
}

// CloseIdleConnections closes pooled idle HTTP connections.
func (c *Client) CloseIdleConnections() {
	c.httpClient.CloseIdleConnections()
}

// Database binds all subsequent operations to one namespace.
func (c *Client) Database(namespace string) (*DatabaseClient, error) {
	if err := validateNamespace(namespace); err != nil {
		return nil, err
	}
	database := &DatabaseClient{client: c, namespace: namespace}
	database.Overview = OverviewClient{database: database}
	database.Query = QueryClient{database: database}
	database.Changelog = ChangelogClient{database: database}
	database.Events = EventsClient{database: database}
	database.Tables = TablesClient{database: database}
	return database, nil
}

func (c *Client) doJSON(
	ctx context.Context,
	method string,
	path string,
	query url.Values,
	payload any,
	headers http.Header,
	target any,
) error {
	var body io.Reader
	if payload != nil {
		encoded, err := json.Marshal(payload)
		if err != nil {
			return fmt.Errorf("encode request: %w", err)
		}
		body = bytes.NewReader(encoded)
		if headers == nil {
			headers = make(http.Header)
		}
		headers.Set("Content-Type", "application/json")
	}
	response, err := c.send(ctx, method, path, query, body, -1, headers)
	if err != nil {
		return err
	}
	defer response.Body.Close()
	if err := checkResponse(response); err != nil {
		return err
	}
	if target == nil || response.StatusCode == http.StatusNoContent {
		_, _ = io.Copy(io.Discard, response.Body)
		return nil
	}
	if err := json.NewDecoder(response.Body).Decode(target); err != nil {
		return fmt.Errorf("decode response: %w", err)
	}
	return nil
}

func (c *Client) doReaderJSON(
	ctx context.Context,
	method string,
	path string,
	query url.Values,
	body io.Reader,
	contentLength int64,
	headers http.Header,
	target any,
) error {
	response, err := c.send(ctx, method, path, query, body, contentLength, headers)
	if err != nil {
		return err
	}
	defer response.Body.Close()
	if err := checkResponse(response); err != nil {
		return err
	}
	if target == nil || response.StatusCode == http.StatusNoContent {
		_, _ = io.Copy(io.Discard, response.Body)
		return nil
	}
	if err := json.NewDecoder(response.Body).Decode(target); err != nil {
		return fmt.Errorf("decode response: %w", err)
	}
	return nil
}

func (c *Client) doStream(
	ctx context.Context,
	method string,
	path string,
	query url.Values,
	body io.Reader,
	contentLength int64,
	headers http.Header,
) (*StreamResponse, error) {
	response, err := c.send(ctx, method, path, query, body, contentLength, headers)
	if err != nil {
		return nil, err
	}
	if err := checkResponse(response); err != nil {
		response.Body.Close()
		return nil, err
	}
	return &StreamResponse{
		Status: response.StatusCode,
		Header: response.Header.Clone(),
		Body:   response.Body,
	}, nil
}

func (c *Client) send(
	ctx context.Context,
	method string,
	path string,
	query url.Values,
	body io.Reader,
	contentLength int64,
	headers http.Header,
) (*http.Response, error) {
	requestURL := *c.baseURL
	requestURL.Path = strings.TrimRight(c.baseURL.Path, "/") + path
	requestURL.RawQuery = query.Encode()

	request, err := http.NewRequestWithContext(ctx, method, requestURL.String(), body)
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}
	request.Header.Set("Authorization", "Bearer "+c.token)
	for name, values := range headers {
		for _, value := range values {
			request.Header.Add(name, value)
		}
	}
	if contentLength >= 0 {
		request.ContentLength = contentLength
	}

	response, err := c.httpClient.Do(request)
	if err != nil {
		return nil, fmt.Errorf("send request: %w", err)
	}
	return response, nil
}

func checkResponse(response *http.Response) error {
	if response.StatusCode >= 200 && response.StatusCode < 300 {
		return nil
	}
	defer response.Body.Close()
	limited := io.LimitReader(response.Body, 1<<20)
	var payload struct {
		Code    string `json:"error"`
		Message string `json:"message"`
	}
	if err := json.NewDecoder(limited).Decode(&payload); err != nil {
		payload.Code = "unknown_error"
		payload.Message = response.Status
	}
	return &APIError{
		Status:  response.StatusCode,
		Code:    payload.Code,
		Message: payload.Message,
		Header:  response.Header.Clone(),
	}
}
