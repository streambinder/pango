# Feature Specification: Dry-Run Flag for Unsync

**Feature Branch**: `feat/unsync`

**Created**: 2026-11-26

**Status**: Ratified 2026-11-26. The owner ratified this text
as proposed: Q1 no short flag, Q2 same counters with "would
be" wording, Q3 fold the implementation into PR #286.
Implementation follows tasks T003–T013. This cycle depends
on cycle 004: the `unsync` command exists only on branch
`feat/unsync`, in PR #286, open and unmerged.

**Input**: Order by the owner, 2026-11-26: add a dry-run flag
to the `unsync` subcommand that simulates the run but
removes nothing at the end.

## Hypothesis Investigation

**Hypothesis**: A dry-run flag can preview exactly what
`unsync` would delete, by running every read stage
identically and skipping only the removal calls, without
changing the match, skip, or report logic.

**Evidence** (verified on 2026-11-26, against real code:
`unsync` on branch `feat/unsync`, commit `b603317`):

1. The command has exactly two removal sites. In
   `unsyncRemoveFile` (`cmd/unsync.go`), `os.Remove`
   deletes the track file, `indexData.Delete` drops the
   index entry, and the `deleted` map marks the identifier.
   In `unsyncDeletePlaylistFiles` (`cmd/unsync.go`),
   `os.Remove` runs once per encoding. Every other step
   only reads: `auditBuildIndex` walks the output
   directory and reads ID3 frames, `auditAuthenticate`
   opens a Spotify session, `unsyncFetchAll` pulls the
   collections, and `unsyncDeleteTrack` stats the file and
   reads the stored identifier. A dry run differs from a
   real run at exactly two call sites.
2. The in-memory index is never consulted after the build
   in this command: the deletion path checks the `deleted`
   map, not the index (`cmd/unsync.go`,
   `unsyncDeleteTrack`). `Index.Delete` matters only for
   later lookups in the same run, and `unsync` performs
   none. Skipping `Index.Delete` in a dry run is still
   required: a dry run must leave every state untouched,
   in memory as on disk, so the simulation is provably a
   pure read.
3. The deduplication map must still record would-be-deleted
   identifiers in a dry run. A track in two requested
   collections is reported once as would-be-deleted and
   once as already counted, exactly as a real run reports
   it once as deleted and once as already deleted. The
   wording changes, the counting does not.
4. No `-n` short flag exists in any command: a search for
   `"n"` over `cmd/*.go` returns nothing. Boolean flags in
   this codebase are long-only: `sync` offers `--plain`
   and `--skip-lyrics` with no short form, `audit` offers
   `--library` with no short form (`cmd/sync.go`,
   `cmd/audit.go`). A long-only `--dry-run` follows that
   convention.
5. The exit path depends on `results.failed`, which only
   `os.Remove` failures increment (`cmd/unsync.go`). A dry
   run never calls `os.Remove`, so `failed` stays zero and
   the exit is zero whenever index, auth, and fetch
   succeed. A fetch failure still aborts before any
   would-be deletion, with the same error as a real run.
6. A dry run cannot predict a deletion failure. A
   non-empty directory carrying a track filename, or a
   read-only file, fails only when `os.Remove` runs
   (spec 004, acceptance case of US4). The dry run reports
   the match, not the removability. This is a stated
   limit, not a defect: the report must never claim a file
   "would be deleted" as a promise that it will succeed.
7. The empty-selection check runs before any work in
   `RunE` (`cmd/unsync.go`). Bare `unsync --dry-run`
   prints "nothing to do" and exits zero, exactly like
   bare `unsync`, without a change of directory, an index
   scan, or an authentication.

**Assumed cause check**: The request assumes "simulate"
means "run everything except the removal". The code
confirms it: the read stages are pure, the removal sites
are two, and the report already distinguishes deleted,
skipped, absent, and already-deleted files. The request
assumes the summary can reuse the same counts. The code
confirms it, with the verb changed: the counters record
outcomes, and only the deletion outcome changes meaning,
from "deleted" to "would be deleted".

