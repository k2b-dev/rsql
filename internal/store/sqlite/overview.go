package sqlite

import (
	"database/sql"
	"fmt"

	"github.com/k2b-dev/rsql/internal/domain"
)

// SchemaSummary returns inexpensive schema object counts.
func SchemaSummary(db *sql.DB) (domain.OverviewSchema, error) {
	var summary domain.OverviewSchema
	err := db.QueryRow(`
		SELECT
			COUNT(*) FILTER (WHERE type = 'table' AND name NOT GLOB '_*' AND name != 'sqlite_sequence'),
			COUNT(*) FILTER (WHERE type = 'view' AND name NOT GLOB '_*'),
			COUNT(*) FILTER (
				WHERE type = 'index'
					AND name NOT GLOB '_*'
					AND tbl_name NOT GLOB '_fts_*'
					AND sql IS NOT NULL
			),
			COUNT(*) FILTER (WHERE type = 'table' AND name GLOB '_fts_*' AND sql LIKE '%USING fts5%')
		FROM sqlite_schema
	`).Scan(&summary.Tables, &summary.Views, &summary.Indexes, &summary.FTSIndexes)
	if err != nil {
		return domain.OverviewSchema{}, fmt.Errorf("query schema summary: %w", err)
	}
	return summary, nil
}
