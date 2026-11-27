# Feature Specification: Targeted Lookup for Unsync

**Feature Branch**: `feat/unsync`

**Created**: 2026-11-27

**Status**: Ratified 2026-11-27. The owner ordered this change on
2026-11-27, after the live dry run below, decided the Q1 answer on
2026-11-27, and ratified this text as proposed on 2026-11-27: the
fetcher and the parser run concurrently over channels, and the
pruner waits until their queues are empty and closed; `Index.Delete`
and its tests are removed; the implementation folds into PR #286.

**Input**: Order by the owner, 2026-11-27: the dry run works,
but the full index scan is wasteful when the target set is
small. Fetch the input collections first, then parse the
ID3 tags (with Spotify ID) only for the files that match
each fetched track's target file, using the same queue
approach as the sync command, and the same approach to
dedupe duplicates in the queue. Live evidence: `go run
main.go unsync -p ponz --dry-run` on the machine of the
owner indexed 4824 tracks, authenticated, fetched ponz,
and reported 198 would-be deleted, 0 skipped, 2 absent,
0 already counted, 1 playlist file. On 2026-11-27 the owner
decided the Q1 answer: the fetcher and the parser run
concurrently over channels, and the pruner waits until
their queues are empty and closed before it starts.

## Hypothesis Investigation

**Hypothesis**: Unsync can delete exactly the local files
that belong to given Spotify collections without building
the full in-memory index first, because the delete step
already resolves each file from the fetched track alone.

**Evidence** (verified on 2026-11-27, against real code:
pango at branch `feat/unsync`, commit `9b2a037`):

1. The delete step never reads the index.
   `unsyncDeleteTrack` (`cmd/unsync.go`) resolves the file
   from `track.Path().Final()`, stats it, and reads that
   one file's Spotify identifier with `id3.OpenSpotifyID`.
   A grep over `cmd/` shows the only index use in the
   command is `indexData.Delete(track)` inside
   `unsyncRemoveFile` (`cmd/unsync.go:221`), in the
   real-run path only, and nothing reads those maps
   afterward.
2. The index build is the expensive step.
   `auditBuildIndex` (`cmd/audit.go`) calls
   `indexData.BuildWithProgress(".", indexed)`, which
   walks the whole output directory and opens the ID3 tag
   of every `.mp3` file (`entity/index/index.go`). The
   live run of the owner indexed 4824 tracks before
   fetching a 200-track playlist.
3. The premise that an unreadable file aborts the index
   build is false. `BuildWithProgress` returns nil (skip)
   for any non-root walk error and for any ID3 open
   failure; only a failure on the walk root itself is
   fatal. The run already changed directory into the
   output path, so the root failure is near-unreachable.
   Removing the build removes no abort mode of
   consequence.
4. The queue becomes real channels. The Spotify client
   methods already stream fetched tracks into channels:
   `Playlist`, `Album`, `Track`, and `Library` take
   variadic `channels ...chan interface{}` and send each
   `*entity.Track` into every channel as they page
   (`spotify/playlist.go`). Sync's fetcher uses this: it
   passes `routineQueues[routineTypeDecide]` into the
   same calls (`cmd/sync.go`). The fetcher feeds the
   tracks channel, parser workers consume it through a
   sequencer that numbers each track, and the pruner
   starts only after both channels drain and close. The
   overlap is real: parsing runs while the fetcher still
   pages through the next collection, instead of waiting
   for every fetch to finish.
5. The playlist-file line prints twice.
   `unsyncDeletePlaylistFiles` prints "would remove
   playlist file %s" (or "removed playlist file %s") per
   file and appends the name to `results.playlistFiles`;
   `RunE` then loops over `results.playlistFiles` after
   the summary and prints the same line again. The output
   of the owner shows "would remove playlist file
   ponz.m3u" both before and after the "dry run complete:"
   line.
6. `Index.Delete` has one caller. A grep over `cmd/` and
   `entity/` shows `indexData.Delete(track)` only in
   `cmd/unsync.go:221`. Its tests live in
   `entity/index/index_test.go` (`TestDelete`,
   `TestDeleteUnknown`, `TestDeleteSharedPath`). The
   `indexData` variable stays in use by `sync` and
   `audit`; only unsync stops touching it.
