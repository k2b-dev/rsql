package rsql

import (
	"context"
	"encoding/json"
	"net/http"
	"net/url"
	"strconv"
)

// TableClient exposes operations for one fixed table.
type TableClient struct {
	database *DatabaseClient
	name     string

	Schema  SchemaClient
	Indexes IndexesClient
	Rows    RowsClient
}

// Name returns the bound table name.
func (t *TableClient) Name() string {
	return t.name
}

func (t *TableClient) path(suffix string) string {
	return t.database.path("/tables/" + url.PathEscape(t.name) + suffix)
}

// Export streams this table as CSV.
func (t *TableClient) Export(ctx context.Context, options TableExportOptions) (*StreamResponse, error) {
	query := queryValues(options.Query)
	if query == nil {
		query = make(url.Values)
	}
	query.Set("format", "csv")
	if options.BOM {
		query.Set("bom", "true")
	}
	return t.database.client.doStream(ctx, http.MethodGet, t.path("/export"), query, nil, -1, nil)
}

// SchemaClient manages one table or view schema.
type SchemaClient struct {
	table *TableClient
}

// Get returns table or view schema.
func (s SchemaClient) Get(ctx context.Context) (map[string]any, error) {
	var result map[string]any
	err := s.table.database.client.doJSON(ctx, http.MethodGet, s.table.path(""), nil, nil, nil, &result)
	return result, err
}

// Update applies a schema update.
func (s SchemaClient) Update(ctx context.Context, request TableUpdateRequest) error {
	return s.table.database.client.doJSON(ctx, http.MethodPut, s.table.path(""), nil, request, nil, nil)
}

// Delete removes the table or view.
func (s SchemaClient) Delete(ctx context.Context, meta json.RawMessage) error {
	var payload any
	if len(meta) > 0 {
		payload = map[string]json.RawMessage{"_meta": meta}
	}
	return s.table.database.client.doJSON(ctx, http.MethodDelete, s.table.path(""), nil, payload, nil, nil)
}

// IndexesClient manages indexes for one table.
type IndexesClient struct {
	table *TableClient
}

// Create creates a regular, unique, or FTS index.
func (i IndexesClient) Create(ctx context.Context, request IndexCreateRequest) error {
	if request.Name != "" {
		if err := validateIdentifier("index", request.Name); err != nil {
			return err
		}
	}
	return i.table.database.client.doJSON(
		ctx,
		http.MethodPost,
		i.table.path("/indexes"),
		nil,
		request,
		nil,
		nil,
	)
}

// Delete removes an index.
func (i IndexesClient) Delete(ctx context.Context, name string, meta json.RawMessage) error {
	if err := validateIdentifier("index", name); err != nil {
		return err
	}
	var payload any
	if len(meta) > 0 {
		payload = map[string]json.RawMessage{"_meta": meta}
	}
	return i.table.database.client.doJSON(
		ctx,
		http.MethodDelete,
		i.table.path("/indexes/"+url.PathEscape(name)),
		nil,
		payload,
		nil,
		nil,
	)
}

// RowsClient manages rows for one table.
type RowsClient struct {
	table *TableClient
}

// List returns filtered and paginated rows.
func (r RowsClient) List(ctx context.Context, query map[string][]string) (RowsResponse, error) {
	var result RowsResponse
	err := r.table.database.client.doJSON(
		ctx,
		http.MethodGet,
		r.table.path("/rows"),
		queryValues(query),
		nil,
		nil,
		&result,
	)
	return result, err
}

// Get returns one row by id.
func (r RowsClient) Get(ctx context.Context, id int64) (map[string]any, error) {
	var result map[string]any
	err := r.table.database.client.doJSON(
		ctx,
		http.MethodGet,
		r.table.path("/rows/"+strconv.FormatInt(id, 10)),
		nil,
		nil,
		nil,
		&result,
	)
	return result, err
}

// InsertOne inserts one row.
func (r RowsClient) InsertOne(
	ctx context.Context,
	row map[string]any,
	options MutateOptions,
) (MutationResult, error) {
	payload := cloneMap(row)
	if len(options.Meta) > 0 {
		payload["_meta"] = options.Meta
	}
	return r.mutate(ctx, http.MethodPost, nil, payload, options)
}

// Insert inserts multiple rows.
func (r RowsClient) Insert(
	ctx context.Context,
	rows []map[string]any,
	options MutateOptions,
) (MutationResult, error) {
	payload := map[string]any{"rows": rows}
	if len(options.Meta) > 0 {
		payload["_meta"] = options.Meta
	}
	return r.mutate(ctx, http.MethodPost, nil, payload, options)
}

// Update updates one row by id.
func (r RowsClient) Update(
	ctx context.Context,
	id int64,
	payload map[string]any,
	options MutateOptions,
) (MutationResult, error) {
	return r.mutatePath(
		ctx,
		http.MethodPut,
		r.table.path("/rows/"+strconv.FormatInt(id, 10)),
		nil,
		withMeta(payload, options.Meta),
		options,
	)
}

// Delete removes one row by id.
func (r RowsClient) Delete(ctx context.Context, id int64, options MutateOptions) (MutationResult, error) {
	return r.mutatePath(
		ctx,
		http.MethodDelete,
		r.table.path("/rows/"+strconv.FormatInt(id, 10)),
		nil,
		metaPayload(options.Meta),
		options,
	)
}

// BulkUpdate updates all rows matching query.
func (r RowsClient) BulkUpdate(
	ctx context.Context,
	query map[string][]string,
	payload map[string]any,
	options MutateOptions,
) (MutationResult, error) {
	return r.mutate(ctx, http.MethodPatch, queryValues(query), withMeta(payload, options.Meta), options)
}

// BulkDelete removes all rows matching query.
func (r RowsClient) BulkDelete(
	ctx context.Context,
	query map[string][]string,
	options MutateOptions,
) (MutationResult, error) {
	return r.mutate(ctx, http.MethodDelete, queryValues(query), metaPayload(options.Meta), options)
}

func (r RowsClient) mutate(
	ctx context.Context,
	method string,
	query url.Values,
	payload any,
	options MutateOptions,
) (MutationResult, error) {
	return r.mutatePath(ctx, method, r.table.path("/rows"), query, payload, options)
}

func (r RowsClient) mutatePath(
	ctx context.Context,
	method string,
	path string,
	query url.Values,
	payload any,
	options MutateOptions,
) (MutationResult, error) {
	headers := make(http.Header)
	if options.Prefer != "" {
		headers.Set("Prefer", options.Prefer)
	}
	var result MutationResult
	err := r.table.database.client.doJSON(ctx, method, path, query, payload, headers, &result)
	return result, err
}

func withMeta(payload map[string]any, meta json.RawMessage) map[string]any {
	result := cloneMap(payload)
	if len(meta) > 0 {
		result["_meta"] = meta
	}
	return result
}

func metaPayload(meta json.RawMessage) any {
	if len(meta) == 0 {
		return nil
	}
	return map[string]json.RawMessage{"_meta": meta}
}

func cloneMap(input map[string]any) map[string]any {
	result := make(map[string]any, len(input)+1)
	for key, value := range input {
		result[key] = value
	}
	return result
}
