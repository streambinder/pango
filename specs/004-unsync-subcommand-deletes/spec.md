# Feature Specification: Unsync Subcommand Deletes Local Tracks

**Feature Branch**: `feat/unsync`

**Created**: 2026-11-26

**Status**: Ratified 2026-11-26. The owner ordered this change on 2026-11-26,
after the investigation below, and ratified this text as proposed on
2026-11-26, with the answers recorded under Open Questions for
Ratification. Implementation follows the plan and tasks of this cycle.

**Input**: Order by the owner, 2026-11-26: add an `unsync`
subcommand. It deletes local tracks from the index and from
the output music directory, managed the same way `pango sync`
manages the `--output` flag. It allows the collection types
playlist (which deletes tracks and playlist output files,
both m3u and pls), album, and track. If the upstream track
(from playlist, album, or track) does not match the target
file's Spotify ID, the command skips it: it deletes only
matching local Spotify IDs for the requested collections. At
least one collection type (playlist, album, track) must be
given; if not explicit, `unsync` exits gracefully with
nothing to do.

## Hypothesis Investigation

**Hypothesis**: Pango can delete exactly the local files that
belong to given Spotify collections, using the same identity
that the synchronization uses, without a persistent index and
without touching files that belong to other collections.

**Evidence** (verified on 2026-11-26, against real code:
pango at master `90e195f`):

1. The index is in memory only. `entity/index/index.go`
   holds two maps, `ids` and `paths`. `BuildWithProgress`
   walks the output directory, reads the Spotify
   identifier of every `.mp3` file from its ID3 frames,
   and marks each entry Offline. No file on disk persists
   the index. "Delete from index" has no persistent
   target: deleting the file removes the track from every
   later index build. Inside one run the entry must leave
   the in-memory maps, which needs a new `Index.Delete`
   method: no delete method exists today.
2. The local identity of a file is its Spotify identifier
   in a user-defined text (TXXX) frame named "Spotify ID"
   (`entity/id3/id3.go`, `OpenSpotifyID` and
   `SpotifyID`), and its canonical filename is
   `Track.Path().Final()`, `LegalizeFilename` of
   `Artist - Title.mp3` (`entity/track.go`). The
   synchronization uses the same two values in
   `decideClassify` and in `routineInstall` (`cmd/sync.go`).
   `show` and `audit` read the same frames
   (`cmd/show.go`, `cmd/audit.go`).
3. Playlist output files live in the output directory.
   The encoders write `LegalizeFilename` of the playlist
   name plus `.m3u` or `.pls` into the current directory
   (`entity/playlist/m3u.go`, `entity/playlist/pls.go`),
   which after the change of directory is the output
   directory. The synchronization writes only the
   `--playlist-encoding` encoding per run, so both files
   can exist from different runs. The name comes from
   Spotify (`spotify/playlist.go`, `playlistEntity`); the
   flag resolves through `personalPlaylistNameToID`, a
   name lookup with a raw identifier fallback.
4. The upstream fetch reuses `spotify.Client.Playlist`,
   `Album`, and `Track` (`spotify/playlist.go`,
   `spotify/album.go`, `spotify/track.go`). Each emits one
   `*entity.Track` per track, with its Spotify identifier.
   `cmd/audit.go` already proves the sequential shape the
   command needs: index, authenticate, fetch, with plain
   output and the same `-p`, `-a`, and `-t` flags.
   `auditFetch` shows that a fetch failure aborts before
   any report. `unsync` follows `audit`, not the
   synchronization pipeline: no decide, collect, process,
   install, or mix stage, no download, no provider search,
   and no `ffmpeg`. `ValidateEnvironment` requires only
   `ffmpeg` on PATH (`sys/cmd/environment.go`), and `audit`
   does not call it.
5. One track can sit in several collections. The index
   keys by Spotify identifier, so deduplication by
   identifier deletes the file once. A track in an
   unsynchronized playlist and in the library (or in
   another playlist) is deleted all the same: `unsync` is
   collection-scoped, not membership-aware. A later
   synchronization re-downloads it, because the index no
   longer holds it. This is a consequence, not a defect.
