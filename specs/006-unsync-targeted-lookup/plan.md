# Implementation Plan: Targeted Lookup for Unsync

**Branch**: `feat/unsync` | **Date**: 2026-11-27 |
**Spec**: `specs/006-unsync-targeted-lookup/spec.md`

**Input**: Feature specification from
`/specs/006-unsync-targeted-lookup/spec.md`

**Depends on**: Cycles 004 and 005, specs
`specs/004-unsync-subcommand-deletes/spec.md` and
`specs/005-unsync-dry-run/spec.md`, implemented on branch
`feat/unsync` in PR #286, open and unmerged. This cycle
removes a stage from the command those cycles introduce
and fixes its playlist-file report.

## Summary

`unsync` drops the full index build. The run order becomes
change of directory, serial authentication, fetch and
parse running concurrently, then the pruner after the
barrier. The fetcher streams each track into the tracks
channel as it arrives; a sequencer numbers the tracks in
arrival order; four parser workers resolve each expected
local file from `Track.Path().Final()`, stat it, and read
that one file's Spotify identifier from the "Spotify ID"
frame. No other file is opened. The main flow drains the
verdicts into a slice, and the sequential pruner consumes
the slice in fetch order: it claims each identifier at
first occurrence, so a track in two collections is
deleted once, and it re-stats before it removes, so a
second same-name occurrence reports absent, never a
deletion failure. `Index.Delete` and its tests go,
because the method has no caller left. Each playlist file
line prints once, at the removal site, and the
post-summary loop that printed it a second time goes. The
summary, the exit policy, and the dry-run semantics stay
as in cycles 004 and 005.

## Technical Context

**Language/Version**: Go, version from `go.mod`

**Primary Dependencies**: none new; the flag, the ID3
layer, and the Spotify client already exist, and the
change removes code instead of adding a dependency

**Storage**: no new storage; the run writes nothing but
the deletions it already performed

**Testing**: command tests through the existing harness in
`cmd/harness_test.go` (`syncEnv`, `testExecute`,
`writeTaggedFile`, scripted Spotify transport); index
tests in `entity/index/index_test.go`; no test calls a
live provider

**Concurrency**: the fetcher and the parser run on
channels with the `pipelineBuffer` depth from sync; the
parser runs four workers through
`nursery.RunMultipleCopiesConcurrently`, the same runner
sync uses; the pruner stays sequential

**Constraints**: per-package coverage at 90 percent; unit
tests stub every network dependency; the exported surface
of the `entity`, `spotify`, and `sys` packages is a
compatibility contract, so the removal of `Index.Delete`
needs the recorded decision this cycle carries; no live
verification of the full library before the merge

## Constitution Check

- Metadata Is the Index: the match rule is unchanged. The
  run reads the same ID3 frames that the processor
  writes and that `show` and `audit` read, but only for
  the target files. The index of a later run is built
  from the files, so deleting a file still deletes it
  from the index.
- Pipeline Shape: `unsync` gains a real pipeline of
  routines, in the shape of sync: the fetcher feeds the
  tracks channel, the parser consumes it through a
  sequencer, and the pruner runs after the barrier. Each
  stage keeps its failure policy local: an authentication
  or fetch failure aborts before any deletion, a parser
  failure is a skip with a reason, and a pruner failure
  is reported while the run continues with a non-zero
  exit at the end. The fetch-all-then-delete order stays,
  enforced by the barrier instead of by the call order.
- Honest Reporting: the report gains one honest line
  less, not more. The playlist-file line prints once,
  at the removal that produced it, and the summary still
  counts the playlist files. The removal of the indexing
  lines is honest too: no index is built, so no indexing
  is reported.
- Graceful Enrichment: not applicable. `unsync` collects
  no enrichment.
- Library Surface: `Index.Delete` is removed with its
  tests. The removal is a breaking change to the exported
  surface of `entity/index`, and the ratified spec 006 is
  the recorded decision the constitution requires. No
  other exported signature changes. The `indexData`
  variable stays, in use by `sync` and `audit`.
- Minimal Owned Surface: no new dependency. The change
  deletes code.
- Production Care: the change is verified on a bounded
  selection before it is declared ready, never on a first
  run against the full library. The live ponz dry run is
  the owner's, after the merge and the deploy.
- Specification Cycle: this document and the task list
  complete the plan and tasks steps of cycle 006. The
  specification is a draft until the owner ratifies it.
  Implementation follows in a later cycle.

No amendment to the constitution is needed for this cycle.
Verified on 2026-11-27 against branch `feat/unsync`,
commit `9b2a037`: the constitution names no index build
for unsync, the provider section is untouched by this
cycle, and the architecture baseline names no component
this cycle changes. Cycles 001, 002, and 003 stay Draft
and unratified even though their changes were implemented
and merged; that anomaly is recorded in the cycle 004
report, not repaired here, because a completed feature
directory is an immutable historical record. Specs 004
and 005 stay ratified and untouched: this cycle records
the supersession of their FR-004 and FR-012 in its own
files instead of editing them.