7. The match rule needs no index. A same-name file with a
   different stored identifier is skipped by the ID3
   comparison alone, and a user-renamed local file is
   missed by the canonical filename lookup in both
   designs.
8. The barrier and the drain have precedents in the
   codebase. Sync's fetcher closes the decide queue when
   it finishes, and the decide workers exit on the closed
   channel (`cmd/sync.go`). The audit collector drains a
   channel into a slice with a WaitGroup (`cmd/audit.go`).
   The gated design combines both: the fetcher closes the
   tracks channel, the parser workers exit on it, the
   verdicts channel closes when the workers return, and
   the main flow drains the verdicts into a slice before
   the pruner starts.

**Assumed cause check**: The request assumes the full
index scan is the cost to remove. The code confirms it:
the scan opens one ID3 tag per file in the library. The
request assumes the sync queue approach transfers to
unsync. The code confirms it: the Spotify calls already
stream tracks into channels, and the parser can consume
them while the fetcher still pages. The request assumes
an unreadable file outside the target set aborts the run
today. The code refutes it: the indexer skips such files
silently.

**Outcome**: Specify as ordered, with the gated pipeline.
The run drops the index build. The fetcher and the parser
run concurrently over channels, and the pruner waits until
both queues are empty and closed. The playlist-file line
prints once. Q1 is decided by the owner; Q2 and Q3 stay
open below.

## User Scenarios and Testing

### User Story 1 - Targeted lookup (Priority: P1)

The owner runs `pango unsync -p ponz` on a library of
thousands of tracks. The command authenticates, fetches
the playlist, and reads the ID3 tag only of the files that
match the fetched tracks. It prints no "indexing
library..." line and no "indexed N tracks" line, and it
leaves every unrelated file untouched.

**Why this priority**: This is the inefficiency the owner
reported on the live run: a full scan of 4824 files for a
200-track target set.

**Independent Test**: Run `unsync` against a scripted
playlist in a temporary output directory that also holds
unrelated tagged files and one file with garbage bytes.
The target file is gone, the unrelated files are intact,
and the output holds no indexing line.

**Acceptance Scenarios**:

1. **Given** an output directory with a target file and
   unrelated files, **When** the owner runs `unsync -t
TRACK`, **Then** the output holds no "indexing
   library..." line and no "indexed N tracks" line.
2. **Given** the same directory, **When** the run ends,
   **Then** every unrelated file is still in place.
3. **Given** a file with garbage bytes in the directory,
   **When** the owner runs `unsync`, **Then** the run
   completes and the file is still in place.

### User Story 2 - Playlist file line prints once (Priority: P1)

The owner runs `pango unsync -p ponz` or the same run
with `--dry-run`. Each playlist file line appears exactly
once in the output, next to the deletion that produced
it, and the summary still reports the count of playlist
files removed.

**Why this priority**: The live output of the owner shows
the defect twice per file: the line is noise in a report
that must stay honest.

**Independent Test**: Run `unsync` against a scripted
playlist with both playlist files present, in real mode
and in dry-run mode, and count the occurrences of each
playlist-file line in the captured output.

**Acceptance Scenarios**:

1. **Given** a playlist with both playlist files present,
   **When** the owner runs `unsync -p <playlist>`,
   **Then** "removed playlist file Mix.m3u" appears
   exactly once and "removed playlist file Mix.pls"
   appears exactly once.
2. **Given** the same playlist, **When** the owner runs
   `unsync -p <playlist> --dry-run`, **Then** "would
   remove playlist file Mix.m3u" appears exactly once
   and "would remove playlist file Mix.pls" appears
   exactly once.
3. **Given** a completed run, **When** the owner reads
   the summary, **Then** the summary still states the
   count of playlist files removed.

### User Story 3 - Same outcomes as before (Priority: P1)

The owner runs `unsync` as in cycles 004 and 005. Every
outcome is unchanged: matching files are deleted,
mismatching same-name files survive, a track in two
collections is deleted once, a fetch failure aborts
before any deletion, a deletion failure is reported and
the run continues with a non-zero exit, and a dry run
removes nothing.

