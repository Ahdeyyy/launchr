package main

import (
	"bytes"
	"context"
	"fmt"
	"image"
	"image/png"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	"github.com/egoist/mygo/ui"
	bolt "go.etcd.io/bbolt"
	"golang.org/x/image/draw"
)

func textSummary(data []byte, limit int, collapse bool) (string, bool) {
	var out strings.Builder
	count, space := 0, false
	for len(data) > 0 {
		r, n := utf8.DecodeRune(data)
		data = data[n:]
		if collapse && unicode.IsSpace(r) {
			if count > 0 {
				space = true
			}
			continue
		}
		if space {
			if count == limit {
				return out.String(), true
			}
			out.WriteByte(' ')
			count++
			space = false
		}
		if count == limit {
			return out.String(), true
		}
		out.WriteRune(r)
		count++
	}
	return out.String(), false
}

func makeThumbnail(data []byte) ([]byte, error) {
	config, _, err := image.DecodeConfig(bytes.NewReader(data))
	if err != nil {
		return nil, err
	}
	if config.Width <= 0 || config.Height <= 0 || config.Width > maxCaptureBytes/4 || config.Height > maxCaptureBytes/(4*config.Width) {
		return nil, fmt.Errorf("image exceeds 64 MiB decoded preview budget; copy to restore it")
	}
	img, _, err := image.Decode(bytes.NewReader(data))
	if err != nil {
		return nil, err
	}
	w, h := config.Width, config.Height
	scale := min(1.0, 480.0/float64(max(w, h)))
	dst := image.NewNRGBA(image.Rect(0, 0, max(1, int(float64(w)*scale)), max(1, int(float64(h)*scale))))
	draw.NearestNeighbor.Scale(dst, dst.Bounds(), img, img.Bounds(), draw.Src, nil)
	var out bytes.Buffer
	err = png.Encode(&out, dst)
	return out.Bytes(), err
}

func (s *store) previewData(ctx context.Context, item clip) (*ui.Bitmap, string, error) {
	if item.Kind != "image" {
		var text string
		err := s.db.View(func(tx *bolt.Tx) error {
			data := tx.Bucket([]byte("data")).Get([]byte(item.Hash))
			if data == nil {
				return fmt.Errorf("clipboard item no longer exists")
			}
			var truncated bool
			text, truncated = textSummary(data, 4000, false)
			if truncated {
				text += "\n… Preview truncated. Copy restores the full text."
			}
			return ctx.Err()
		})
		return nil, text, err
	}
	var thumb []byte
	err := s.db.View(func(tx *bolt.Tx) error {
		thumb = bytes.Clone(tx.Bucket([]byte("thumbnails")).Get([]byte(item.Hash)))
		return ctx.Err()
	})
	if err != nil {
		return nil, "", err
	}
	if len(thumb) == 0 {
		data, err := s.data(item)
		if err != nil {
			return nil, "", err
		}
		if err = ctx.Err(); err != nil {
			return nil, "", err
		}
		thumb, err = makeThumbnail(data)
		if err != nil {
			return nil, "", err
		}
		if err = ctx.Err(); err != nil {
			return nil, "", err
		}
		err = s.db.Update(func(tx *bolt.Tx) error {
			if tx.Bucket([]byte("data")).Get([]byte(item.Hash)) == nil {
				return fmt.Errorf("clipboard item no longer exists")
			}
			return tx.Bucket([]byte("thumbnails")).Put([]byte(item.Hash), thumb)
		})
		if err != nil {
			return nil, "", err
		}
	}
	img, err := png.Decode(bytes.NewReader(thumb))
	if err != nil {
		return nil, "", err
	}
	return ui.NewBitmap(img), "", ctx.Err()
}

type previewJob struct {
	ctx        context.Context
	item       clip
	generation uint64
}

func (a *app) cancelPreview() {
	if a.previewCancel != nil {
		a.previewCancel()
		a.previewCancel = nil
	}
	a.previewGeneration++
	a.previewID = 0
}

func (a *app) startPreviewWorker() {
	a.previewJobs = make(chan previewJob, 1)
	a.background(func() {
		for {
			select {
			case <-a.ctx.Done():
				return
			case job := <-a.previewJobs:
				if job.ctx.Err() != nil {
					continue
				}
				bitmap, text, err := a.db.previewData(job.ctx, job.item)
				if job.ctx.Err() != nil {
					continue
				}
				a.update(func() {
					if a.previewGeneration != job.generation || a.previewID != job.item.ID {
						return
					}
					if err != nil {
						a.status = err.Error()
						return
					}
					a.preview, a.previewText = bitmap, text
				})
			}
		}
	})
}

func (a *app) loadPreview() {
	r, ok := a.selected()
	if !ok || r.Kind != "clip" || r.Clip.ID == a.previewID || a.previewJobs == nil {
		return
	}
	a.cancelPreview()
	a.previewID = r.Clip.ID
	a.preview, a.previewText = nil, ""
	ctx, cancel := context.WithCancel(a.ctx)
	a.previewCancel = cancel
	job := previewJob{ctx: ctx, item: r.Clip, generation: a.previewGeneration}
	select {
	case <-a.previewJobs:
	default:
	}
	a.previewJobs <- job // one UI producer; the previous pending job was removed
}

func (a *app) refreshHistory() {
	if a.refreshPending {
		return
	}
	a.refreshPending = true
	a.background(func() {
		if !waitJob(a.ctx, 150*time.Millisecond) {
			return
		}
		a.update(func() {
			a.refreshPending = false
			if a.tab != "Clipboard" || !a.w.IsVisible() {
				return
			}
			if a.busy {
				a.historyDirty = true
				return
			}
			if len(a.rows) > 100 || a.chosen >= 90 {
				a.status = "New clipboard items saved. Reopen Clipboard to refresh."
				return
			}
			a.searchWithOptions(false, false, true)
		})
	})
}
