package main

import (
	"bytes"
	"context"
	"encoding/json"
	"sort"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	bolt "go.etcd.io/bbolt"
)

// Index byte n-grams of Unicode-lowercased text, preserving substring searches
// (including one/two-byte queries). High-entropy large items use exact fallback
// so their index cannot grow without bound.
func searchGrams(data []byte) []byte {
	seen := make(map[uint32]struct{})
	var rolling uint32
	n := 0
	for len(data) > 0 {
		var encoded [utf8.UTFMax]byte
		var lower []byte
		if data[0] < utf8.RuneSelf {
			b := data[0]
			data = data[1:]
			if b >= 'A' && b <= 'Z' {
				b += 'a' - 'A'
			}
			encoded[0] = b
			lower = encoded[:1]
		} else {
			r, size := utf8.DecodeRune(data)
			data = data[size:]
			lower = utf8.AppendRune(encoded[:0], unicode.ToLower(r))
		}
		for _, b := range lower {
			rolling = (rolling<<8 | uint32(b)) & 0xffffff
			n = min(n+1, 3)
			for width := 1; width <= n; width++ {
				key := uint32(width)<<24 | rolling&(uint32(1)<<(8*width)-1)
				seen[key] = struct{}{}
				if len(seen) > 4096 {
					return []byte{0}
				}
			}
		}
	}
	keys := make([]uint32, 0, len(seen))
	for key := range seen {
		keys = append(keys, key)
	}
	sort.Slice(keys, func(i, j int) bool { return keys[i] < keys[j] })
	var out []byte
	for _, key := range keys {
		width := int(key >> 24)
		out = append(out, byte(width))
		for i := width - 1; i >= 0; i-- {
			out = append(out, byte(key>>(8*i)))
		}
	}
	return out
}
func putSearchIndex(tx *bolt.Tx, hash string, grams []byte) error {
	if tx.Bucket([]byte("index-records")).Get([]byte(hash)) != nil {
		return nil
	}
	if len(grams) == 1 && grams[0] == 0 {
		if err := tx.Bucket([]byte("index-overflow")).Put([]byte(hash), []byte{1}); err != nil {
			return err
		}
	} else {
		for rest := grams; len(rest) > 0; {
			n := int(rest[0])
			key := rest[1 : 1+n]
			rest = rest[1+n:]
			bucket, err := tx.Bucket([]byte("index-grams")).CreateBucketIfNotExists(key)
			if err != nil {
				return err
			}
			if err = bucket.Put([]byte(hash), []byte{1}); err != nil {
				return err
			}
			counts := tx.Bucket([]byte("index-counts"))
			count := uint64(0)
			if value := counts.Get(key); len(value) == 8 {
				count = keyID(value)
			}
			if err = counts.Put(key, idKey(count+1)); err != nil {
				return err
			}
		}
	}
	return tx.Bucket([]byte("index-records")).Put([]byte(hash), grams)
}

func deleteSearchIndex(tx *bolt.Tx, hash string) error {
	grams := tx.Bucket([]byte("index-records")).Get([]byte(hash))
	if len(grams) == 1 && grams[0] == 0 {
		if err := tx.Bucket([]byte("index-overflow")).Delete([]byte(hash)); err != nil {
			return err
		}
	} else {
		for len(grams) > 0 {
			n := int(grams[0])
			key := grams[1 : 1+n]
			grams = grams[1+n:]
			if b := tx.Bucket([]byte("index-grams")).Bucket(key); b != nil {
				if err := b.Delete([]byte(hash)); err != nil {
					return err
				}
				k, _ := b.Cursor().First()
				if k == nil {
					if err := tx.Bucket([]byte("index-grams")).DeleteBucket(key); err != nil {
						return err
					}
				}
				counts := tx.Bucket([]byte("index-counts"))
				count := uint64(0)
				if value := counts.Get(key); len(value) == 8 {
					count = keyID(value)
				}
				if count <= 1 {
					if err := counts.Delete(key); err != nil {
						return err
					}
				} else {
					if err := counts.Put(key, idKey(count-1)); err != nil {
						return err
					}
				}
			}
		}
	}
	return tx.Bucket([]byte("index-records")).Delete([]byte(hash))
}

