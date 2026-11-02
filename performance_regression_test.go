package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"image"
	"image/png"
	"math/rand"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	bolt "go.etcd.io/bbolt"
)

func TestIndexedSubstringSemanticsAndRecency(t *testing.T) {
	s := testStore(t)
	texts := []string{"Résumé 世界 AlphaBeta", "x ALPHABETA y", "abc gap bcd"}
	for _, v := range texts {
		if err := s.add("text", []byte(v)); err != nil {
			t.Fatal(err)
		}
	}
	for query, want := range map[string]int{"RÉSUMÉ": 1, "世界": 1, "phaB": 2, "a": 3, "x ": 1, "abcd": 0, "not present": 0} {
		rows, _, err := s.page(context.Background(), query, 0, 100)
		if err != nil || len(rows) != want {
			t.Fatalf("%q: %d rows, %v", query, len(rows), err)
		}
	}
	if err := s.add("text", []byte(texts[0])); err != nil {
		t.Fatal(err)
	}
	first, more, err := s.page(context.Background(), "alphabeta", 0, 1)
	if err != nil || len(first) != 1 || !more || first[0].Preview != texts[0] {
		t.Fatalf("first: %v %v %v", first, more, err)
	}
	next, more, err := s.page(context.Background(), "alphabeta", first[0].ID, 1)
	if err != nil || len(next) != 1 || more || next[0].Preview != texts[1] {
		t.Fatalf("next: %v %v %v", next, more, err)
	}
	if err := s.delete(first[0]); err != nil {
		t.Fatal(err)
	}
	rows, _, err := s.page(context.Background(), "résumé", 0, 100)
	if err != nil || len(rows) != 0 {
		t.Fatalf("deleted index result: %v %v", rows, err)
	}
}

