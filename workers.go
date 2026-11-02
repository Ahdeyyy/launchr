package main

import (
	"context"
	"fmt"
	"runtime"
	"sync"
	"time"

	"github.com/egoist/mygo/ui"
)

// Capture overload keeps the newest snapshots within a 64 MiB/32-item budget.
// Notification coalescing avoids repeatedly reading the same clipboard owner.
type captureQueue struct {
	mu    sync.Mutex
	items []capture
	bytes int
	wake  chan struct{}
}

func (q *captureQueue) offer(c capture) (dropped bool) {
	q.mu.Lock()
	defer q.mu.Unlock()
	if len(c.data) > maxCaptureBytes {
		return true
	}
	for len(q.items) > 0 && (len(q.items) >= 32 || q.bytes+len(c.data) > maxCaptureBytes) {
		q.bytes -= len(q.items[0].data)
		q.items[0] = capture{}
		q.items = q.items[1:]
		dropped = true
	}
	q.items = append(q.items, c)
	q.bytes += len(c.data)
	select {
	case q.wake <- struct{}{}:
	default:
	}
	return
}

func (q *captureQueue) pop() (capture, bool) {
	q.mu.Lock()
	defer q.mu.Unlock()
	if len(q.items) == 0 {
		return capture{}, false
	}
	c := q.items[0]
	q.bytes -= len(c.data)
	q.items[0] = capture{}
	q.items = q.items[1:]
	return c, true
}

func (a *app) startCaptureWorkers() {
	a.captureSignals = make(chan struct{}, 1)
	q := &captureQueue{wake: make(chan struct{}, 1)}
	a.background(func() {
		runtime.LockOSThread()
		defer runtime.UnlockOSThread()
		var last uint32
		for {
			select {
			case <-a.ctx.Done():
				return
			case <-a.captureSignals:
			}
			epoch := a.captureEpoch.Load()
			if a.capturePaused.Load() {
				continue
			}
			if seq := nativeClipboardSequence(); seq != 0 && seq == last {
				continue
			}
			var c capture
			var seq uint32
			var err error
			for attempt := 0; attempt < 3; attempt++ {
				c, seq, err = readNativeClipboard()
				if err == nil {
					break
				}
				if !waitJob(a.ctx, 15*time.Millisecond) {
					return
				}
			}
			if err != nil {
				a.clipboardError(err)
				continue
			}
			if seq != 0 && seq == last {
				continue
			}
			last = seq
			if len(c.data) == 0 || a.capturePaused.Load() || epoch != a.captureEpoch.Load() {
				continue
			}
			c.epoch = epoch
			if q.offer(c) {
				a.clipboardError(fmt.Errorf("capture backlog exceeded its budget; kept the newest clipboard items"))
			}
		}
	})
	a.background(func() {
		for {
			select {
			case <-a.ctx.Done():
				return
			case <-q.wake:
			}
			for {
				c, ok := q.pop()
				if !ok {
					break
				}
				if a.ctx.Err() != nil {
					return
				}
				if a.capturePaused.Load() || c.epoch != a.captureEpoch.Load() {
					continue
				}
				item, err := normalizeCapture(c)
				if err == nil && !a.capturePaused.Load() && c.epoch == a.captureEpoch.Load() {
					err = a.db.add(item.kind, item.data)
				}
				if err != nil {
					a.clipboardError(err)
					continue
				}
				a.update(func() {
					if a.tab == "Clipboard" && a.w.IsVisible() {
						a.refreshHistory()
					}
				})
			}
		}
	})
}

func (a *app) clipboardError(err error) { a.update(func() { a.status = "Clipboard: " + err.Error() }) }

func waitJob(ctx context.Context, delay time.Duration) bool {
	t := time.NewTimer(delay)
	defer t.Stop()
	select {
	case <-ctx.Done():
		return false
	case <-t.C:
		return true
	}
}

func (a *app) startIconWorker() {
	a.iconJobs = make(chan string, 32)
	a.background(func() {
		load, closeWorker, err := nativeIconLoader()
		if err != nil {
			return
		}
		defer closeWorker()
		for {
			select {
			case <-a.ctx.Done():
				return
			case path := <-a.iconJobs:
				img, err := load(path)
				var bitmap *ui.Bitmap
				if err == nil {
					bitmap = ui.NewBitmap(img)
				}
				a.update(func() {
					if a.icons == nil {
						a.icons = make(map[string]*ui.Bitmap)
					}
					// Negative results are bounded alongside successful requests.
					if len(a.iconPending) >= 256 {
						for key := range a.icons {
							delete(a.icons, key)
							delete(a.iconPending, key)
							break
						}
					}
					a.icons[path] = bitmap
				})
			}
		}
	})
}
