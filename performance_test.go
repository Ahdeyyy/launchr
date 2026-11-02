package main

import (
	"bytes"
	"context"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"image"
	"image/png"
	"strings"
	"testing"
	"time"

	"github.com/egoist/mygo"
	bolt "go.etcd.io/bbolt"
)

// Synthetic performance-review workloads. These use temporary databases only.
func auditStore(b *testing.B) *store {
	b.Helper()
	s, err := openStore(b.TempDir())
	if err != nil {
		b.Fatal(err)
	}
	b.Cleanup(func() { s.db.Close() })
	return s
}

func BenchmarkClipboardSearchMiss10K(b *testing.B) {
	s := auditStore(b)
	err := s.db.Update(func(tx *bolt.Tx) error {
		for i := 1; i <= 10000; i++ {
			hash := fmt.Sprintf("entry-%d", i)
			item := clip{ID: uint64(i), Hash: hash, Kind: "text", Preview: "clipboard record", Size: 4096, Time: time.Now()}
			meta, err := json.Marshal(item)
			if err != nil {
				return err
			}
			if err := tx.Bucket([]byte("clips")).Put(idKey(uint64(i)), meta); err != nil {
				return err
			}
			if err := tx.Bucket([]byte("data")).Put([]byte(hash), bytes.Repeat([]byte("A"), 4096)); err != nil {
				return err
			}
			if err := tx.Bucket([]byte("hashes")).Put([]byte(hash), idKey(uint64(i))); err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		b.Fatal(err)
	}
	if err := s.ensureIndex(context.Background()); err != nil {
		b.Fatal(err)
	}
	b.ReportAllocs()
	b.ResetTimer()
	for b.Loop() {
		if _, _, err := s.page(context.Background(), "no-such-record", 0, 100); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkClipboardAddLargeText(b *testing.B) {
	s := auditStore(b)
	data := []byte(strings.Repeat("A long clipboard line\n", 400000))
	if err := s.add("text", data); err != nil {
		b.Fatal(err)
	}
	b.ReportAllocs()
	b.ResetTimer()
	for b.Loop() {
		if err := s.add("text", data); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkClipboardAddNewLargeText(b *testing.B) {
	s := auditStore(b)
	data := []byte(strings.Repeat("A long clipboard line\n", 400000))
	var sequence uint64
	b.ReportAllocs()
	b.ResetTimer()
	for b.Loop() {
		sequence++
		binary.LittleEndian.PutUint64(data[len(data)-8:], sequence)
		if err := s.add("text", data); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkClipboardPreview4K(b *testing.B) {
	s := auditStore(b)
	var encoded bytes.Buffer
	if err := png.Encode(&encoded, image.NewRGBA(image.Rect(0, 0, 3840, 2160))); err != nil {
		b.Fatal(err)
	}
	if err := s.add("image", encoded.Bytes()); err != nil {
		b.Fatal(err)
	}
	clips, _, err := s.page(context.Background(), "", 0, 1)
	if err != nil {
		b.Fatal(err)
	}
	b.ReportAllocs()
	b.ResetTimer()
	for b.Loop() {
		if _, _, err := s.previewData(context.Background(), clips[0]); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkFilterApps1K(b *testing.B) {
	a := &app{}
	for i := 0; i < 1000; i++ {
		a.apps = append(a.apps, mygo.InstalledApp{ID: fmt.Sprint(i), Name: fmt.Sprintf("Application tool %d", i)})
	}
	b.ReportAllocs()
	b.ResetTimer()
	for b.Loop() {
		a.rows = a.rows[:0]
		a.filterApps("application tool")
	}
}
