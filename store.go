package main

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	bolt "go.etcd.io/bbolt"
)

type clip struct {
	ID                  uint64
	Hash, Kind, Preview string
	Size                int
	Time                time.Time
}
type settings struct {
	Pins       []string
	Vault      string
	Density    string
	WindowSize string
}
type store struct {
	db            *bolt.DB
	indexMu       sync.Mutex
	contentMu     sync.Mutex
	indexComplete atomic.Bool
}

func openStore(dir string) (*store, error) {
	if err := os.MkdirAll(dir, 0700); err != nil {
		return nil, err
	}
	db, err := bolt.Open(filepath.Join(dir, "launchr.db"), 0600, &bolt.Options{Timeout: time.Second})
	if err != nil {
		return nil, err
	}
	err = db.Update(func(tx *bolt.Tx) error {
		for _, name := range []string{"clips", "data", "hashes", "settings", "rates", "thumbnails", "index-grams", "index-counts", "index-records", "index-overflow", "index-state"} {
			if _, err := tx.CreateBucketIfNotExists([]byte(name)); err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		db.Close()
		return nil, err
	}
	s := &store{db: db}
	_ = db.View(func(tx *bolt.Tx) error {
		s.indexComplete.Store(tx.Bucket([]byte("index-state")).Get([]byte("complete")) != nil)
		return nil
	})
	return s, nil
}
func idKey(id uint64) []byte  { b := make([]byte, 8); binary.BigEndian.PutUint64(b, id); return b }
func keyID(key []byte) uint64 { return binary.BigEndian.Uint64(key) }
func (s *store) add(kind string, data []byte) error {
	s.contentMu.Lock()
	defer s.contentMu.Unlock()
	if len(data) == 0 {
		return nil
	}
	hasher := sha256.New()
	hasher.Write([]byte(kind + ":"))
	hasher.Write(data)
	hash := hex.EncodeToString(hasher.Sum(nil))
	var previous clip
	err := s.db.View(func(tx *bolt.Tx) error {
		if old := tx.Bucket([]byte("hashes")).Get([]byte(hash)); old != nil {
			return json.Unmarshal(tx.Bucket([]byte("clips")).Get(old), &previous)
		}
		return nil
	})
	if err != nil {
		return err
	}
	preview := previous.Preview
	var grams, thumb []byte
	if previous.Hash == "" {
		preview = "Image"
		if kind == "text" {
			var truncated bool
			preview, truncated = textSummary(data, 160, true)
			if truncated {
				preview += "…"
			}
			grams = searchGrams(data)
		} else {
			grams = searchGrams([]byte(preview))
			thumb, _ = makeThumbnail(data)
		}
	}
	return s.db.Update(func(tx *bolt.Tx) error {
		entries, hashes := tx.Bucket([]byte("clips")), tx.Bucket([]byte("hashes"))
		if old := hashes.Get([]byte(hash)); old != nil {
			if err := entries.Delete(old); err != nil {
				return err
			}
		}
		id, err := entries.NextSequence()
		if err != nil {
			return err
		}
		meta, err := json.Marshal(clip{id, hash, kind, preview, len(data), time.Now()})
		if err != nil {
			return err
		}
		if err = entries.Put(idKey(id), meta); err != nil {
			return err
		}
		if err = hashes.Put([]byte(hash), idKey(id)); err != nil {
			return err
		}
		if tx.Bucket([]byte("data")).Get([]byte(hash)) == nil {
			if err = tx.Bucket([]byte("data")).Put([]byte(hash), data); err != nil {
				return err
			}
			if len(thumb) > 0 {
				if err = tx.Bucket([]byte("thumbnails")).Put([]byte(hash), thumb); err != nil {
					return err
				}
			}
		}
		if previous.Hash == "" {
			return putSearchIndex(tx, hash, grams)
		}
		return nil
	})
}
func (s *store) page(ctx context.Context, query string, before uint64, limit int) (out []clip, more bool, err error) {
	q := []byte(strings.ToLower(strings.TrimSpace(query)))
	if len(q) > 0 {
		return s.searchPage(ctx, query, before, limit)
	}
	err = s.db.View(func(tx *bolt.Tx) error {
		cur := tx.Bucket([]byte("clips")).Cursor()
		k, v := cur.Last()
		if before > 0 {
			k, v = cur.Seek(idKey(before))
			if k == nil {
				k, v = cur.Last()
			} else {
				k, v = cur.Prev()
			}
		}
		for ; k != nil; k, v = cur.Prev() {
			if err := ctx.Err(); err != nil {
				return err
			}
			var item clip
			if err := json.Unmarshal(v, &item); err != nil {
				return err
			}
			if len(out) == limit {
				more = true
				break
			}
			out = append(out, item)
		}
		return nil
	})
	return
}
func (s *store) data(item clip) (data []byte, err error) {
	err = s.db.View(func(tx *bolt.Tx) error {
		b := tx.Bucket([]byte("data")).Get([]byte(item.Hash))
		if b == nil {
			return fmt.Errorf("clipboard item no longer exists")
		}
		data = bytes.Clone(b)
		return nil
	})
	return
}
func (s *store) delete(item clip) error {
	s.contentMu.Lock()
	defer s.contentMu.Unlock()
	return s.db.Update(func(tx *bolt.Tx) error {
		if tx.Bucket([]byte("clips")).Get(idKey(item.ID)) == nil || !bytes.Equal(tx.Bucket([]byte("hashes")).Get([]byte(item.Hash)), idKey(item.ID)) {
			return fmt.Errorf("this clipboard item changed; refresh the history before deleting it")
		}
		if err := deleteSearchIndex(tx, item.Hash); err != nil {
			return err
		}
		for _, p := range []struct {
			bucket string
			key    []byte
		}{{"clips", idKey(item.ID)}, {"data", []byte(item.Hash)}, {"hashes", []byte(item.Hash)}, {"thumbnails", []byte(item.Hash)}} {
			if err := tx.Bucket([]byte(p.bucket)).Delete(p.key); err != nil {
				return err
			}
		}
		return nil
	})
}
func (s *store) load(bucket, key string, value any) error {
	return s.db.View(func(tx *bolt.Tx) error {
		b := tx.Bucket([]byte(bucket)).Get([]byte(key))
		if b == nil {
			return os.ErrNotExist
		}
		return json.Unmarshal(b, value)
	})
}
func (s *store) save(bucket, key string, value any) error {
	b, err := json.Marshal(value)
	if err != nil {
		return err
	}
	return s.db.Update(func(tx *bolt.Tx) error { return tx.Bucket([]byte(bucket)).Put([]byte(key), b) })
}
