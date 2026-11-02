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
	Pins  []string
	Vault string
}
type store struct{ db *bolt.DB }

func openStore(dir string) (*store, error) {
	if err := os.MkdirAll(dir, 0700); err != nil {
		return nil, err
	}
	db, err := bolt.Open(filepath.Join(dir, "launchr.db"), 0600, &bolt.Options{Timeout: time.Second})
	if err != nil {
		return nil, err
	}
	err = db.Update(func(tx *bolt.Tx) error {
		for _, name := range []string{"clips", "data", "hashes", "settings", "rates"} {
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
	return &store{db}, nil
}
func idKey(id uint64) []byte { b := make([]byte, 8); binary.BigEndian.PutUint64(b, id); return b }
func (s *store) add(kind string, data []byte) error {
	if len(data) == 0 {
		return nil
	}
	sum := sha256.Sum256(append([]byte(kind+":"), data...))
	hash := hex.EncodeToString(sum[:])
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
		preview := "Image"
		if kind == "text" {
			preview = strings.Join(strings.Fields(string(data)), " ")
			r := []rune(preview)
			if len(r) > 160 {
				preview = string(r[:160]) + "…"
			}
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
		return tx.Bucket([]byte("data")).Put([]byte(hash), data)
	})
}
func (s *store) page(ctx context.Context, query string, before uint64, limit int) (out []clip, more bool, err error) {
	q := []byte(strings.ToLower(strings.TrimSpace(query)))
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
			if len(q) > 0 {
				hay := []byte(strings.ToLower(item.Preview))
				if item.Kind == "text" {
					hay = bytes.ToLower(tx.Bucket([]byte("data")).Get([]byte(item.Hash)))
				}
				if !bytes.Contains(hay, q) {
					continue
				}
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
	return s.db.Update(func(tx *bolt.Tx) error {
		if tx.Bucket([]byte("clips")).Get(idKey(item.ID)) == nil || !bytes.Equal(tx.Bucket([]byte("hashes")).Get([]byte(item.Hash)), idKey(item.ID)) {
			return fmt.Errorf("this clipboard item changed; refresh the history before deleting it")
		}
		for _, p := range []struct {
			bucket string
			key    []byte
		}{{"clips", idKey(item.ID)}, {"data", []byte(item.Hash)}, {"hashes", []byte(item.Hash)}} {
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