**Outcome**: Specify as ordered. One new boolean flag, two
call sites gated, and wording changes in the per-track
lines and in the summary. The open points go to the
ratification questions below.

## User Scenarios and Testing

### User Story 1 - Preview a playlist unsync (Priority: P1)

The owner runs `pango unsync -p ponz --dry-run`. The
command lists every track that would be deleted and both
playlist files that would be removed. Nothing leaves the
library.

**Why this priority**: This is the use case that ordered
the feature: the owner previews the ponz removal before
the real run.

**Independent Test**: Run `unsync --dry-run` against a
scripted Spotify playlist and tagged fixture files in a
temporary output directory. Every fixture file is still
there after the run, and the output lists what would be
deleted.

**Acceptance Scenarios**:

1. **Given** a playlist whose tracks exist locally with
   matching Spotify identifiers, **When** the owner runs
   `unsync -p <playlist> --dry-run`, **Then** each track
   reports "would delete" and both `<name>.m3u` and
   `<name>.pls` report "would remove playlist file".
2. **Given** the same dry run, **When** the run ends,
   **Then** every track file and both playlist files are
   still in the output directory, and the exit code is
   zero.
3. **Given** a playlist file that exists in only one
   encoding, **When** the owner runs the dry run, **Then**
   the existing file reports "would remove" and the
   missing file is not an error.

### User Story 2 - Preview an album or a track (Priority: P1)

The owner runs `pango unsync -a <album> --dry-run` or
`pango unsync -t <track> --dry-run`. The command reports
only the local files whose stored Spotify identifier
matches the upstream track. No playlist file is reported,
and no file is removed.

**Why this priority**: Album and track are the other two
collection types the order names. The identifier match
is the safety rule of the whole feature, and the preview
must prove it before the real run.

**Independent Test**: Run `unsync --dry-run` against a
scripted album and a scripted track, with fixture files
that include one file whose stored identifier differs
from the upstream track. The matching files report "would
delete", the mismatching file reports a skip, and every
file survives.

**Acceptance Scenarios**:

1. **Given** an album whose tracks exist locally with
   matching identifiers, **When** the owner runs
   `unsync -a <album> --dry-run`, **Then** each track
   reports "would delete" and no playlist file is
   reported.
2. **Given** a local file whose canonical filename matches
   the upstream track but whose stored identifier differs,
   **When** the owner runs the dry run, **Then** the file
   reports a skip with the reason and survives.
3. **Given** a track that appears in two requested
   collections, **When** the owner runs the dry run with
   both, **Then** the first occurrence reports "would
   delete", the second reports "already counted", and the
   file is deleted zero times.

### User Story 3 - Bare unsync with dry-run does nothing (Priority: P1)

The owner runs `pango unsync --dry-run` with no collection
flag. The command prints that there is nothing to do and
exits successfully. It touches nothing: no change of
directory, no index scan, no authentication, no deletion.

**Why this priority**: The order states the rule
explicitly for `unsync`. A simulation flag must not wake
a command that names nothing.

**Independent Test**: Run bare `unsync --dry-run` against
a temporary output directory that holds tagged fixture
files. The output states that there is nothing to do, the
exit code is zero, and every fixture file is still there.

**Acceptance Scenarios**:

1. **Given** no collection flag, **When** the owner runs
   `unsync --dry-run`, **Then** the run prints that there
   is nothing to do and exits with code zero.
2. **Given** no collection flag, **When** the owner runs
   `unsync --dry-run`, **Then** no file in the output
   directory changes.

### User Story 4 - The dry-run report stays honest (Priority: P2)

The owner runs `unsync --dry-run` and reads what would
happen. The run reports every file that would be deleted,
every skipped file with its reason, and a final summary
with counts. A fetch failure aborts the run with an error.
The exit code is zero whenever the read stages succeed,
even when a real run would fail a deletion.