// Legacy archives are indexed incrementally, outside the UI thread, with one
// payload per read transaction and cancellation between records.
func (s *store) ensureIndex(ctx context.Context) error {
	if s.indexComplete.Load() {
		return ctx.Err()
	}
	for !s.indexMu.TryLock() {
		if !waitJob(ctx, 10*time.Millisecond) {
			return ctx.Err()
		}
	}
	defer s.indexMu.Unlock()
	if s.indexComplete.Load() {
		return ctx.Err()
	}
	var after uint64
	type update struct {
		hash  string
		grams []byte
	}
	pending := make([]update, 0, 32)
	flush := func() error {
		if len(pending) == 0 {
			return nil
		}
		err := s.db.Update(func(tx *bolt.Tx) error {
			for _, item := range pending {
				if tx.Bucket([]byte("hashes")).Get([]byte(item.hash)) == nil {
					continue
				}
				if err := putSearchIndex(tx, item.hash, item.grams); err != nil {
					return err
				}
			}
			return nil
		})
		pending = pending[:0]
		return err
	}
	for {
		if err := ctx.Err(); err != nil {
			return err
		}
		var item clip
		var data []byte
		found := false
		indexed := false
		err := s.db.View(func(tx *bolt.Tx) error {
			cur := tx.Bucket([]byte("clips")).Cursor()
			k, v := cur.Seek(idKey(after + 1))
			if k == nil {
				return nil
			}
			if err := json.Unmarshal(v, &item); err != nil {
				return err
			}
			found = true
			indexed = tx.Bucket([]byte("index-records")).Get([]byte(item.Hash)) != nil
			if !indexed {
				if item.Kind == "text" {
					data = bytes.Clone(tx.Bucket([]byte("data")).Get([]byte(item.Hash)))
				} else {
					data = []byte(item.Preview)
				}
			}
			return nil
		})
		if err != nil {
			return err
		}
		if !found {
			break
		}
		after = item.ID
		if indexed {
			continue
		}
		grams := searchGrams(data)
		if err = ctx.Err(); err != nil {
			return err
		}
		pending = append(pending, update{item.Hash, grams})
		if len(pending) == 32 {
			if err := flush(); err != nil {
				return err
			}
		}
	}
	if err := flush(); err != nil {
		return err
	}
	err := s.db.Update(func(tx *bolt.Tx) error { return tx.Bucket([]byte("index-state")).Put([]byte("complete"), []byte{1}) })
	if err == nil {
		s.indexComplete.Store(true)
	}
	return err
}

func containsFold(ctx context.Context, data, query []byte) (bool, error) {
	// KMP consumes lowercased UTF-8 incrementally rather than allocating a
	// lowercased copy of an arbitrarily large clipboard item.
	failure := make([]int, len(query))
	for i, j := 1, 0; i < len(query); i++ {
		for j > 0 && query[i] != query[j] {
			j = failure[j-1]
		}
		if query[i] == query[j] {
			j++
		}
		failure[i] = j
	}
	j, processed := 0, 0
	for len(data) > 0 {
		if processed%4096 == 0 {
			if err := ctx.Err(); err != nil {
				return false, err
			}
		}
		processed++
		r, n := utf8.DecodeRune(data)
		data = data[n:]
		var buf [4]byte
		for _, b := range utf8.AppendRune(buf[:0], unicode.ToLower(r)) {
			for j > 0 && b != query[j] {
				j = failure[j-1]
			}
			if b == query[j] {
				j++
			}
			if j == len(query) {
				return true, nil
			}
		}
	}
	return false, ctx.Err()
}

func (s *store) searchPage(ctx context.Context, query string, before uint64, limit int) (out []clip, more bool, err error) {
	if err = s.ensureIndex(ctx); err != nil {
		return
	}
	q := []byte(strings.ToLower(strings.TrimSpace(query)))
	gram := q[max(0, len(q)-3):]
	// Prefer the rarest query gram so a common suffix cannot force a scan of
	// otherwise unrelated text payloads. Counts are maintained transactionally.
	err = s.db.View(func(tx *bolt.Tx) error {
		counts := tx.Bucket([]byte("index-counts"))
		best := ^uint64(0)
		width := min(3, len(q))
		for i := 0; i+width <= len(q); i++ {
			if i%4096 == 0 {
				if err := ctx.Err(); err != nil {
					return err
				}
			}
			candidate := q[i : i+width]
			value := counts.Get(candidate)
			count := uint64(0)
			if len(value) == 8 {
				count = keyID(value)
			}
			if count < best {
				best, gram = count, candidate
			}
			if best == 0 {
				break
			}
		}
		return nil
	})
	if err != nil {
		return
	}
	var ids []uint64
	for _, overflow := range []bool{false, true} {
		var after []byte
		for {
			if err = ctx.Err(); err != nil {
				return
			}
			count := 0
			err = s.db.View(func(tx *bolt.Tx) error {
				bucket := tx.Bucket([]byte("index-overflow"))
				if !overflow {
					bucket = tx.Bucket([]byte("index-grams")).Bucket(gram)
				}
				if bucket == nil {
					return nil
				}
				cur := bucket.Cursor()
				k, _ := cur.First()
				if after != nil {
					k, _ = cur.Seek(after)
					if bytes.Equal(k, after) {
						k, _ = cur.Next()
					}
				}
				for ; k != nil && count < 128; k, _ = cur.Next() {
					count++
					after = append(after[:0], k...)
					id := tx.Bucket([]byte("hashes")).Get(k)
					if len(id) == 8 {
						n := keyID(id)
						if before == 0 || n < before {
							ids = append(ids, n)
						}
					}
				}
				return nil
			})
			if err != nil {
				return
			}
			if count < 128 {
				break
			}
		}
	}
	sort.Slice(ids, func(i, j int) bool { return ids[i] > ids[j] })
	for _, id := range ids {
		if err = ctx.Err(); err != nil {
			return
		}
		var item clip
		match := false
		err = s.db.View(func(tx *bolt.Tx) error {
			meta := tx.Bucket([]byte("clips")).Get(idKey(id))
			if meta == nil {
				return nil
			}
			if e := json.Unmarshal(meta, &item); e != nil {
				return e
			}
			data := []byte(item.Preview)
			if item.Kind == "text" {
				data = tx.Bucket([]byte("data")).Get([]byte(item.Hash))
			}
			var e error
			match, e = containsFold(ctx, data, q)
			return e
		})
		if err != nil {
			return
		}
		if !match {
			continue
		}
		if len(out) == limit {
			more = true
			break
		}
		out = append(out, item)
	}
	return
}