6. Two upstream tracks can share one canonical filename
   with different identifiers. The synchronization aborts
   on that filename collision (`decideClassify`). For
   `unsync` the identifier check decides: the file is
   deleted only when its stored identifier equals the
   upstream track identifier. A mismatching file is
   skipped, so a same-name file of another track survives.

**Assumed cause check**: The request assumes a persistent
index to delete from. The code shows there is none: the
index rebuilds from the files on every run. Deleting the
file is the deletion from the index. The request assumes
the playlist files follow the playlist name. The code
confirms it, with the name from Spotify, not from the flag
value. The request assumes the match is by Spotify
identifier. The code confirms it: the identifier frame is
the only stored identity, and the filename alone cannot
distinguish two tracks that share artist and title.

**Outcome**: Specify as ordered. No persistent index work,
one new `Index` method, and one new command that reuses the
`audit` shape. The open points go to the ratification
questions below.

## User Scenarios and Testing

### User Story 1 - Unsync a playlist (Priority: P1)

The owner runs `pango unsync -p ponz`. The command deletes
every track of the playlist from the local library and
removes the `ponz.m3u` and `ponz.pls` playlist files.

**Why this priority**: This is the use case that ordered the
feature: the playlist leaves the local library in one run.

**Independent Test**: Run `unsync` against a scripted
Spotify playlist and tagged fixture files in a temporary
output directory. The track files and both playlist files
are gone after the run.

**Acceptance Scenarios**:

1. **Given** a playlist whose tracks exist locally with
   matching Spotify identifiers, **When** the owner runs
   `unsync -p <playlist>`, **Then** the track files are
   deleted and both `<name>.m3u` and `<name>.pls` are
   deleted.
2. **Given** a playlist file that exists in only one
   encoding, **When** the owner runs `unsync -p
<playlist>`, **Then** the existing file is deleted and
   the missing file is not an error.
3. **Given** a playlist whose tracks do not exist locally,
   **When** the owner runs `unsync -p <playlist>`, **Then**
   the tracks report as absent and the playlist files are
   still deleted.

### User Story 2 - Unsync an album or a track (Priority: P1)

The owner runs `pango unsync -a <album>` or `pango unsync
-t <track>`. The command deletes only the local files whose
stored Spotify identifier matches the upstream track. No
playlist file is touched.

**Why this priority**: Album and track are the other two
collection types the order names. The identifier match is
the safety rule of the whole feature.

**Independent Test**: Run `unsync` against a scripted album
and a scripted track, with fixture files that include one
file whose stored identifier differs from the upstream
track. The matching files are gone and the mismatching
file survives.

**Acceptance Scenarios**:

1. **Given** an album whose tracks exist locally with
   matching identifiers, **When** the owner runs `unsync
-a <album>`, **Then** the track files are deleted and
   no playlist file is touched.
2. **Given** a track that exists locally with a matching
   identifier, **When** the owner runs `unsync -t TRACK`,
   **Then** the track file is deleted.
3. **Given** a local file whose canonical filename matches
   the upstream track but whose stored identifier differs,
   **When** the owner runs `unsync`, **Then** the file
   survives and the run reports the skip with the reason.
4. **Given** a track that appears in two requested
   collections, **When** the owner runs `unsync` with
   both, **Then** the file is deleted once and the second
   occurrence reports as already deleted.

### User Story 3 - No collection does nothing (Priority: P1)

The owner runs `pango unsync` with no collection flag. The
command prints that there is nothing to do and exits
successfully. It touches nothing: no change of directory, no
index scan, no authentication, no deletion.

**Why this priority**: The order states the rule
explicitly. A destructive command must stay inert when the
owner names nothing.

**Independent Test**: Run bare `unsync` against a
temporary output directory that holds tagged fixture
files. The output states that there is nothing to do, the
exit code is zero, and every fixture file is still there.

**Acceptance Scenarios**:

