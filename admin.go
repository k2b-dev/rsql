package rsql

import (
	"context"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"net/textproto"
	"net/url"
)

// AdminClient exposes control-plane operations.
type AdminClient struct {
	Namespaces NamespaceAdmin
}

// NamespaceAdmin manages namespace lifecycle.
type NamespaceAdmin struct {
	client *Client
}

// Create provisions a namespace.
func (n NamespaceAdmin) Create(ctx context.Context, definition NamespaceDefinition) (NamespaceRecord, error) {
	if err := validateNamespace(definition.Name); err != nil {
		return NamespaceRecord{}, err
	}
	if definition.Config == nil {
		config := DefaultNamespaceConfig()
		definition.Config = &config
	}
	var result NamespaceRecord
	err := n.client.doJSON(ctx, http.MethodPost, "/v1/namespaces", nil, definition, nil, &result)
	return result, err
}

// List returns all active namespaces without opening their database files.
func (n NamespaceAdmin) List(ctx context.Context) ([]NamespaceRecord, error) {
	var result []NamespaceRecord
	err := n.client.doJSON(ctx, http.MethodGet, "/v1/namespaces", nil, nil, nil, &result)
	return result, err
}

// Get returns one namespace record and its config.
func (n NamespaceAdmin) Get(ctx context.Context, namespace string) (NamespaceRecord, error) {
	if err := validateNamespace(namespace); err != nil {
		return NamespaceRecord{}, err
	}
	var result NamespaceRecord
	err := n.client.doJSON(ctx, http.MethodGet, namespaceAdminPath(namespace), nil, nil, nil, &result)
	return result, err
}

// Update applies a namespace config.
func (n NamespaceAdmin) Update(ctx context.Context, namespace string, config NamespaceConfig) (NamespaceRecord, error) {
	if err := validateNamespace(namespace); err != nil {
		return NamespaceRecord{}, err
	}
	var result NamespaceRecord
	err := n.client.doJSON(
		ctx,
		http.MethodPut,
		namespaceAdminPath(namespace),
		nil,
		map[string]any{"config": config},
		nil,
		&result,
	)
	return result, err
}

// Delete removes a namespace.
func (n NamespaceAdmin) Delete(ctx context.Context, namespace string) error {
	if err := validateNamespace(namespace); err != nil {
		return err
	}
	return n.client.doJSON(ctx, http.MethodDelete, namespaceAdminPath(namespace), nil, nil, nil, nil)
}

// Duplicate copies a namespace into target.
func (n NamespaceAdmin) Duplicate(ctx context.Context, source, target string) error {
	if err := validateNamespace(source); err != nil {
		return err
	}
	if err := validateNamespace(target); err != nil {
		return err
	}
	return n.client.doJSON(
		ctx,
		http.MethodPost,
		namespaceAdminPath(source)+"/duplicate",
		nil,
		map[string]string{"name": target},
		nil,
		nil,
	)
}

// Export streams a consistent namespace database snapshot.
func (n NamespaceAdmin) Export(ctx context.Context, namespace string) (*StreamResponse, error) {
	if err := validateNamespace(namespace); err != nil {
		return nil, err
	}
	return n.client.doStream(ctx, http.MethodGet, namespaceAdminPath(namespace)+"/export", nil, nil, -1, nil)
}

// ImportDatabase replaces a namespace from a streamed SQLite upload.
func (n NamespaceAdmin) ImportDatabase(ctx context.Context, namespace string, upload ImportOptions) error {
	if err := validateNamespace(namespace); err != nil {
		return err
	}
	body, contentType, err := multipartUpload(upload)
	if err != nil {
		return err
	}
	headers := make(http.Header)
	headers.Set("Content-Type", contentType)
	return n.client.doReaderJSON(
		ctx,
		http.MethodPost,
		namespaceAdminPath(namespace)+"/import",
		nil,
		body,
		-1,
		headers,
		nil,
	)
}

// ImportCSV streams CSV rows into an existing table.
func (n NamespaceAdmin) ImportCSV(
	ctx context.Context,
	namespace string,
	table string,
	upload ImportOptions,
) (MutationResult, error) {
	if err := validateNamespace(namespace); err != nil {
		return MutationResult{}, err
	}
	if err := validateIdentifier("table", table); err != nil {
		return MutationResult{}, err
	}
	body, contentType, err := multipartUpload(upload)
	if err != nil {
		return MutationResult{}, err
	}
	headers := make(http.Header)
	headers.Set("Content-Type", contentType)
	var result MutationResult
	err = n.client.doReaderJSON(
		ctx,
		http.MethodPost,
		namespaceAdminPath(namespace)+"/import",
		url.Values{"table": {table}},
		body,
		-1,
		headers,
		&result,
	)
	return result, err
}

func namespaceAdminPath(namespace string) string {
	return "/v1/namespaces/" + url.PathEscape(namespace)
}

func multipartUpload(upload ImportOptions) (io.Reader, string, error) {
	if upload.Body == nil {
		return nil, "", fmt.Errorf("upload body is required")
	}
	filename := upload.Filename
	if filename == "" {
		filename = "upload"
	}
	contentType := upload.ContentType
	if contentType == "" {
		contentType = "application/octet-stream"
	}

	reader, writer := io.Pipe()
	multipartWriter := multipart.NewWriter(writer)
	go func() {
		header := make(textproto.MIMEHeader)
		header.Set("Content-Disposition", fmt.Sprintf(`form-data; name="file"; filename="%s"`, escapeQuotes(filename)))
		header.Set("Content-Type", contentType)
		part, err := multipartWriter.CreatePart(header)
		if err == nil {
			_, err = io.Copy(part, upload.Body)
		}
		if closeErr := multipartWriter.Close(); err == nil {
			err = closeErr
		}
		_ = writer.CloseWithError(err)
	}()
	return reader, multipartWriter.FormDataContentType(), nil
}

func escapeQuotes(value string) string {
	result := make([]rune, 0, len(value))
	for _, char := range value {
		if char != '"' && char != '\r' && char != '\n' {
			result = append(result, char)
		}
	}
	return string(result)
}
