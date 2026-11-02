Performance fixes, 2026-11-02

All nine findings in the baseline performance review have corresponding fixes. Production changes preserve full clipboard payloads and Unicode-aware substring search. No dependency version was upgraded.

1. Windows enumeration now reuses one process-lifetime callback. Each enumeration passes an integer identifying temporary Go-owned state; that state is removed when enumeration returns. Native procedure lookups are cached. Window enumeration also runs outside the UI thread, so opening the launcher can immediately use its previous window list while a fresh list loads.

2. Clipboard notifications signal a dedicated native snapshot worker. Snapshot reading no longer goes through MyGo's UI dispatcher; a separate worker converts DIBs/UTF-16 and saves history after the clipboard lock is released. Capture never blocks the UI on a storage-channel send. The pending queue holds at most 32 snapshots and 64 MiB, evicts the oldest pending snapshots under overload, and reports overflow in the status bar. Duplicate current-owner notifications are coalesced. Native snapshots larger than 64 MiB are rejected with a status message. Pausing invalidates queued snapshots. These limits affect capture under load, not retention of saved history.

3. Clipboard previews use one worker and a single replaceable pending request. Selection changes and hiding the panel cancel obsolete requests, and generation checks prevent stale publication. PNG decoders cannot be interrupted mid-decode, so the worker limit is necessary even with cancellation. New images get persistent 480-pixel thumbnails; old images get them lazily on first preview. Scaling uses typed pixel operations through x/image/draw. Full-image decoding has a 64 MiB pixel budget; oversized originals remain available for restoration.

4. Clipboard search now maintains persistent byte n-gram postings for Unicode-lowercased content. It selects the rarest query gram and verifies candidates exactly with a streaming case-insensitive matcher. One- and two-byte queries retain their substring semantics. There is no full-payload lowercase allocation during verification. Existing databases migrate in cancellable background batches with short read transactions and grouped writes; initial text searches wait for migration. Recent unfiltered history remains available during that migration. Per-item index growth is capped at 4,096 distinct grams; high-entropy items use an exact-search fallback. Recopying content updates its recency without rebuilding its postings. Deleting history removes associated postings and thumbnails transactionally.

5. Hashing streams the content prefix and payload. Short text previews scan only the required characters and do not materialize full strings, word lists, or rune arrays. Duplicate captures reuse the stored preview, payload, thumbnail, and index. The preview pane reads a bounded text prefix directly from a read transaction. New large payloads still incur hashing, indexing, and database write costs; bbolt's write buffers remain part of ingestion allocations.

6. Icons load on one dedicated, COM-initialized Windows thread that owns and releases its shell objects and bitmaps. There is no main-thread shell extraction or PNG round trip. Visible rows submit bounded requests, and positive and negative cache entries are bounded in memory. The cache is intentionally in memory; a persistent icon cache is unnecessary to keep cold extraction off the UI thread.

7. The 120 ms debounce now applies to typing-driven clipboard and Obsidian searches. Initial loads, explicit pagination, arithmetic, and cached conversions proceed immediately. Incoming clipboard captures trigger coalesced refreshes that retain results and selection. While browsing older pages, the app retains that page and displays a message asking the user to reopen Clipboard for new items.

8. Expired currency rates display immediately with their cached date. Refreshes start after the initial result is published, run independently of amount edits, and are deduplicated/backed off per pair for one minute. Completed refreshes only affect a currently matching pair; the current amount is recalculated from the newly saved rate. Existing fresh-cache and uncached-error behavior remains.

9. App filtering tokenizes queries once, caches normalized names, uses a pin-membership set, and partitions apps in one traversal. Result buffers are reused. Command filtering also reuses query tokens and avoids repeating title normalization for each word. Existing ordering and command ranking remain covered by UI tests.

Verification:

- `CGO_ENABLED=0 go test ./...` passed, including existing UI/navigation tests and new regressions for Unicode substring search, false positives, pagination/recency, deletion, migration/restarts, overflow fallback, complete payload restoration, stored thumbnails, bounded capture queues, preview cancellation, concurrent search/writes, and stale-rate refresh deduplication.
- Native Windows tests passed for repeated enumeration cleanup, COM icon extraction on a worker, and DIB pixel conversion/validation. Tests do not modify the system clipboard or access an Obsidian vault.
- `go tool mygo vet .` passed.
- `go test -gcflags=all=-d=checkptr=2 ./...` passed with CGO disabled.
- `go tool mygo build -skip-installer .` passed; executable is `build/windows-amd64/launchr.exe`.
- `go test -race ./...` was attempted with CGO enabled, but could not build because the environment has no `gcc`. Concurrent regression coverage is present, but this is not a substitute for a successful race-detector run.

Current synthetic benchmark allocation samples (Windows/amd64, Go 1.27.3, Intel Core i5-1145G7):

- Indexed search miss over 10,000 records: about 1.5 KB and 24 allocations per query, after migration.
- Duplicate capture of 8.4 MB text: about 13 KB and 103 allocations per operation, with the initial payload already stored.
- First-time capture of a distinct 8.4 MB text item: about 35 MB of allocations in the sampled operation, including bbolt writes. This workload is separate from the duplicate-capture baseline.
- Preview of a stored thumbnail for a 4K original: about 1.1 MB and 28 allocations per operation. This includes thumbnail decoding and bitmap creation; original decoding occurs during capture or lazy migration, not on every selection.
- Warm filtering of 1,000 apps: about 167 bytes and one allocation per query.

These are allocated bytes, not retained memory or peak working set. Timing varies with disk and host load. `performance_test.go` retains the workloads; setup, migration, and initial duplicate storage are excluded from the relevant steady-state timers. The preview benchmark was updated to call the worker's storage/decoding operation directly because previews now use a persistent worker. No live clipboard burst test, startup-to-first-paint trace, or idle CPU/GPU power measurement was performed.

Repeat benchmarks with:

```powershell
$env:CGO_ENABLED = '0'
go test -run '^$' -bench 'Benchmark(Clipboard|FilterApps)' -benchmem -benchtime=1s -count=3 .
```
