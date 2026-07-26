package sqlite

import (
	"database/sql"
	"fmt"
	"os"
)

// StorageInfo describes logical quota usage and physical SQLite files.
type StorageInfo struct {
	UsedBytes         int64    `json:"used_bytes"`
	AllocatedBytes    int64    `json:"allocated_bytes"`
	ReclaimableBytes  int64    `json:"reclaimable_bytes"`
	QuotaBytes        int64    `json:"quota_bytes"`
	RemainingBytes    *int64   `json:"remaining_bytes"`
	UsageRatio        *float64 `json:"usage_ratio"`
	DatabaseFileBytes int64    `json:"database_file_bytes"`
	WALBytes          int64    `json:"wal_bytes"`
	SHMBytes          int64    `json:"shm_bytes"`
	DiskBytes         int64    `json:"disk_bytes"`
}

// Storage reads constant-time page and file statistics.
func Storage(db *sql.DB, dbPath string, quotaBytes int64) (StorageInfo, error) {
	pageSize, err := pragmaInt64(db, "page_size")
	if err != nil {
		return StorageInfo{}, err
	}
	pageCount, err := pragmaInt64(db, "page_count")
	if err != nil {
		return StorageInfo{}, err
	}
	freePages, err := pragmaInt64(db, "freelist_count")
	if err != nil {
		return StorageInfo{}, err
	}

	databaseBytes, err := fileSize(dbPath)
	if err != nil {
		return StorageInfo{}, fmt.Errorf("stat database file: %w", err)
	}
	walBytes, err := optionalFileSize(dbPath + "-wal")
	if err != nil {
		return StorageInfo{}, fmt.Errorf("stat wal file: %w", err)
	}
	shmBytes, err := optionalFileSize(dbPath + "-shm")
	if err != nil {
		return StorageInfo{}, fmt.Errorf("stat shm file: %w", err)
	}

	allocated := pageCount * pageSize
	reclaimable := freePages * pageSize
	used := allocated - reclaimable
	info := StorageInfo{
		UsedBytes:         used,
		AllocatedBytes:    allocated,
		ReclaimableBytes:  reclaimable,
		QuotaBytes:        quotaBytes,
		DatabaseFileBytes: databaseBytes,
		WALBytes:          walBytes,
		SHMBytes:          shmBytes,
		DiskBytes:         databaseBytes + walBytes + shmBytes,
	}
	if quotaBytes > 0 {
		remaining := max(quotaBytes-allocated, 0)
		ratio := float64(allocated) / float64(quotaBytes)
		info.RemainingBytes = &remaining
		info.UsageRatio = &ratio
	}
	return info, nil
}

func pragmaInt64(db *sql.DB, name string) (int64, error) {
	var value int64
	if err := db.QueryRow("PRAGMA " + name).Scan(&value); err != nil {
		return 0, fmt.Errorf("read pragma %s: %w", name, err)
	}
	return value, nil
}

func fileSize(path string) (int64, error) {
	info, err := os.Stat(path)
	if err != nil {
		return 0, err
	}
	return info.Size(), nil
}

func optionalFileSize(path string) (int64, error) {
	size, err := fileSize(path)
	if os.IsNotExist(err) {
		return 0, nil
	}
	return size, err
}
