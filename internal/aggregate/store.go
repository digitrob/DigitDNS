package aggregate

import (
	"crypto/rand"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"

	bolt "go.etcd.io/bbolt"
)

const schemaVersion = "1"

var (
	metaBucket    = []byte("meta")
	hourlyBucket  = []byte("hourly")
	dailyBucket   = []byte("daily")
	schemaKey     = []byte("schema")
	clientSaltKey = []byte("client_salt")
)

type bucketPair struct {
	key    string
	bucket *Bucket
	daily  bool
}

type encodedPair struct {
	key   string
	raw   []byte
	daily bool
}

type Store struct {
	db   *bolt.DB
	path string
}

func openStore(path string) (*Store, error) {
	if path == "" {
		return nil, fmt.Errorf("aggregate: stats database path is empty")
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return nil, fmt.Errorf("aggregate: create stats directory: %w", err)
	}
	db, err := bolt.Open(path, 0o600, &bolt.Options{Timeout: 3 * time.Second})
	if err != nil {
		return nil, fmt.Errorf("aggregate: open %s: %w", path, err)
	}
	if err := db.Update(func(tx *bolt.Tx) error {
		for _, name := range [][]byte{metaBucket, hourlyBucket, dailyBucket} {
			if _, err := tx.CreateBucketIfNotExists(name); err != nil {
				return err
			}
		}
		meta := tx.Bucket(metaBucket)
		if meta.Get(schemaKey) == nil {
			if err := meta.Put(schemaKey, []byte(schemaVersion)); err != nil {
				return err
			}
		}
		return nil
	}); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("aggregate: initialise %s: %w", path, err)
	}
	return &Store{db: db, path: path}, nil
}

func (s *Store) Path() string { return s.path }

// clientSalt returns the per-store HMAC key used when client anonymization is
// enabled. It is created once and kept in the stats database, so hashed client
// identifiers remain stable across restarts without putting the salt in a
// separate file.
func (s *Store) clientSalt() ([]byte, error) {
	var salt []byte
	err := s.db.Update(func(tx *bolt.Tx) error {
		meta := tx.Bucket(metaBucket)
		if raw := meta.Get(clientSaltKey); raw != nil {
			salt = append([]byte(nil), raw...)
			return nil
		}
		raw := make([]byte, 32)
		if _, err := rand.Read(raw); err != nil {
			return err
		}
		if err := meta.Put(clientSaltKey, raw); err != nil {
			return err
		}
		salt = append([]byte(nil), raw...)
		return nil
	})
	return salt, err
}

func (s *Store) close() error {
	if s == nil || s.db == nil {
		return nil
	}
	return s.db.Close()
}

func (s *Store) readBucket(name []byte, key string) (*Bucket, error) {
	var out *Bucket
	err := s.db.View(func(tx *bolt.Tx) error {
		raw := tx.Bucket(name).Get([]byte(key))
		if raw == nil {
			return nil
		}
		out = &Bucket{}
		if err := json.Unmarshal(raw, out); err != nil {
			return err
		}
		out.ensureMaps()
		return nil
	})
	return out, err
}

func (s *Store) readHour(key string) (*Bucket, error) {
	return s.readBucket(hourlyBucket, key)
}

func (s *Store) readDaily(key string) (*Bucket, error) {
	return s.readBucket(dailyBucket, key)
}

// writeBuckets writes the current hour and any pending previous hours in one
// transaction. The flusher is the only writer, so one commit per interval is
// enough and keeps bbolt's fsync off the DNS path.
func (s *Store) writeBuckets(pairs []bucketPair) error {
	encoded := make([]encodedPair, 0, len(pairs))
	for _, pair := range pairs {
		if pair.bucket == nil {
			continue
		}
		raw, err := json.Marshal(pair.bucket)
		if err != nil {
			return fmt.Errorf("aggregate: encode %s: %w", pair.key, err)
		}
		encoded = append(encoded, encodedPair{key: pair.key, raw: raw, daily: pair.daily})
	}
	if len(encoded) == 0 {
		return nil
	}
	return s.db.Update(func(tx *bolt.Tx) error {
		hourly := tx.Bucket(hourlyBucket)
		daily := tx.Bucket(dailyBucket)
		for _, pair := range encoded {
			dst := hourly
			if pair.daily {
				dst = daily
			}
			if err := dst.Put([]byte(pair.key), pair.raw); err != nil {
				return err
			}
		}
		return nil
	})
}

func (s *Store) writeHour(key string, b *Bucket) error {
	return s.writeBuckets([]bucketPair{{key: key, bucket: b}})
}

func (s *Store) writeDaily(key string, b *Bucket) error {
	return s.writeBuckets([]bucketPair{{key: key, bucket: b, daily: true}})
}

func (s *Store) dailyExists(key string) (bool, error) {
	exists := false
	err := s.db.View(func(tx *bolt.Tx) error {
		exists = tx.Bucket(dailyBucket).Get([]byte(key)) != nil
		return nil
	})
	return exists, err
}

func (s *Store) allHourKeys() ([]string, error) {
	var keys []string
	err := s.db.View(func(tx *bolt.Tx) error {
		return tx.Bucket(hourlyBucket).ForEach(func(k, _ []byte) error {
			keys = append(keys, string(k))
			return nil
		})
	})
	return keys, err
}

func (s *Store) hourKeysForDay(day string) ([]string, error) {
	var keys []string
	err := s.db.View(func(tx *bolt.Tx) error {
		c := tx.Bucket(hourlyBucket).Cursor()
		prefix := []byte(day)
		for k, _ := c.Seek(prefix); k != nil && len(k) >= len(prefix) && string(k[:len(prefix)]) == day; k, _ = c.Next() {
			keys = append(keys, string(k))
		}
		return nil
	})
	return keys, err
}

func (s *Store) pruneHourly(cutoff time.Time) error {
	return s.db.Update(func(tx *bolt.Tx) error {
		b := tx.Bucket(hourlyBucket)
		c := b.Cursor()
		var doomed [][]byte
		for k, _ := c.First(); k != nil; k, _ = c.Next() {
			t, err := time.ParseInLocation("2006010215", string(k), time.UTC)
			if err == nil && t.Before(cutoff) {
				doomed = append(doomed, append([]byte(nil), k...))
			}
		}
		for _, k := range doomed {
			if err := b.Delete(k); err != nil {
				return err
			}
		}
		return nil
	})
}

func (s *Store) pruneDaily(cutoffDay time.Time) error {
	return s.db.Update(func(tx *bolt.Tx) error {
		b := tx.Bucket(dailyBucket)
		c := b.Cursor()
		var doomed [][]byte
		for k, _ := c.First(); k != nil; k, _ = c.Next() {
			t, err := time.ParseInLocation("20060102", string(k), time.UTC)
			if err == nil && t.Before(cutoffDay) {
				doomed = append(doomed, append([]byte(nil), k...))
			}
		}
		for _, k := range doomed {
			if err := b.Delete(k); err != nil {
				return err
			}
		}
		return nil
	})
}