## Decisions

- **Drop the index build call**: `RunE` in
  `cmd/unsync.go` no longer calls `auditBuildIndex`.
  The run order becomes change of directory,
  `auditAuthenticate`, `unsyncFetchAll`, then the delete
  loop. Rejected: keeping a partial scan (no partial
  scan exists: the build walks the whole directory, and
  the delete step reads nothing from it).
- **Run the fetcher and the parser concurrently, and gate
  the pruner on the drained queues**: the fetcher streams
  tracks into the tracks channel, a sequencer numbers
  them in arrival order, four parser workers emit
  verdicts into the verdicts channel, the main flow
  drains the verdicts into a slice, and the sequential
  pruner consumes the slice in fetch order. The decision
  is the owner-decided answer to Q1 (2026-11-27).
- **Fetch the collections sequentially from one
  goroutine**: the fetcher keeps the current order
  (albums, tracks, playlists) and passes the tracks
  channel into the Spotify calls, which already stream
  each track as it arrives. Rejected: fetching the
  collections in parallel (the client is never called
  concurrently anywhere in the codebase, and a fetch
  failure must abort the run cleanly, which sequential
  fetch gives for free).
- **Keep the parser stateless and move the claim to the
  pruner**: parser workers print nothing and hold no
  shared state, so they need no mutex. The pruner checks
  the `deleted` map, re-stats before it removes, and
  prints every per-track line itself. Rejected: claiming
  in the parser under a mutex, as sync's `decideClassify`
  does (the claim would then race the file state the
  pruner acts on, and the output order would depend on
  worker scheduling).
- **Re-stat before removal, and reclassify a missing file
  as absent**: if the file is gone at prune time, the
  verdict reports absent, and an `os.Remove` failure
  with "not exist" reports absent too, never a deletion
  failure. This closes the same-filename race: both
  parsers may read the file before either deletes it,
  and the second track still reports absent.
- **Order the output by sequence number**: the pruner
  sorts the buffered verdicts by sequence number with a
  stable sort, so the per-track lines follow the fetch
  order on every run. Rejected: printing verdicts as
  they arrive (the order would depend on worker
  scheduling, and the report of a destructive command
  must read the same for the same library state).
- **Close the verdicts channel from the worker pool**:
  the parser pool runs under `nursery` in its own
  goroutine, which closes the verdicts channel when the
  workers return. The main flow drains the channel until
  it closes, so it can never block forever: the fetcher
  always closes the tracks channel, even on error.
- **Remove `Index.Delete`**: the method and its three
  tests (`TestDelete`, `TestDeleteUnknown`,
  `TestDeleteSharedPath`) go, and `unsyncRemoveFile`
  drops the call. The method was introduced by cycle 004
  solely for unsync, and with no index built it has no
  caller. Rejected: keeping the method as unused public
  API (it would leave dead exported surface that the
  constitution asks to keep minimal). The decision stays
  open for ratification as Q2, and the ratified spec 006
  is the recorded decision for the exported-surface
  change.
- **Fix the double print at the removal site**: the
  post-summary loop over `results.playlistFiles` is
  removed, and the line prints once inside
  `unsyncDeletePlaylistFiles`, where the removal (or the
  would-be removal) happens. The `playlistFiles` list
  stays, because the summary counts it. Rejected:
  printing only in the post-summary loop (the line would
  move away from the deletion that produced it, and the
  per-file failure line would still print at the site).
- **No abort mode is lost**: `BuildWithProgress` already
  skips unreadable and corrupt files silently, so no
  test can prove that an unrelated file used to abort a
  run. The proof of the removal is the absence of the
  indexing lines in the output, plus the unrelated
  files staying in place as a no-regression guard.
- **Imports in `cmd/unsync.go`**: the file gains
  `github.com/arunsworld/nursery`, already a dependency
  used by `cmd/sync.go`. It imports no index package;
  `indexData` is a package-level variable declared in
  `cmd/sync.go`.
- **Fold into PR #286**: the implementation amends the
  single squashed commit of `feat/unsync` and re-verifies
  the pipeline green on the new head, instead of opening
  a separate stacked PR. The change touches the same
  lines PR #286 introduces (`RunE`, `unsyncRemoveFile`,
  `unsyncDeletePlaylistFiles`), #286 is unmerged, and
  one feature keeps one commit and one review. Rejected:
  a stacked PR on `feat/unsync` (it would add a second
  review round for a change of a few dozen lines on an
  unmerged branch, and the stacked diff would churn the
  same lines the reviewer is already reading). The
  decision stays open for ratification as Q3.

## Project Structure

### Documentation (this feature)

```text
specs/006-unsync-targeted-lookup/
├── spec.md
├── plan.md
└── tasks.md
```

### Source Code (repository root)

```text
cmd/unsync.go              # drop index build, gated pipeline, fix double print
cmd/unsync_test.go         # harness tests: no index lines, single print, order
entity/index/index.go      # remove Index.Delete (per Q2)
entity/index/index_test.go # remove Delete tests (per Q2)
```
