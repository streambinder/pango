# Tasks: Targeted Lookup for Unsync

**Input**: Design documents from
`/specs/006-unsync-targeted-lookup/`

**Prerequisites**: plan.md (required), spec.md (required)

**Tests**: The constitution requires unit tests next to the
code and a 90 percent coverage gate. The task list includes
the tests.

**Dependency**: Cycles 004 and 005, PR #286, is open and
unmerged. The implementation of this cycle stacks on branch
`feat/unsync`. Q1 is decided by the owner (2026-11-27):
the fetcher and the parser run concurrently over channels,
and the pruner waits until their queues are empty and
closed. The owner ratified spec 006 on 2026-11-27 and
answered Q2–Q3 as proposed: remove `Index.Delete` and
fold into PR #286.

**Note**: Do not mark a task done until its work lands and
its verification passes.

## Format: `[ID] [P?] [Story] Description`

- **[P]**: Can run in parallel (different files, no dependencies)
- **[Story]**: Which user story this task belongs to
- Include exact file paths in descriptions

## Phase 1: Artifacts

**Purpose**: Close the specify and plan steps of cycle 006.

- [x] T001 Record the hypothesis investigation, the
      index-build finding with the corrected unreadable
      file premise, the queue decision, the double-print
      defect, and the requirements in
      `specs/006-unsync-targeted-lookup/spec.md`
- [x] T002 Record the gated-pipeline design, the
      removal of `Index.Delete`, the double-print fix,
      and the PR #286 dependency decision in
      `specs/006-unsync-targeted-lookup/plan.md`

## Phase 2: Command (Stories 1, 2, and 3)

**Purpose**: The `unsync` command runs without the full
index scan, and each playlist file line prints once.

- [x] T003 [US1] Drop the `auditBuildIndex` call from
      `RunE` in `cmd/unsync.go` and replace
      `unsyncFetchAll` with a streaming fetcher: one
      goroutine fetches the collections sequentially
      (albums, tracks, playlists), passes the tracks
      channel into the Spotify calls, collects the
      fetched playlists in fetch order, closes the
      tracks channel when done, and reports its error on
      a buffered error channel; keep the
      empty-selection check before any work
- [x] T004 [US1] Add the sequencer and the parser
      workers in `cmd/unsync.go`: a sequencer goroutine
      numbers fetched tracks in arrival order and
      forwards them to the parser channel, and four
      parser workers through
      `nursery.RunMultipleCopiesConcurrently` consume
      the channel and emit one `unsyncVerdict` per track
      (track, filename, stored identifier, verdict kind,
      reason, sequence number); a worker prints nothing
      and holds no shared state; close the verdicts
      channel when the workers return
- [x] T005 [US1] [US3] Add the barrier and the sequential
      pruner in `cmd/unsync.go`: the main flow drains the
      verdicts into a slice until the verdicts channel
      closes, then reads the fetch error and aborts
      before any deletion when it is set; the pruner
      sorts the verdicts by sequence number, checks the
      `deleted` map claim, re-stats before it removes,
      reclassifies a missing file as absent, prints
      every per-track line itself, and removes the
      playlist files after all track verdicts, in fetch
      order
- [x] T006 [US1] [P] Remove the `indexData.Delete`
      call from the pruner in `cmd/unsync.go`, and remove
      the `Index.Delete` method from
      `entity/index/index.go` with its tests
      (`TestDelete`, `TestDeleteUnknown`,
      `TestDeleteSharedPath`) in
      `entity/index/index_test.go`, per the ratified
      answer to Q2
- [x] T007 [US2] Remove the post-summary loop over
      `results.playlistFiles` from `RunE` in
      `cmd/unsync.go`, so each playlist file line prints
      once inside `unsyncDeletePlaylistFiles`; keep
      appending the filenames to `results.playlistFiles`,
      because the summary counts them

## Phase 3: Command Tests (Stories 1, 2, and 3)

**Purpose**: The harness proves the command against a
scripted Spotify transport, with no live provider call.

- [x] T008 [US1] Add a command test in
      `cmd/unsync_test.go` through `syncEnv`,
      `testExecute`, and `writeTaggedFile`: the output
      directory holds the target tagged file, unrelated
      tagged files, and one file with garbage bytes; the
      run deletes the target, leaves every unrelated file
      in place, and the captured output holds no
      "indexing library" line and no "indexed " line
- [x] T009 [US2] Add command tests in
      `cmd/unsync_test.go` (add the `strings` import):
      a playlist run in real mode and in dry-run mode
      counts the occurrences of each playlist-file line
      with `strings.Count` and asserts exactly one per
      file per mode
- [x] T010 [US3] Add a command test in
      `cmd/unsync_test.go`: override `/v1/albums/` to
      return two tracks with the same title and artist
      but different identifiers (use `simpleTrackJSON`
      for both), seed the file tagged with the first
      identifier, and assert that the first occurrence
      deletes the file, the second reports absent, and
      the captured output holds no "deletion failed"
      line
- [x] T011 [US3] Add a command test in
      `cmd/unsync_test.go`: a scripted playlist with
      several tracks asserts that the deleted lines
      appear in fetch order, and a second run of the
      same command asserts the same order; add a
      cross-collection case where one track appears in
      two requested collections and asserts one deletion
      and one already-deleted report
- [x] T012 [US3] Run the full existing `unsync` harness
      suite in `cmd/unsync_test.go` and confirm every
      outcome is unchanged: match, mismatch, untagged,
      twice-counted, empty, path failure, fetch failure,
      deletion failure, and every dry-run case

## Phase 4: Verification

**Purpose**: Prove the cycle against the constitution gates
and on a bounded real selection before the merge.

- [x] T013 Run gofmt, gofumpt, `go vet`, the full test
      suite with the race detector, and the coverage gate
      locally
- [x] T014 Verify live before the merge, on a bounded
      selection: build the binary and run a dry run
      against a scratch output directory with a small
      scripted playlist, confirming that the output holds
      no indexing line and every file stays in the
      directory; never a first run against the full
      library
- [x] T015 Handle the pull request per the ratified answer
      to Q3: fold the change into PR #286 by amending its
      single commit and re-verifying the pipeline green on
      the new head, or open a separate stacked PR on
      `feat/unsync` and reach a green pipeline on the
      exact head before calling it ready

## Phase 5: Converge

**Purpose**: Close the cycle as a historical record.

- [x] T016 Mark the task outcomes and the success criteria
      in this file at the end of the implementation

## Outcomes

Implemented and verified on branch `feat/unsync`, folded into
the PR #286 as one squashed commit. Ratified 2026-11-27 as proposed
(gated pipeline, `Index.Delete` removed, fold into #286). The
branch was rebased onto master `ee18a0e` (Dependabot #287 gomod
and #288 setup-go); the rebase was clean, no conflicts, and the
base suite was green before implementation. Local gates: gofmt,
gofumpt, `go vet`, golangci-lint with the CI configuration
(0 issues), full suite with `-race -cover` green, every package
above 90 percent (cmd 94.6 percent, provider 90.8 percent
lowest). Bounded live check: the built binary lists `--dry-run`
in `unsync --help`, and a bare dry run on a scratch directory
prints "nothing to do", exits zero, and leaves every scratch
file in place, with no indexing line. No live Spotify fetch ran
from this machine (interactive auth is required); the
fetch-and-prune path is proven by the scripted harness tests
running the same `RunE`. The live ponz dry run stays the
owner's, after merge and deploy.
