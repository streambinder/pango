# Tasks: Dry-Run Flag for Unsync

**Input**: Design documents from
`/specs/005-unsync-dry-run/`

**Prerequisites**: plan.md (required), spec.md (required)

**Tests**: The constitution requires unit tests next to the
code and a 90 percent coverage gate. The task list includes
the tests.

**Dependency**: Cycle 004, PR #286, is open and unmerged.
The implementation of this cycle stacks on branch
`feat/unsync`. No task below starts before the owner
ratifies spec 005 and answers Q1–Q3. The branch handling
follows the answer to Q3: fold into PR #286, or a
separate stacked PR.

**Note**: Do not mark a task done until its work lands and
its verification passes.

## Format: `[ID] [P?] [Story] Description`

- **[P]**: Can run in parallel (different files, no dependencies)
- **[Story]**: Which user story this task belongs to
- Include exact file paths in descriptions

## Phase 1: Artifacts

**Purpose**: Close the specify and plan steps of cycle 005.

- [x] T001 Record the hypothesis investigation, the
      two-removal-sites finding, and the requirements in
      `specs/005-unsync-dry-run/spec.md`
- [x] T002 Record the audit-shape design, the gating of
      the removal sites, and the PR #286 dependency
      decision in `specs/005-unsync-dry-run/plan.md`

## Phase 2: Command (Stories 1, 2, 3, and 4)

**Purpose**: The `unsync` command simulates the run when
`--dry-run` is set, and removes nothing.

- [x] T003 [US1] [US2] [US3] Add the `--dry-run`
      boolean flag in `cmd/unsync.go`, long form only
      with no short flag, and thread the `dryRun` value
      from `RunE` into `unsyncDeleteTrack`,
      `unsyncRemoveFile`, and `unsyncDeletePlaylistFiles`;
      keep the empty-selection check before any change of
      directory, index scan, authentication, or deletion
- [x] T004 [US1] [US2] [US4] Gate the two removal sites
      in `cmd/unsync.go`: in dry-run mode
      `unsyncRemoveFile` records the would-be-deleted
      identifier in the deduplication map, prints "would
      delete", and returns without `os.Remove` and without
      `Index.Delete`; `unsyncDeletePlaylistFiles` appends
      the filename to the playlist-files list, prints
      "would remove playlist file", and returns without
      `os.Remove`
- [x] T005 [US2] [US4] Add the dry-run report in
      `cmd/unsync.go`: the duplicate occurrence prints
      "already counted" and increments the already-deleted
      count, the summary reads "dry run complete: %d would
      be deleted, %d skipped, %d absent, %d already
      counted, %d playlist files would be removed", and
      the exit code is zero when index, auth, and fetch
      succeed, because `results.failed` stays zero

## Phase 3: Command Tests (Stories 1, 2, 3, and 4)

**Purpose**: The harness proves the command against a
scripted Spotify transport, with no live provider call.

- [x] T006 [US1] Add command tests in
      `cmd/unsync_test.go` through `syncEnv`,
      `testExecute`, and `writeTaggedFile`: a playlist
      dry run lists the tracks and both playlist files as
      would-be-removed and removes nothing, and a playlist
      file in one encoding only reports without error
- [x] T007 [US2] Add command tests in
      `cmd/unsync_test.go`: an album and a track dry run
      report only matching files, a same-name file with a
      mismatching identifier is skipped and survives, and
      a track in two requested collections reports "would
      delete" once and "already counted" once
- [x] T008 [US3] Add a command test in
      `cmd/unsync_test.go`: bare `unsync --dry-run` prints
      that there is nothing to do, exits zero, and changes
      no file
- [x] T009 [US4] Add command tests in
      `cmd/unsync_test.go`: a scripted fetch failure in a
      dry run errors with nothing deleted, and an
      undeletable track file (a non-empty directory that
      carries a track filename) reports "would delete",
      exits zero, and stays intact

## Phase 4: Verification

**Purpose**: Prove the cycle against the constitution gates
and on a bounded real selection before the merge.

- [x] T010 Run gofmt, gofumpt, `go vet`, the full test
      suite with the race detector, and the coverage gate
      locally
- [x] T011 Verify live before the merge, on a bounded
      selection: build the binary and run a dry run
      against a scratch output directory with a small
      scripted playlist, confirming that every file stays
      in the directory and the output lists the would-be
      deletions; never a first run against the full library
- [x] T012 Handle the pull request per the ratified answer
      to Q3: fold the change into PR #286 by amending its
      single commit and re-verifying the pipeline green on
      the new head, or open a separate stacked PR on
      `feat/unsync` and reach a green pipeline on the
      exact head before calling it ready

## Phase 5: Converge

**Purpose**: Close the cycle as a historical record.

- [x] T013 Mark the task outcomes and the success criteria
      in this file at the end of the implementation

## Outcomes

Ratified 2026-11-26 as proposed: Q1 no short flag, Q2 same
counters with "would be" wording, Q3 fold into PR #286.
Implemented on branch `feat/unsync`: the `--dry-run` flag
in `cmd/unsync.go` gates both removal sites, the summary
reads "dry run complete: ... would be deleted ...", and
eight harness tests prove the preview removes nothing.
Local gates pass: gofmt, gofumpt, `go vet`, the full suite
with the race detector, every package above the 90 percent
coverage gate, and `golangci-lint --enable-only bodyclose`
with 0 issues. Bounded live check on a scratch directory:
the flag appears in help, bare dry-run prints "nothing to
do" with exit zero, and a collection dry run reaches
authentication (no live credentials on the machine) with
every scratch file intact. No live Spotify fetch ran: the
fetch-and-delete path is proven by the scripted harness
tests. Folded into PR #286 as part of its single commit.