1. **Given** no collection flag, **When** the owner runs
   `unsync`, **Then** the run prints that there is nothing
   to do and exits with code zero.
2. **Given** no collection flag, **When** the owner runs
   `unsync`, **Then** no file in the output directory
   changes.

### User Story 4 - The report stays honest (Priority: P2)

The owner runs `unsync` and reads what happened. The run
reports every deleted file, every skipped file with its
reason, and a final summary with counts. A fetch failure
aborts the run before any deletion. A deletion failure is
reported, the run continues with the remaining tracks, and
the exit code is non-zero when any deletion failed.

**Why this priority**: The command deletes user data. The
owner must see exactly what left the library and what did
not, and a partial upstream read must never look like a
complete deletion.

**Independent Test**: Run `unsync` against a scripted
Spotify transport that fails the fetch, with fixture files
present: nothing is deleted. Run `unsync` with one
undeletable fixture (a non-empty directory that carries a
track filename): the failure is reported, the other tracks
are deleted, and the exit code is non-zero.

**Acceptance Scenarios**:

1. **Given** a Spotify fetch that fails, **When** the owner
   runs `unsync`, **Then** no file is deleted and the run
   exits with an error.
2. **Given** a track file that cannot be deleted, **When**
   the owner runs `unsync`, **Then** the failure is
   reported, the remaining tracks are still processed,
   and the exit code is non-zero.
3. **Given** a completed run, **When** the owner reads the
   output, **Then** a final summary states the deleted,
   skipped, absent, and already-deleted counts and the
   playlist files removed.

### User Story 5 - The ponz playlist leaves the library (Priority: P2)

The owner runs `pango unsync -p ponz` on the real library,
after the feature is merged and deployed. The tracks of the
ponz playlist and the `ponz.m3u` and `ponz.pls` files leave
the local library.

**Why this priority**: This is the concrete run the owner
asked to be reminded of. It is a deployment-time action,
not an implementation test.

**Independent Test**: The same test as User Story 1, with
the scripted playlist named `ponz`. The live ponz run
itself is the owner's, after the merge and the deploy, and
is tracked separately.

**Acceptance Scenarios**:

1. **Given** the feature merged and deployed, **When** the
   owner runs `unsync -p ponz`, **Then** the ponz tracks
   and both playlist files leave the local library.

## Edge Cases

- A track file is absent locally: the run reports it as
  absent, not as an error.
- A track file is unreadable or carries no Spotify
  identifier: the run reports the skip with the reason and
  never deletes the file.
- The upstream playlist is empty: no tracks are processed
  and the playlist files are still deleted.
- The playlist was renamed on Spotify after the playlist
  file was written: the file is named from the fetched
  playlist name, so a file under the old name stays. The
  run reports the playlist files it removed by name.
- The `--output` path does not exist: the change of
  directory fails and the run exits with an error, as the
  synchronization does.
- Authentication fails: the run exits with an error before
  any deletion.
- A track of the unsynchronized playlist also belongs to
  the library or to another playlist: the file is deleted,
  and a later synchronization re-downloads it.

## Requirements

### Functional Requirements

- **FR-001**: A new `unsync` subcommand is registered on
  the root command. It offers `-o` and `--output` (default
  XDG music directory, same as `sync`), `-p` and
  `--playlist`, `-a` and `--album`, `-t` and `--track`,
  with the same spellings as `sync` and `audit`.
- **FR-002**: With no collection flag, the command prints
  that there is nothing to do and exits with code zero,
  before any change of directory, index scan,
  authentication, or deletion.
- **FR-003**: The output management matches `sync` and
  `audit`: the command resolves `--output` (default XDG
  music directory) and changes directory into it, so every
  relative path resolves there.
- **FR-004**: The command builds the in-memory index from
  the output directory with `BuildWithProgress`, then
  authenticates to Spotify, then fetches every requested
  collection. A failure in any of these steps aborts the
  run before any deletion.
