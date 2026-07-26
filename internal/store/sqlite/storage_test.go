package sqlite

import (
	"strings"
	"testing"

	"github.com/ValentinKolb/rsql/internal/domain"
)

func TestStorageAndQuota(t *testing.T) {
	db, path := openTestDB(t)
	cfg := domain.NamespaceConfig{
		JournalMode:  "wal",
		BusyTimeout:  1000,
		MaxDBSize:    64 << 10,
		QueryTimeout: 1000,
		ForeignKeys:  domain.BoolPtr(true),
	}
	if err := ApplyNamespaceConfig(db, cfg); err != nil {
		t.Fatalf("apply quota: %v", err)
	}
	if _, err := db.Exec(`CREATE TABLE payloads (value TEXT NOT NULL)`); err != nil {
		t.Fatalf("create table: %v", err)
	}

	var fullErr error
	for i := 0; i < 100; i++ {
		if _, err := db.Exec(`INSERT INTO payloads(value) VALUES (?)`, strings.Repeat("x", 4096)); err != nil {
			fullErr = err
			break
		}
	}
	if !IsFullError(fullErr) {
		t.Fatalf("expected SQLITE_FULL, got %v", fullErr)
	}

	info, err := Storage(db, path, cfg.MaxDBSize)
	if err != nil {
		t.Fatalf("storage: %v", err)
	}
	if info.AllocatedBytes > cfg.MaxDBSize {
		t.Fatalf("allocated %d bytes above quota %d", info.AllocatedBytes, cfg.MaxDBSize)
	}
	if info.QuotaBytes != cfg.MaxDBSize || info.RemainingBytes == nil || info.UsageRatio == nil {
		t.Fatalf("incomplete quota info: %#v", info)
	}
	if *info.RemainingBytes != cfg.MaxDBSize-info.AllocatedBytes {
		t.Fatalf("remaining bytes = %d, want %d", *info.RemainingBytes, cfg.MaxDBSize-info.AllocatedBytes)
	}
}

func TestQuotaBelowCurrentSizeIsRejected(t *testing.T) {
	db, _ := openTestDB(t)
	err := ApplyNamespaceConfig(db, domain.NamespaceConfig{MaxDBSize: 1})
	if err == nil {
		t.Fatal("expected too-small quota error")
	}
}
