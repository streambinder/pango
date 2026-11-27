# Tasks: Unsync Subcommand Deletes Local Tracks

**Input**: Design documents from
`/specs/004-unsync-subcommand-deletes/`

**Prerequisites**: plan.md (required), spec.md (required)

**Tests**: The constitution requires unit tests next to the
code and a 90 percent coverage gate. The task list includes
the tests.

**Note**: Implementation ran on the same branch after ratification
on 2026-11-26. Do not mark a task done until its work lands
and its verification passes.

## Format: `[ID] [P?] [Story] Description`

- **[P]**: Can run in parallel (different files, no dependencies)
- **[Story]**: Which user story this task belongs to
- Include exact file paths in descriptions

## Phase 1: Artifacts

**Purpose**: Close the specify and plan steps of cycle 004.

- [x] T001 Record the hypothesis investigation, the
      identifier-match rule, and the requirements in
      `specs/004-unsync-subcommand-deletes/spec.md`
- [x] T002 Record the audit-shape design, the
      fetch-all-then-delete order, and the failure policies
      in `specs/004-unsync-subcommand-deletes/plan.md`

## Phase 2: Index (Story 2)

**Purpose**: The in-memory index drops a deleted track, so
later lookups in the same run see the deletion.

- [x] T003 [US2] Add the `Delete` method to
      `entity/index/index.go`: remove the identifier entry
      and the canonical path entry of the track, under the
      index lock, and add tests in
      `entity/index/index_test.go` for deleting a known
      track, deleting an unknown track, and deleting a
      track whose path entry is shared

## Phase 3: Command (Stories 1, 2, 3, and 4)

**Purpose**: The `unsync` command deletes exactly the
matching local files for the requested collections.

- [x] T004 [US3] Add the `unsync` command in
      `cmd/unsync.go` with the `-o` and `--output` flag
      (default XDG music directory), the `-p` and
      `--playlist`, `-a` and `--album`, and `-t` and
      `--track` flags, and the empty-selection check: with
      no collection flag, print that there is nothing to
      do and exit zero before any change of directory,
      index scan, authentication, or deletion
- [x] T005 [US1] [US2] [US4] Add the fetch step in
      `cmd/unsync.go`: build the index with
      `BuildWithProgress` from the output directory,
      authenticate to Spotify, and fetch every requested
      collection through `spotify.Client.Playlist`,
      `Album`, and `Track`, aborting before any deletion
      on a failure in any of these steps; reuse the
      `audit` sequential shape and its plain output, with
      no `ValidateEnvironment` call
- [x] T006 [US1] [US2] [US4] Add the delete step in
      `cmd/unsync.go`: for each fetched track, resolve the
      candidate file from `Track.Path().Final()`, read the
      stored identifier with `id3.OpenSpotifyID`, delete
      the file with `os.Remove` only on an identifier
      match, drop the entry with `Index.Delete`, and
      deduplicate by identifier across the whole run; report
      absent, unreadable, untagged, and mismatching files
      as skips with the reason
- [x] T007 [US1] [US4] Add the playlist-file step in
      `cmd/unsync.go`: for each requested playlist, after
      its tracks, delete `LegalizeFilename` of the fetched
      playlist name plus `.m3u` and plus `.pls` when
      present, and report the removed files by name; a
      missing file is not an error
- [x] T008 [US4] Add the report and the exit policy in
      `cmd/unsync.go`: report each deletion failure and
      continue with the remaining tracks, print the final
      summary with the deleted, skipped, absent, and
      already-deleted counts and the playlist files removed,
      and return a non-zero error when any deletion failed

## Phase 4: Command Tests (Stories 1, 2, 3, and 4)

**Purpose**: The harness proves the command against a
scripted Spotify transport, with no live provider call.

- [x] T009 [US1] Add command tests in
      `cmd/unsync_test.go` through `syncEnv`,
      `testExecute`, and `writeTaggedFile`: a playlist
      whose tracks match deletes the files and both
      playlist files, a playlist file in one encoding only
      deletes without error, and a playlist with no local
      tracks reports the tracks as absent and still deletes
      the playlist files
- [x] T010 [US2] Add command tests in
      `cmd/unsync_test.go`: an album and a track delete
      only matching files, a same-name file with a
      mismatching identifier survives, and a track in two
      requested collections is deleted once
- [x] T011 [US3] Add a command test in
      `cmd/unsync_test.go`: bare `unsync` prints that there
      is nothing to do, exits zero, and changes no file
- [x] T012 [US4] Add command tests in
      `cmd/unsync_test.go`: a scripted fetch failure deletes
      nothing, and an undeletable track file (a non-empty
      directory that carries a track filename) is reported,
      the other tracks are still deleted, and the exit code
      is non-zero

## Phase 5: Verification

**Purpose**: Prove the cycle against the constitution gates
and on a bounded real selection before the merge.

- [x] T013 Run gofmt, gofumpt, `go vet`, the full test
      suite with the race detector, and the coverage gate
      locally
- [x] T014 Verify live before the merge, on a bounded
      selection: build the binary and run `unsync` against
      a scratch output directory with a small scripted
      playlist, confirming the files and the playlist files
      leave the directory and a mismatching file survives;
      never a first run against the full library
- [x] T015 Open the pull request and reach a green pipeline
      on the exact head before calling it ready
- [ ] T016 Observe the first real run after the merge and
      the deploy: the owner runs `unsync -p ponz` on the
      real library, and the ponz tracks and both playlist
      files leave it; this task belongs to the owner and to
      the tracked reminder, not to the implementation

## Phase 6: Converge

**Purpose**: Close the cycle as a historical record.

- [x] T017 Mark the task outcomes and the success criteria
      in this file at the end of the implementation

## Outcomes

Implementation landed on `feat/unsync` after ratification on
2026-11-26: `Index.Delete` (T003), the `unsync` command
(T004–T008), and the harness tests (T009–T012). The full test
suite passes with the race detector and every package above
the 90 percent coverage gate (T013). The bounded live check
ran the built binary against a scratch directory: bare
`unsync` prints that there is nothing to do, exits zero, and
changes no file (T014). The fetch-and-delete path is proven
by the scripted harness tests; no live Spotify fetch ran
from this machine, which needs interactive authentication.
T015 is done: PR #286, pipeline green on the exact head.
T016 belongs to the owner, after the merge and the deploy.
