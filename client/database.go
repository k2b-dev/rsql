package rsql

import (
	"context"
	"encoding/json"
	"net/http"
	"net/url"
	"strconv"
	"strings"
)

// DatabaseClient exposes data-plane operations for one fixed namespace.
type DatabaseClient struct {
	client    *Client
	namespace string

	Overview  OverviewClient
	Query     QueryClient
	Changelog ChangelogClient
	Events    EventsClient
	Tables    TablesClient
}

// Namespace returns the bound namespace name.
func (d *DatabaseClient) Namespace() string {
	return d.namespace
}

// Table binds operations to one table.
func (d *DatabaseClient) Table(name string) (*TableClient, error) {
	if err := validateIdentifier("table", name); err != nil {
		return nil, err
	}
	table := &TableClient{database: d, name: name}
	table.Schema = SchemaClient{table: table}
	table.Indexes = IndexesClient{table: table}
	table.Rows = RowsClient{table: table}
	return table, nil
}

func (d *DatabaseClient) path(suffix string) string {
	return "/v1/" + url.PathEscape(d.namespace) + suffix
}

// OverviewClient reads namespace health and usage.
type OverviewClient struct {
	database *DatabaseClient
}

// Get returns a namespace overview for window.
func (o OverviewClient) Get(ctx context.Context, window OverviewWindow) (NamespaceOverview, error) {
	if window == "" {
		window = Window24Hour
	}
	var result NamespaceOverview
	err := o.database.client.doJSON(
		ctx,
		http.MethodGet,
		o.database.path("/overview"),
		url.Values{"window": {string(window)}},
		nil,
		nil,
		&result,
	)
	return result, err
}

// QueryClient runs read-only SQL.
type QueryClient struct {
	database *DatabaseClient
}

// Run executes a single query or request-defined batch.
func (q QueryClient) Run(ctx context.Context, request QueryRequest) (map[string]any, error) {
	var result map[string]any
	err := q.database.client.doJSON(ctx, http.MethodPost, q.database.path("/query"), nil, request, nil, &result)
	return result, err
}

// Batch executes read-only statements in one request.
func (q QueryClient) Batch(ctx context.Context, statements []QueryStatement) (map[string]any, error) {
	return q.Run(ctx, QueryRequest{Statements: statements})
}

// ChangelogClient reads schema changes.
type ChangelogClient struct {
	database *DatabaseClient
}

// List returns schema changelog entries.
func (c ChangelogClient) List(
	ctx context.Context,
	table string,
	limit int,
	offset int,
) ([]ChangelogEntry, error) {
	query := make(url.Values)
	if table != "" {
		if err := validateIdentifier("table", table); err != nil {
			return nil, err
		}
		query.Set("table", table)
	}
	if limit > 0 {
		query.Set("limit", strconv.Itoa(limit))
	}
	if offset > 0 {
		query.Set("offset", strconv.Itoa(offset))
	}
	var result []ChangelogEntry
	err := c.database.client.doJSON(
		ctx,
		http.MethodGet,
		c.database.path("/changelog"),
		query,
		nil,
		nil,
		&result,
	)
	return result, err
}

// TablesClient manages tables and views inside a namespace.
type TablesClient struct {
	database *DatabaseClient
}

// List returns all user tables and views.
func (t TablesClient) List(ctx context.Context) ([]map[string]any, error) {
	var result []map[string]any
	err := t.database.client.doJSON(ctx, http.MethodGet, t.database.path("/tables"), nil, nil, nil, &result)
	return result, err
}

// Create creates a table or view.
func (t TablesClient) Create(ctx context.Context, request TableCreateRequest) error {
	if err := validateIdentifier("table", request.Name); err != nil {
		return err
	}
	return t.database.client.doJSON(ctx, http.MethodPost, t.database.path("/tables"), nil, request, nil, nil)
}

// Get returns table or view schema.
func (t TablesClient) Get(ctx context.Context, name string) (map[string]any, error) {
	table, err := t.database.Table(name)
	if err != nil {
		return nil, err
	}
	return table.Schema.Get(ctx)
}

// Update updates table or view schema.
func (t TablesClient) Update(ctx context.Context, name string, request TableUpdateRequest) error {
	table, err := t.database.Table(name)
	if err != nil {
		return err
	}
	return table.Schema.Update(ctx, request)
}

// Delete removes a table or view.
func (t TablesClient) Delete(ctx context.Context, name string, meta json.RawMessage) error {
	table, err := t.database.Table(name)
	if err != nil {
		return err
	}
	return table.Schema.Delete(ctx, meta)
}

func queryValues(input map[string][]string) url.Values {
	if input == nil {
		return nil
	}
	values := make(url.Values, len(input))
	for name, items := range input {
		values[name] = append([]string(nil), items...)
	}
	return values
}

func joinTables(tables []string) string {
	return strings.Join(tables, ",")
}
