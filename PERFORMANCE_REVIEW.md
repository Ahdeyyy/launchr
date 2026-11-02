Performance review of Launchr, 2026-11-02

Implementation status: the nine findings below have now been addressed. See [PERFORMANCE_CHANGES.md](PERFORMANCE_CHANGES.md) for the changes, verification, benchmark notes, and operating limits. This document preserves the original review of the baseline.

Reviewed application sources at commit `0ffc2a0`, MyGo v0.7.4, relevant bbolt v1.5.0 internals, and Go's Windows callback implementation. Scope included launcher opening, application filtering, rendering, icons, clipboard capture/storage/search/preview, settings, calculator, currency, Obsidian, and shutdown. No production behavior was changed. Added synthetic benchmarks in `performance_test.go` so the expensive paths can be measured again.

The most urgent issues are the callback allocated on every launcher open, main-thread clipboard work, and uncancelled full-resolution previews. Existing virtualization, search cancellation, background database writes, installed-app discovery, and event-driven clipboard notifications are useful foundations and should be preserved.

1. High: every launcher open permanently retains a native callback and its captured window list.

   Location: `platform_windows.go:29`, called by `main.go:735`.

   `openWindows` creates a new closure with `syscall.NewCallback` every time. That closure captures `result`. Go documents that callback storage is never released; the pinned runtime keeps the function value in its callback table and terminates with `too many callback functions` when that table fills. Consequently each open retains a snapshot of window metadata as well as consuming another callback slot. This is a code-confirmed lifetime defect; the review did not deliberately exhaust the table in a running application.

   Use a single process-lifetime callback and pass per-enumeration state through a safe identifier in `lparam`. Clean up the associated state after enumeration. Do not pass an unmanaged Go pointer through native storage. Verify repeated enumeration does not create additional callback closures or retain old results.

2. High: clipboard capture can stall the UI, including while the launcher is hidden.

   Location: `main.go:748`, queue configured at `main.go:799`.

   The entire capture callback runs inside `Window.Update`, hence on the main thread. `ReadImage` is synchronous. In the pinned Windows backend (`internal/windows/system.go:188`) a DIB-only clipboard image is copied, converted, and PNG-encoded before returning, while the clipboard is open. This work scales with image dimensions. Moving only the database write into a worker does not remove that CPU work from the main thread. Simply wrapping `ReadImage` in another goroutine would also be insufficient because MyGo marshals clipboard calls to the main thread.

   The subsequent send to the 32-item channel can block the same thread when the database worker falls behind. Capacity is measured in items, not bytes, so large payloads can also create substantial queued memory. These are source-confirmed blocking paths; end-to-end clipboard latency was not measured against the user's clipboard.

   Keep the native clipboard snapshot short, with format conversion/PNG encoding on a worker using a suitable Windows adapter or dependency enhancement. Design explicit backpressure with a byte budget and a documented overflow policy; preserve the intended history semantics. Coalesce pending notifications using clipboard sequence identity when they refer to the same current content. Test capture bursts with a deliberately slow database writer and a responsive UI heartbeat.

3. High: rapid preview selection launches overlapping full-image decodes without cancellation or a worker limit.

   Location: `main.go:400`, invoked from the view at `main.go:678`.

   Every newly selected clipboard entry starts a goroutine that reads the complete payload and decodes the complete image before scaling it to at most 480 pixels. The preview-ID check rejects stale publication only after all that work finishes. Holding Down through image history can therefore leave several obsolete large decodes consuming CPU and memory. Hiding the window also does not cancel these jobs. A synthetic transparent 3840 x 2160 PNG preview allocated about 35.1 MB and 259,234 allocations per selection. The allocation profile identified the full-size PNG decode as the dominant byte allocation; the generic per-pixel `At`/`Set` loop also creates many small allocations.

   Use a latest-selection worker with bounded concurrency and generation checks before expensive phases. Most standard image decoders cannot be interrupted mid-decode, so limiting concurrency remains necessary even with cancellation. Generate and store thumbnails during capture, cache a bounded number by content hash, and inspect image dimensions before decoding oversized originals. Use an optimized scaler or typed pixel operations. Verify rapid selection keeps at most the intended number of jobs active and stale completions cannot replace the selected preview.

4. Medium: full-text clipboard searches scan and allocate across the entire archive.

   Location: `store.go:94`.

   A rare or absent term makes `page` deserialize each metadata record, look up every text payload, and allocate a lowercased copy of its full contents. The 100-result limit caps matches, not records scanned. A synthetic search miss over 10,000 records of 4 KiB each allocated about 44.9 MB and roughly 90,000 objects per search. Cancellation is checked between records, so a single very large payload remains an expensive unit of work.

   The complete scan holds one bbolt read transaction. It does not block every concurrent write, but can delay a writer that needs to grow/remap the database and prevents reuse of pages needed by that reader. This can compound the clipboard queue problem.

   For large archives, maintain a search index with the existing Unicode/case-insensitive substring semantics, for example n-gram candidates followed by exact verification. An ordinary word index would change behavior. Use bounded read transactions and generation-aware cancellation; caching lowercase contents alone trades CPU for disk space without fixing the linear scan. Verify common, absent, Unicode, and beyond-preview matches, including deletion and deduplication updates.