- **FR-005**: For each fetched track, the command resolves
  the expected local file from `Track.Path().Final()`,
  reads the stored Spotify identifier with
  `id3.OpenSpotifyID`, and deletes the file only when the
  stored identifier equals the upstream track identifier.
- **FR-006**: Deduplication runs by Spotify identifier
  across the whole run: an identifier deleted once is not
  deleted again, and a later occurrence reports as already
  deleted.
- **FR-007**: Skipped files (absent, unreadable, untagged,
  mismatching identifier) are reported with the reason and
  are never deleted.
- **FR-008**: For each requested playlist, after its
  tracks, the command deletes `LegalizeFilename` of the
  fetched playlist name plus `.m3u` and plus `.pls` when
  present. A missing playlist file is not an error.
- **FR-009**: A failed deletion is reported, the run
  continues with the remaining tracks, and the exit code
  is non-zero when any deletion failed.
- **FR-010**: The run prints a final summary: deleted,
  skipped, absent, and already-deleted counts, and the
  playlist files removed.
- **FR-011**: The command downloads nothing, needs no
  `ffmpeg`, and calls no provider search. It never calls
  `ValidateEnvironment`.
- **FR-012**: A new `Index.Delete` method removes the
  identifier and the path entries of a deleted track from
  the in-memory index.
- **FR-013**: The 004 scope carries no `--library` flag,
  no `--playlist-tracks` flag, and no `--playlist-encoding`
  flag.

### Key Entities

- **Local track file**: an mp3 file in the output
  directory. Its identity is the Spotify identifier in
  the "Spotify ID" frame and its canonical filename,
  `Artist - Title.mp3`.
- **Upstream track**: an `entity.Track` from Spotify, with
  its Spotify identifier, title, and artists.
- **Playlist output file**: `<LegalizeFilename(Name)>.m3u`
  or `<LegalizeFilename(Name)>.pls` in the output
  directory, where Name is the playlist name from Spotify.

## Success Criteria

### Measurable Outcomes

- **SC-001**: A command test proves that `unsync -p`
  deletes the playlist tracks and both playlist files
  (User Story 1).
- **SC-002**: A command test proves that `unsync -a` and
  `unsync -t` delete only matching files and that a
  mismatching same-name file survives (User Story 2).
- **SC-003**: A command test proves that bare `unsync`
  prints that there is nothing to do, exits with code
  zero, and changes no file (User Story 3).
- **SC-004**: A command test proves that a fetch failure
  deletes nothing and that a deletion failure is reported
  with a non-zero exit code (User Story 4).
- **SC-005**: The full test suite passes with the
  per-package coverage gate at 90 percent, locally and in
  CI. Unit tests stub every network dependency: no test
  calls a live provider.

## Assumptions

- **A1**: Playlist files are deleted on a successful fetch
  of that playlist, even when some of its tracks were
  skipped for a mismatching identifier. The playlist as a
  collection is gone; an orphaned skipped file stays and
  a later synchronization or audit still sees it.
- **A2**: A deletion failure does not abort the run. The
  run continues and exits non-zero at the end.
- **A3**: Every requested collection is fetched before any
  deletion starts. A fetch failure aborts before any file
  is deleted.
- **A4**: No `--library`, `--playlist-tracks`, or
  `--playlist-encoding` flag ships in 004.
- **A5**: Playlist filenames come from the fetched
  playlist name, not from the flag value.
- **A6**: The live ponz run is the owner's, after the
  merge and the deploy. Implementation verification uses a
  scripted playlist named `ponz`, never the real playlist
  or the real library.

## Open Questions for Ratification

Ratified 2026-11-26 as proposed.

- **Q1**: Should `--playlist-tracks` (delete the tracks,
  keep no playlist file) be supported? Answer: out of
  scope for 004.
- **Q2**: Is A1 accepted: the playlist files are deleted
  even when some tracks were skipped for a mismatching
  identifier? Answer: yes, as specified.
- **Q3**: Is A2 accepted: a deletion failure continues the
  run with a non-zero exit code at the end, instead of
  aborting at the first failure? Answer: yes.
