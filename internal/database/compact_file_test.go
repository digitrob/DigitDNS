package database

import (
	"fmt"
	"path/filepath"
	"strings"
	"testing"

	bolt "go.etcd.io/bbolt"

	"digitdns/internal/crypto"
)

func TestCompactFileReclaimsFreePagesWithoutLosingBuckets(t *testing.T) {
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "data.db")
	keyPath := filepath.Join(dir, "master.key")
	cipher, err := crypto.LoadOrGenerateMasterKey(keyPath)
	if err != nil {
		t.Fatal(err)
	}
	db, err := Create(dbPath, cipher)
	if err != nil {
		t.Fatal(err)
	}

	// Fill well past bbolt's initial 2 MiB file, then delete. The high-water
	// size stays large while the pages move to the freelist — exactly the shape
	// a long-lived query-log bucket leaves behind in production.
	const items = 1200
	pad := []byte(strings.Repeat("x", 4096))
	for start := 0; start < items; start += 100 {
		if err := db.bolt.Update(func(tx *bolt.Tx) error {
			b := tx.Bucket(bucketFilterLists)
			for i := start; i < start+100 && i < items; i++ {
				if err := b.Put([]byte(fmt.Sprintf("list-%04d", i)), pad); err != nil {
					return err
				}
			}
			return nil
		}); err != nil {
			t.Fatal(err)
		}
	}
	for start := 0; start < items; start += 100 {
		if err := db.bolt.Update(func(tx *bolt.Tx) error {
			b := tx.Bucket(bucketFilterLists)
			for i := start; i < start+100 && i < items; i++ {
				if err := b.Delete([]byte(fmt.Sprintf("list-%04d", i))); err != nil {
					return err
				}
			}
			return nil
		}); err != nil {
			t.Fatal(err)
		}
	}
	beforeBuckets := map[string]int{}
	if err := db.bolt.View(func(tx *bolt.Tx) error {
		return tx.ForEach(func(name []byte, b *bolt.Bucket) error {
			beforeBuckets[string(name)] = b.Stats().KeyN
			return nil
		})
	}); err != nil {
		t.Fatal(err)
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}

	before, after, err := CompactFile(dbPath, cipher)
	if err != nil {
		t.Fatal(err)
	}
	if before.FreePages <= 0 || before.FreeBytes <= 0 {
		t.Fatalf("test setup did not create free pages: %+v", before)
	}
	if after.SizeBytes >= before.SizeBytes {
		t.Fatalf("compaction did not shrink the file: before=%d after=%d", before.SizeBytes, after.SizeBytes)
	}
	for name, keys := range beforeBuckets {
		if got, ok := after.Buckets[name]; !ok || got != keys {
			t.Fatalf("bucket %q lost keys after compaction: got %v, want %d", name, got, keys)
		}
	}
}