5. Medium: storing large text creates multiple full-size temporary representations and rewrites duplicate payloads.

   Location: `store.go:60` and `store.go:74`; related preview conversion at `main.go:430`.

   Hashing first builds a new `kind + ':' + payload` byte slice. Building a 160-character preview splits and joins the entire text and converts the entire result to runes. This work occurs inside the write transaction. Recopying identical content still rebuilds the preview and calls `Put` on the unchanged payload. A repeated 8,400,000-byte text capture allocated approximately 98-100 MB per operation in this workload. Text preview separately clones the complete payload and converts it to runes before truncating to 4,000 characters.

   Stream the hash prefix and payload into the hasher. Generate bounded previews with an incremental UTF-8/whitespace scan outside the write transaction. Reuse immutable payload and preview data on duplicate captures while updating recency metadata. Expose a bounded text-preview read so displaying a short preview does not require a complete payload clone. Verify exact full-text restoration and whitespace/Unicode preview behavior.

6. Medium: cold icon extraction still executes on the main thread.

   Location: `appearance.go:40`.

   Although `FileIcon` is called from a goroutine, MyGo `modules.go:72` uses `onMainValue`. The Windows backend performs shell COM lookup, image extraction, pixel conversion, and PNG encoding synchronously there. Only the subsequent PNG decoding in this app is off-thread. Scrolling through uncached apps can queue several of these UI-thread calls. The visible-row limit and current in-memory cache help, but do not remove cold extraction stalls. No end-to-end shell extraction timings were captured.

   A dedicated COM-initialized worker that owns its shell objects, or a corresponding dependency capability, is the robust fix. A persistent cache can reduce cold work on later runs; bound outstanding requests and cache size. Correctly initialize COM and preserve thread ownership rather than directly calling the existing backend on an arbitrary goroutine.

7. Medium: the shared debounce adds avoidable latency to local actions.

   Location: `main.go:137`.

   The 120 ms timer runs for every asynchronous search, including initial empty clipboard loads, explicit pagination, simple arithmetic, and fresh cached currency conversions. Those actions wait even when there is no remote work to debounce. `search(false)` clears results first, making the wait visible. Each clipboard capture while that view is visible also restarts its search and clears selection, potentially postponing results during sustained copy activity.

   Restrict debounce to typing-driven expensive searches. Execute bounded arithmetic, initial page loads, and explicit pagination promptly. Refresh visible history in a coalesced way that preserves selection and retains useful existing results while replacement results load. Verify timing and cancellation with a controlled scheduler rather than fragile elapsed-time assertions.

8. Medium: expired currency cache entries are withheld until the network attempt finishes.

   Location: `integrations.go:32`; request timeout configured at `main.go:170`.

   After six hours, a previously usable rate waits for `client.Do` before it is shown. On a slow or unreachable network this can consume the eight-second request timeout before falling back to the cached result. Each amount edit can restart that path. This is a code-confirmed latency bound; the review did not issue live currency requests.

   Return an explicitly dated cached estimate immediately and refresh independently. Deduplicate refreshes by currency pair, add retry backoff, and publish the new rate only to the appropriate active query. Preserve the displayed stale/offline status and prevent obsolete refreshes from replacing current results.

9. Low: synchronous app filtering repeatedly normalizes identical strings and allocates a new result buffer.

   Location: `main.go:218`, and analogous small repeated normalization in `navigation.go:29`.

   Every candidate recomputes `strings.Fields(strings.ToLower(q))`; names are lowercased again for each search word. Apps are traversed twice to separate pins, and pin membership is a linear slice lookup. The synthetic 1,000-app, two-word query allocated approximately 1.00 MB and 3,012 objects per filter. This is lower priority for typical small app catalogues, but runs synchronously while typing.

   Tokenize the query once, cache normalized names when the catalogue changes, build a pin-membership set, and partition results in one pass with a reused buffer. Preserve current ordering and multiword matching. Avoid introducing a complex search service unless measurements on real catalogues justify one.

Verification and measurement limits:

- `CGO_ENABLED=0 go test ./...` passed.
- `go tool mygo vet .` passed with no diagnostics.
- Synthetic benchmarks ran on Windows/amd64, Go 1.27.3, Intel Core i5-1145G7, with temporary databases and no user clipboard or vault access. Fixture setup was excluded from benchmark timing. The search fixture used uppercase text so case normalization requires real allocation. The text-add workload repeats identical content and includes transaction commits. The preview benchmark exercises background read/decode/resize/bitmap creation, but has no native window, so it excludes GPU upload, UI publication, and presentation.
- Timing varied materially across local runs; allocation totals are the more reliable baseline here. The figures above are allocated bytes per operation, not retained memory, peak working set, or a claim about observed UI freezes.
- Allocation profiling of the 4K preview confirmed full-resolution PNG decoding dominates allocated bytes. This supports thumbnails and bounded jobs as the first fixes before optimizing the small output image's scaling loop.
- No live native-app interaction, startup-to-first-paint measurement, idle CPU/power trace, GPU/Acrylic comparison, network service test, or large real-archive trial was performed. Those remain verification tasks, not confirmed performance defects. Font initialization and visual styling should be profiled before being changed.
- Suggested implementation order: fix the callback lifetime; remove capture/UI backpressure; bound previews and add thumbnails; reduce archive search/write allocation; then address cold icon extraction, debounce/cached-rate latency, and app-filter micro-optimizations.

To repeat the retained benchmarks in PowerShell:

```powershell
$env:CGO_ENABLED = '0'
go test -run '^$' -bench 'Benchmark(Clipboard|FilterApps)' -benchmem -benchtime=1s -count=3 .
```

The benchmarks are baseline workloads, not pass/fail latency gates. Future verification should add integration coverage for callback lifetime, capture backpressure, preview concurrency, archive growth during search, and stale-rate refresh behavior as the corresponding implementations are changed.