**Why this priority**: The preview is only useful when
the owner can trust it. A fetch failure must never look
like a completed preview, and the preview must never
promise a deletion it cannot prove.

**Independent Test**: Run `unsync --dry-run` against a
scripted Spotify transport that fails the fetch, with
fixture files present: the run errors and nothing is
deleted. Run `unsync --dry-run` with one undeletable
fixture (a non-empty directory that carries a track
filename): the run reports "would delete", exits zero,
and the directory is intact.

**Acceptance Scenarios**:

1. **Given** a Spotify fetch that fails, **When** the owner
   runs `unsync --dry-run`, **Then** no file is deleted
   and the run exits with an error.
2. **Given** a track file that cannot be deleted, **When**
   the owner runs the dry run, **Then** the run reports
   "would delete" and exits zero: the dry run cannot
   predict the deletion failure.
3. **Given** a completed dry run, **When** the owner reads
   the output, **Then** a final summary states the
   would-be-deleted, skipped, absent, and already-counted
   counts and the playlist files that would be removed.

### User Story 5 - Preview the ponz removal (Priority: P2)

The owner runs `pango unsync -p ponz --dry-run` on the
real library, after the feature is merged and deployed.
The run lists the ponz tracks and both playlist files
that would leave the library, and removes nothing.

**Why this priority**: This is the concrete run the owner
asked to be reminded of. The dry run previews it before
the real run.

**Independent Test**: The same test as User Story 1, with
the scripted playlist named `ponz`. The live ponz dry run
itself is the owner's, after the merge and the deploy.

**Acceptance Scenarios**:

1. **Given** the feature merged and deployed, **When** the
   owner runs `unsync -p ponz --dry-run`, **Then** the
   ponz tracks and both playlist files are listed as
   would-be-removed, and every file stays in the library.

## Edge Cases

- A track file is absent locally: the run reports it as
  absent, not as an error, as in a real run.
- A track file is unreadable or carries no Spotify
  identifier: the run reports the skip with the reason
  and never deletes the file.
- The upstream playlist is empty: no tracks are processed
  and the playlist files are still reported as
  would-be-removed.
- The playlist was renamed on Spotify after the playlist
  file was written: the file is named from the fetched
  playlist name, so a file under the old name stays, as
  in a real run.
- The `--output` path does not exist: the change of
  directory fails and the run exits with an error, as the
  synchronization does.
- Authentication fails: the run exits with an error before
  any would-be deletion is reported.
- A track of the unsynchronized playlist also belongs to
  the library or to another playlist: the file is reported
  as would-be-deleted, and a later synchronization would
  re-download it after a real run.
- A track filename is a non-empty directory: the dry run
  reports "would delete" and exits zero. The removal
  would fail in a real run; the dry run cannot know that.

## Requirements

### Functional Requirements

- **FR-001**: A new `--dry-run` boolean flag is registered
  on the `unsync` command, long form only, with no short
  flag. The default is false.
- **FR-002**: With no collection flag, the command prints
  that there is nothing to do and exits with code zero,
  before any change of directory, index scan,
  authentication, or deletion, whether or not `--dry-run`
  is set.
- **FR-003**: A dry run builds the index with
  `BuildWithProgress` from the output directory,
  authenticates to Spotify, and fetches every requested
  collection, exactly like a real run. A failure in any
  of these steps aborts the run with the same error and
  exit code as a real run.
- **FR-004**: A dry run performs no `os.Remove` call, for
  tracks or for playlist files.
- **FR-005**: A dry run performs no `Index.Delete` call:
  the in-memory index is untouched.
- **FR-006**: The deduplication map still records
  would-be-deleted identifiers across the whole run. A
  second occurrence reports "already counted" and
  increments the already-deleted count.
- **FR-007**: Per-track lines in a dry run read "would
  delete %s by %s". Skip and absent lines are unchanged.