**Why this priority**: The change removes a stage, so the
proof must show that no outcome moved with it.

**Independent Test**: Run the existing `unsync` harness
suite against the changed command. Every existing test
passes with the same assertions, plus one new test for
the second same-name occurrence.

**Acceptance Scenarios**:

1. **Given** two upstream tracks that share one canonical
   filename with different identifiers, **When** the
   owner runs `unsync`, **Then** the first track deletes
   the file and the second reports absent.
2. **Given** a fetch failure, **When** the owner runs
   `unsync`, **Then** no file is deleted and the run
   exits with an error.
3. **Given** `--dry-run`, **When** the owner runs
   `unsync`, **Then** no file is removed and the summary
   uses the "would be" wording.
4. **Given** a playlist with several tracks, **When** the
   owner runs `unsync`, **Then** the per-track lines
   appear in fetch order, and a repeat run prints the
   same lines in the same order.
5. **Given** two upstream tracks that share one canonical
   filename with different identifiers, **When** the
   owner runs `unsync`, **Then** the output holds no
   "deletion failed" line for the second track.

### User Story 4 - The ponz preview without the scan (Priority: P2)

The owner runs `pango unsync -p ponz --dry-run` on the
real library, after the feature is merged and deployed.
The run prints no indexing lines and reports the same
would-be deletions as the run of 2026-11-27, subject to
library changes since then.

**Why this priority**: This is the concrete run that
ordered the feature. It is a deployment-time action, not
an implementation test.

**Independent Test**: The same test as User Story 1, with
the scripted playlist named `ponz`. The live ponz run
itself is the owner's, after the merge and the deploy,
and is tracked separately.

**Acceptance Scenarios**:

1. **Given** the feature merged and deployed, **When**
   the owner runs `unsync -p ponz --dry-run`, **Then**
   the output holds no indexing line and lists the
   would-be deletions.

## Edge Cases

- A track file is absent locally: the run reports it as
  absent, not as an error.
- A track file is unreadable or carries no Spotify
  identifier: the run reports the skip with the reason
  and never deletes the file.
- Two upstream tracks share one canonical filename with
  different identifiers: the first matching track deletes
  the file, the second reports absent.
- Two parser workers read the same file before either
  deletes it: the pruner still processes the verdicts in
  fetch order, re-stats before it removes, and the second
  track reports absent, never a deletion failure.
- A local file was renamed by the user: the canonical
  filename lookup misses it, as before. No new lookup is
  added.
- The upstream playlist is empty: no tracks are processed
  and the playlist files are still deleted.
- The playlist was renamed on Spotify after the playlist
  file was written: the file is named from the fetched
  playlist name, so a file under the old name stays. The
  run reports the playlist files it removed by name.
- The `--output` path does not exist: the change of
  directory fails and the run exits with an error, as
  before.
- Authentication fails: the run exits with an error
  before any deletion.
- A track of the unsynchronized playlist also belongs to
  the library or to another playlist: the file is deleted,
  and a later synchronization re-downloads it.
- A playlist file exists in one encoding only: the
  existing file is deleted and the missing file is not an
  error, and the line prints once.

## Requirements

### Functional Requirements

- **FR-001**: `RunE` no longer calls `auditBuildIndex`.
  The run order is change of directory, serial
  authentication, fetch and parse running concurrently,
  then the pruner after the barrier. A failure in
  authentication or fetch still aborts the run before
  any deletion.
- **FR-002**: The fetcher is one goroutine. It fetches
  the collections sequentially, in the current order
  (albums, tracks, playlists), and passes the tracks
  channel into the Spotify calls, so each track streams
  in as it arrives. It collects the fetched playlists in
  fetch order, closes the tracks channel when done, and
  reports its error on a buffered error channel. A fetch
  failure aborts the run, and the parsed verdicts are
  discarded.
