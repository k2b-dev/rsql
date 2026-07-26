package sqlite

import (
	"database/sql"
	"encoding/json"
	"fmt"
)

// ChangelogEntry is one schema changelog item.
type ChangelogEntry struct {
	ID        int             `json:"id"`
	Timestamp string          `json:"timestamp"`
	Action    string          `json:"action"`
	Table     string          `json:"table"`
	Detail    json.RawMessage `json:"detail"`
	Meta      json.RawMessage `json:"_meta,omitempty"`
}

// ListChangelog lists schema changelog entries.
func ListChangelog(db *sql.DB, tableFilter string, limit, offset int) ([]ChangelogEntry, error) {
	if limit <= 0 {
		limit = 50
	}
	if offset < 0 {
		offset = 0
	}

	query := `SELECT id, timestamp, action, table_name, detail_json, COALESCE(meta_json,'null') FROM _schema_log`
	args := make([]any, 0, 3)
	if tableFilter != "" {
		query += ` WHERE table_name = ?`
		args = append(args, tableFilter)
	}
	query += ` ORDER BY id DESC LIMIT ? OFFSET ?`
	args = append(args, limit, offset)

	rows, err := db.Query(query, args...)
	if err != nil {
		return nil, fmt.Errorf("query changelog: %w", err)
	}
	defer rows.Close()

	var out []ChangelogEntry
	for rows.Next() {
		var e ChangelogEntry
		var detailRaw string
		var metaRaw string
		if err := rows.Scan(&e.ID, &e.Timestamp, &e.Action, &e.Table, &detailRaw, &metaRaw); err != nil {
			return nil, fmt.Errorf("scan changelog entry: %w", err)
		}
		e.Detail = json.RawMessage(detailRaw)
		e.Meta = json.RawMessage(metaRaw)
		out = append(out, e)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate changelog: %w", err)
	}
	return out, nil
}