func TestLegacyIndexMigrationAndRestart(t *testing.T) {
	dir := t.TempDir()
	s, err := openStore(dir)
	if err != nil {
		t.Fatal(err)
	}
	if err = s.add("text", []byte("legacy résumé needle")); err != nil {
		t.Fatal(err)
	}
	err = s.db.Update(func(tx *bolt.Tx) error {
		for _, name := range []string{"index-grams", "index-counts", "index-records", "index-overflow", "index-state"} {
			if err := tx.DeleteBucket([]byte(name)); err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	s.db.Close()
	for i := 0; i < 2; i++ {
		s, err = openStore(dir)
		if err != nil {
			t.Fatal(err)
		}
		rows, _, err := s.page(context.Background(), "RÉSUMÉ", 0, 10)
		if err != nil || len(rows) != 1 {
			t.Fatalf("restart %d: %v %v", i, rows, err)
		}
		s.db.Close()
	}
}

func TestIndexOverflowExactFallback(t *testing.T) {
	s := testStore(t)
	rng := rand.New(rand.NewSource(42))
	data := make([]byte, 30000)
	for i := range data {
		data[i] = byte(32 + rng.Intn(90))
	}
	if !bytes.Equal(searchGrams(data), []byte{0}) {
		t.Fatal("fixture did not overflow")
	}
	if err := s.add("text", data); err != nil {
		t.Fatal(err)
	}
	query := string(data[10000:10025])
	rows, _, err := s.page(context.Background(), query, 0, 10)
	if err != nil || len(rows) != 1 {
		t.Fatalf("fallback: %v %v", rows, err)
	}
	if err = s.delete(rows[0]); err != nil {
		t.Fatal(err)
	}
	rows, _, err = s.page(context.Background(), query, 0, 10)
	if err != nil || len(rows) != 0 {
		t.Fatalf("deleted fallback: %v %v", rows, err)
	}
}

func TestBoundedUnicodePreviewsAndDuplicatePayload(t *testing.T) {
	s := testStore(t)
	data := []byte(" \tRésumé\n\n" + strings.Repeat("界", 10000))
	if err := s.add("text", data); err != nil {
		t.Fatal(err)
	}
	rows, _, _ := s.page(context.Background(), "", 0, 1)
	if len([]rune(rows[0].Preview)) != 161 || !strings.HasPrefix(rows[0].Preview, "Résumé 界") {
		t.Fatal(rows[0].Preview)
	}
	if err := s.add("text", data); err != nil {
		t.Fatal(err)
	}
	_, text, err := s.previewData(context.Background(), rows[0])
	if err != nil || !strings.HasSuffix(text, "Copy restores the full text.") {
		t.Fatalf("preview: %v %q", err, text)
	}
	restored, err := s.data(rows[0])
	if err != nil || !bytes.Equal(restored, data) {
		t.Fatal("full payload changed")
	}
}

func TestStoredThumbnailSurvivesRestartAndDeletes(t *testing.T) {
	dir := t.TempDir()
	s, err := openStore(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { s.db.Close() }()
	var buf bytes.Buffer
	if err := png.Encode(&buf, image.NewNRGBA(image.Rect(0, 0, 1000, 500))); err != nil {
		t.Fatal(err)
	}
	if err := s.add("image", buf.Bytes()); err != nil {
		t.Fatal(err)
	}
	rows, _, _ := s.page(context.Background(), "", 0, 1)
	s.db.Close()
	s, err = openStore(dir)
	if err != nil {
		t.Fatal(err)
	}
	err = s.db.View(func(tx *bolt.Tx) error {
		thumb := tx.Bucket([]byte("thumbnails")).Get([]byte(rows[0].Hash))
		config, err := png.DecodeConfig(bytes.NewReader(thumb))
		if err != nil {
			return err
		}
		if config.Width != 480 || config.Height != 240 {
			return fmt.Errorf("thumbnail dimensions: %v", config)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err = s.previewData(context.Background(), rows[0]); err != nil {
		t.Fatal(err)
	}
	if err = s.delete(rows[0]); err != nil {
		t.Fatal(err)
	}
	if _, _, err = s.previewData(context.Background(), rows[0]); err == nil {
		t.Fatal("deleted preview survived")
	}
}

func TestCaptureQueueNeverBlocksAndKeepsNewest(t *testing.T) {
	q := &captureQueue{wake: make(chan struct{}, 1)}
	for i := 0; i < 40; i++ {
		q.offer(capture{kind: fmt.Sprint(i), data: []byte{1}})
	}
	if len(q.items) != 32 || q.bytes != 32 {
		t.Fatal("item budget")
	}
	c, _ := q.pop()
	if c.kind != "8" {
		t.Fatal("oldest snapshot not evicted")
	}
	q = &captureQueue{wake: make(chan struct{}, 1)}
	data := make([]byte, maxCaptureBytes/2+1)
	q.offer(capture{kind: "old", data: data})
	if !q.offer(capture{kind: "new", data: data}) {
		t.Fatal("byte budget not enforced")
	}
	c, _ = q.pop()
	if c.kind != "new" {
		t.Fatal("newest snapshot lost")
	}
}

func TestPreviewPendingRequestIsReplacedAndCancelled(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	a := &app{ctx: ctx, previewJobs: make(chan previewJob, 1), rows: []result{{Kind: "clip", Clip: clip{ID: 1}}, {Kind: "clip", Clip: clip{ID: 2}}}}
	a.loadPreview()
	first := <-a.previewJobs
	a.previewJobs <- first
	a.chosen = 1
	a.loadPreview()
	if first.ctx.Err() == nil {
		t.Fatal("previous preview was not cancelled")
	}
	latest := <-a.previewJobs
	if latest.item.ID != 2 || latest.ctx.Err() != nil {
		t.Fatal("latest preview missing")
	}
	a.hide()
	if latest.ctx.Err() == nil {
		t.Fatal("hidden preview was not cancelled")
	}
}

func TestStaleCurrencyIsImmediateAndRefreshIsDeduplicated(t *testing.T) {
	s := testStore(t)
	if err := s.save("rates", "USD/NGN", rate{Date: "2026-10-30", Base: "USD", Quote: "NGN", Rate: 100, Fetched: time.Now().Add(-24 * time.Hour)}); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	a := &app{db: s, ctx: ctx}
	var calls atomic.Int32
	entered, release := make(chan struct{}, 1), make(chan struct{})
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		select {
		case entered <- struct{}{}:
		default:
		}
		<-release
		json.NewEncoder(w).Encode(rate{Date: "2026-11-02", Base: "USD", Quote: "NGN", Rate: 200})
	}))
	defer server.Close()
	q := conversion{Amount: 2, From: "USD", To: "NGN"}
	value, detail, err := a.currencyResult(ctx, q, server.Client(), server.URL)
	if err != nil || value != "200.00 NGN" || !strings.HasPrefix(detail, "Cached rate") || calls.Load() != 0 {
		t.Fatalf("cached: %q %q %v", value, detail, err)
	}
	a.refreshCurrency(q, server.Client(), server.URL)
	select {
	case <-entered:
	case <-time.After(3 * time.Second):
		close(release)
		t.Fatal("refresh did not start")
	}
	for i := 0; i < 20; i++ {
		a.refreshCurrency(q, server.Client(), server.URL)
	}
	close(release)
	a.wg.Wait()
	if calls.Load() != 1 {
		t.Fatalf("%d refreshes", calls.Load())
	}
	value, detail, err = a.currencyResult(ctx, q, server.Client(), server.URL)
	if err != nil || value != "400.00 NGN" || !strings.HasPrefix(detail, "Rate from") {
		t.Fatalf("refreshed: %q %q %v", value, detail, err)
	}
}

func TestConcurrentHistorySearchAndWrites(t *testing.T) {
	s := testStore(t)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	var wg sync.WaitGroup
	for worker := 0; worker < 3; worker++ {
		wg.Add(1)
		go func(worker int) {
			defer wg.Done()
			for i := 0; i < 15; i++ {
				if err := s.add("text", []byte(fmt.Sprintf("worker %d item %d needle", worker, i))); err != nil {
					t.Error(err)
					return
				}
				if _, _, err := s.page(ctx, "needle", 0, 10); err != nil {
					t.Error(err)
					return
				}
			}
		}(worker)
	}
	wg.Wait()
}