- **FR-003**: The parser is four workers, the same count
  as sync's decide workers, run with
  `nursery.RunMultipleCopiesConcurrently`. Each worker
  consumes numbered work items, resolves the expected
  file from `Track.Path().Final()`, stats it, reads the
  stored Spotify identifier with `id3.OpenSpotifyID`,
  and emits one `unsyncVerdict`: the track, the
  filename, the stored identifier, the verdict kind
  (match, directory, absent, or skip), the reason, and
  the sequence number. A worker prints nothing and holds
  no shared state.
- **FR-004**: A sequencer goroutine sits between the
  fetcher and the parser. It reads tracks from the fetch
  channel, attaches a monotonically increasing sequence
  number in arrival order, and forwards each numbered
  work item to the parser channel. It closes the parser
  channel when the fetch channel closes. The sequence
  number is the fetch order, and only the sequencer
  assigns it.
- **FR-005**: The barrier is the drain. The main flow
  reads verdicts into a slice until the verdicts channel
  closes, then reads the fetch error. The pruner starts
  only after the drain ends and the fetch error is nil.
  "Queues empty and closed" means exactly this: the
  tracks channel is closed by the fetcher, the verdicts
  channel is closed after the last parser worker exits,
  and the verdict slice is fully drained.
- **FR-006**: The pruner is sequential. It sorts the
  buffered verdicts by sequence number (stable sort),
  prints every per-track line itself, and processes the
  track verdicts in fetch order. Then it removes the
  playlist files, in fetch order of the playlists. All
  user-visible per-track lines come from the pruner, so
  the output order is deterministic.
- **FR-007**: The match rule is unchanged in substance
  from cycle 004: the file is deleted only when the
  stored identifier equals the upstream track
  identifier. The parser evaluates it; the pruner
  re-stats the file before it removes.
- **FR-008**: The claim lives in the pruner. The
  `deleted` map is keyed by Spotify identifier and
  checked only by the sequential pruner: the first
  matching verdict claims the identifier, and a later
  occurrence reports as already deleted (or already
  counted in dry-run mode). The claim is marked only on
  a successful removal, or on a recorded would-be
  removal in dry-run mode.
- **FR-009**: The same-filename race rule. Before the
  pruner removes a file, it re-stats it: if the file is
  gone, the verdict reports absent. An `os.Remove`
  failure with "not exist" is reclassified as absent,
  never as a deletion failure. A skip verdict whose
  file is gone at prune time also reports absent.
- **FR-010**: Each playlist file line prints once, at
  the removal site inside
  `unsyncDeletePlaylistFiles`. The post-summary loop
  over `results.playlistFiles` is removed. The
  `playlistFiles` list stays, because the summary counts
  it.
- **FR-011**: `Index.Delete` and its tests
  (`TestDelete`, `TestDeleteUnknown`,
  `TestDeleteSharedPath` in
  `entity/index/index_test.go`) are removed, and the
  pruner drops the call. The ratified spec 006 is the
  recorded decision the constitution requires for the
  change to the exported surface of `entity/index`. The
  `indexData` variable stays, in use by `sync` and
  `audit`; `auditBuildIndex` stays, in use by `audit`.
- **FR-012**: The "indexing library..." and "indexed N
  tracks" lines are gone from unsync output. A fetch
  failure aborts the run before any deletion, a parser
  failure is a skip with a reason, and a pruner failure
  is reported, the run continues with the remaining
  verdicts, and the exit code is non-zero when any
  deletion failed.
- **FR-013**: With no collection flag, the command prints
  that there is nothing to do and exits with code zero,
  before any change of directory, authentication, or
  deletion. Dry-run semantics are unchanged from cycle 005.
  The 006 scope carries no `--library` flag, no
  `--playlist-tracks` flag, and no
  `--playlist-encoding` flag. No new flag ships in 006.

### Key Entities

- **Fetched track**: an `entity.Track` from Spotify, with
  its Spotify identifier, title, and artists.
- **Target file**: the local file named by
  `Track.Path().Final()`, `Artist - Title.mp3`, in the
  output directory. It is the only file the run stats
  and reads for that track.
- **Deleted map**: the `map[string]bool` keyed by Spotify
  identifier, checked at claim time by the sequential
  pruner only. The first matching verdict claims the
  identifier; a later occurrence reports as already
  deleted. Parser workers never touch the map.