- **FR-008**: For each requested playlist, the playlist
  files report "would remove playlist file %s" and are
  counted in the playlist-files list. No file is removed.
- **FR-009**: The summary line in a dry run reads "dry run
  complete: %d would be deleted, %d skipped, %d absent,
  %d already counted, %d playlist files would be
  removed", followed by the would-be-removed playlist
  filenames.
- **FR-010**: `results.failed` stays zero in a dry run.
  The exit code is zero when index, auth, and fetch
  succeed, even when a real run would fail a deletion.
- **FR-011**: A dry run downloads nothing, needs no
  `ffmpeg`, and calls no `ValidateEnvironment`, as in
  spec 004.
- **FR-012**: The flag composes with `-o`, `-p`, `-a`,
  and `-t` exactly as in spec 004.

### Key Entities

- **Dry-run mode**: a boolean run mode of the `unsync`
  command. In dry-run mode every read stage runs
  identically, and every removal call is skipped.
- **Local track file**: an mp3 file in the output
  directory, as in spec 004. Its identity is the Spotify
  identifier in the "Spotify ID" frame and its canonical
  filename, `Artist - Title.mp3`.
- **Playlist output file**: `<LegalizeFilename(Name)>.m3u`
  or `<LegalizeFilename(Name)>.pls` in the output
  directory, as in spec 004.

## Success Criteria

### Measurable Outcomes

- **SC-001**: A command test proves that `unsync -p
--dry-run` lists the playlist tracks and both playlist
  files as would-be-removed and removes nothing
  (User Story 1).
- **SC-002**: A command test proves that `unsync -a
--dry-run` and `unsync -t --dry-run` report only
  matching files, that a mismatching same-name file is
  skipped, and that a track in two collections is counted
  once (User Story 2).
- **SC-003**: A command test proves that bare `unsync
--dry-run` prints that there is nothing to do, exits
  with code zero, and changes no file (User Story 3).
- **SC-004**: A command test proves that a fetch failure
  in a dry run errors with nothing deleted, and that an
  undeletable fixture reports "would delete" with a zero
  exit code (User Story 4).
- **SC-005**: The full test suite passes with the
  per-package coverage gate at 90 percent, locally and in
  CI. Unit tests stub every network dependency: no test
  calls a live provider.

## Assumptions

- **A1**: `--dry-run` has no short flag, following the
  long-only boolean convention of `sync` (`--plain`,
  `--skip-lyrics`) and `audit` (`--library`).
- **A2**: The summary reuses the same counters with
  "would be" wording (see Q2). The already-deleted count
  reads as "already counted" in dry-run mode.
- **A3**: A dry run cannot predict a deletion failure. It
  reports matches, not removability.
- **A4**: The duplicate line reads "already counted" in
  dry-run mode, instead of "already deleted".
- **A5**: The implementation stacks on `feat/unsync` and
  PR #286 (see Q3). No implementation starts before this
  spec is ratified.
- **A6**: No confirmation prompt is added. The dry run is
  the safe preview instead.

## Open Questions for Ratification

- **Q1**: Should `--dry-run` carry a short flag `-n`?
  Proposed: no short flag, long form only, following the
  codebase convention. `-n` is free in every command, so
  the choice is convention, not a conflict.
  Ratified 2026-11-26: no short flag.
- **Q2**: Should the summary reuse the same counters with
  "would be" wording, as in FR-009? Proposed: yes, same
  counters, same order, verbs changed. Rejected
  alternative: a separate would-be-deleted counter next
  to the deleted count, which would double every
  downstream parsing of the summary for no new
  information.
  Ratified 2026-11-26: yes, same counters.
- **Q3**: Should the implementation fold into PR #286, or
  land as a separate stacked PR on `feat/unsync`?
  Proposed: fold into PR #286. The flag changes the same
  lines the PR introduces, #286 is unmerged, and one
  feature keeps one commit and one review. See the plan
  for the full reasoning.
  Ratified 2026-11-26: fold into PR #286.
