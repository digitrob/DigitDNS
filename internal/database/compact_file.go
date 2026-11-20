package database

import (
	"fmt"
	"os"
	"time"

	bolt "go.etcd.io/bbolt"

	"digitdns/internal/crypto"
)

// FileStats is the on-disk shape of a bbolt store. It is deliberately a plain
// value: the maintenance command prints it, and a caller can diff a before and
// after snapshot without holding a live handle open.
type FileStats struct {
	Path      string         `json:"path"`
	SizeBytes int64          `json:"size_bytes"`
	FreeBytes int64          `json:"free_bytes"`
	FreePages int            `json:"free_pages"`
	Buckets   map[string]int `json:"buckets"`
}

// InspectFile reports a store's high-water size, reclaimable free space and
// bucket counts using a read-only bbolt handle. bbolt still takes an exclusive
// file lock, so the daemon must be stopped for this command as well.
func InspectFile(path string) (FileStats, error) {
	info, err := os.Stat(path)
	if err != nil {
		return FileStats{}, fmt.Errorf("stat database: %w", err)
	}
	db, err := bolt.Open(path, 0600, &bolt.Options{
		ReadOnly:        true,
		PreLoadFreelist: true,
		Timeout:         3 * time.Second,
	})
	if err != nil {
		return FileStats{}, fmt.Errorf("open database read-only: %w", err)
	}
	defer db.Close()

	out := FileStats{
		Path:      path,
		SizeBytes: info.Size(),
		Buckets:   make(map[string]int),
	}
	// The freelist is loaded on the first transaction, not at Open — in a
	// read-only handle, stats read before this are all zeroes and a genuinely
	// bloated file looks deceptively clean.
	if err := db.View(func(tx *bolt.Tx) error {
		return tx.ForEach(func(name []byte, b *bolt.Bucket) error {
			out.Buckets[string(name)] = b.Stats().KeyN
			return nil
		})
	}); err != nil {
		return FileStats{}, fmt.Errorf("survey database buckets: %w", err)
	}
	stats := db.Stats()
	out.FreePages = stats.FreePageN
	// FreeAlloc is only maintained for the live write handle; a read-only audit
	// reconstructs the freelist and sets the page count, so derive the byte count
	// from the database's own page size instead of reporting a misleading zero.
	out.FreeBytes = int64(stats.FreePageN) * int64(db.Info().PageSize)
	return out, nil
}

// CompactFile rewrites a store in place through the same verified path the
// encryption migration uses. The daemon must be stopped: compaction needs the
// exclusive bbolt lock. It returns the before and after snapshots so a caller
// can say how much was actually reclaimed instead of guessing from a log line.
func CompactFile(path string, cipher *crypto.Cipher) (FileStats, FileStats, error) {
	before, err := InspectFile(path)
	if err != nil {
		return before, before, err
	}
	src, err := bolt.Open(path, 0600, &bolt.Options{Timeout: 3 * time.Second})
	if err != nil {
		return before, before, fmt.Errorf("open database for compaction: %w", err)
	}
	rebuilt, cerr := compactDatabase(src, path, cipher)
	if rebuilt != nil {
		_ = rebuilt.Close()
	} else if cerr != nil {
		// compactDatabase returns the original handle on a recoverable failure and
		// nil only after it has already closed it. A second Close on an unused
		// handle is harmless; leaking one would keep the lock until process exit.
		_ = src.Close()
	}
	if cerr != nil {
		return before, before, cerr
	}
	after, err := InspectFile(path)
	if err != nil {
		return before, after, fmt.Errorf("inspect compacted database: %w", err)
	}
	return before, after, nil
}