- **Unsync verdict**: one parsed track: the track, the
  resolved filename, the stored identifier, the verdict
  kind (match, directory, absent, or skip), the reason,
  and the sequence number.
- **Sequencer**: the goroutine that numbers fetched tracks
  in arrival order and forwards them to the parser.
- **Pruner**: the sequential stage that consumes the
  buffered verdicts in fetch order, claims identifiers,
  removes files, and prints every per-track line.
- **Playlist output file**: `<LegalizeFilename(Name)>.m3u`
  or `<LegalizeFilename(Name)>.pls` in the output
  directory, where Name is the playlist name from Spotify.

## Success Criteria

### Measurable Outcomes

- **SC-001**: A command test proves that an `unsync` run
  prints no indexing line and leaves every unrelated
  file in place (User Story 1).
- **SC-002**: A command test proves that each playlist
  file line prints exactly once, in real mode and in
  dry-run mode (User Story 2).
- **SC-003**: A command test proves that a second
  same-name occurrence reports absent after the first
  deletes the file, with no "deletion failed" line, and
  the existing harness suite passes with unchanged
  outcomes (User Story 3).
- **SC-004**: The full test suite passes with the
  per-package coverage gate at 90 percent, locally and in
  CI. Unit tests stub every network dependency: no test
  calls a live provider.
- **SC-005**: A bounded live check on a scratch directory
  proves that a dry run prints no indexing line and
  leaves every scratch file in place.
- **SC-006**: A command test proves that the per-track
  lines appear in fetch order and repeat identically
  across runs (User Story 3).

## Assumptions

- **A1**: The delete stage is a gated pipeline: the
  fetcher and the parser run concurrently over channels,
  and the sequential pruner starts only after both
  queues are empty and closed. The verdict slice is the
  buffer between the parser and the pruner.
- **A2**: `Index.Delete` is removed with its tests, and
  the ratified spec 006 is the recorded decision for the
  exported-surface change.
- **A3**: The summary counts and wording stay as in
  cycles 004 and 005.
- **A4**: Playlist files are deleted on a successful
  fetch of that playlist, even when some of its tracks
  were skipped for a mismatching identifier. Unchanged
  from cycle 004.
- **A5**: Every requested collection is fetched before
  any deletion starts. A fetch failure aborts before any
  file is deleted. Unchanged from cycle 004.
- **A6**: The live ponz dry run is the owner's, after the
  merge and the deploy. Implementation verification uses
  a scripted playlist named `ponz`, never the real
  playlist or the real library.
- **A7**: The fetcher is one goroutine and fetches the
  collections sequentially, in the current order. The
  Spotify client is never called concurrently, so no
  client-concurrency question arises.
- **A8**: The parser runs four workers, the same count as
  sync's decide workers, and both channels use the
  `pipelineBuffer` depth from sync. Only the sequencer
  assigns sequence numbers.
- **A9**: No external writer touches the output directory
  during a run. The re-stat rule covers only deletions
  made by the run itself.

## Open Questions for Ratification

- **Q1**: Decided by the owner on 2026-11-27: the
  fetcher and the parser run concurrently over channels,
  and the pruner waits until their queues are empty and
  closed. The fetcher streams tracks into the tracks
  channel, a sequencer numbers them in arrival order,
  four parser workers emit verdicts, the main flow drains
  the verdicts into a slice, and the sequential pruner
  consumes the slice in fetch order. The claim stays in
  the pruner, so it needs no mutex.
- **Q2**: Should `Index.Delete` and its tests be removed,
  or should the method stay as unused public API?
  Proposed: remove. It was introduced by cycle 004
  solely for unsync, and with no index built it has no
  caller. The ratified spec 006 is the recorded decision
  the constitution requires. Ratified 2026-11-27 as
  proposed.
- **Q3**: Should the implementation fold into PR #286, or
  should it open a separate stacked PR on `feat/unsync`?
  Proposed: fold into #286. The change touches the same
  lines #286 introduces, #286 is unmerged, and one
  feature keeps one commit and one review. Ratified
  2026-11-27 as proposed.
