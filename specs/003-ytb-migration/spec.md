# Feature Specification: YouTube Search and Download through ytb-cli

**Feature Branch**: `feat/ytb-migration`

**Created**: 2026-11-22

**Status**: Draft. The owner ordered this change on 2026-11-22.
The owner ratifies this text at the review of the pull request.
Implementation follows in a later cycle; this cycle covers
specify, plan, and tasks only.

**Input**: Order by the owner, 2026-11-22: switch pango entirely
from the yt-dlp binary to the tamnd/ytb-cli backing library
(`github.com/tamnd/ytb-cli`, Apache-2.0, latest stable release
v0.6.0, pinned exactly), and remove yt-dlp everywhere. ffmpeg
stays: the processor uses it directly for loudness detect and
normalize, independent of yt-dlp.

## Hypothesis Investigation

**Hypothesis**: The ytb-cli library (`ytb` package) can replace
both yt-dlp call sites, search and download, with no scoring
input lost, no behaviour lost, and no yt-dlp fallback path
taken.

**Evidence** (verified on 2026-11-22, against real code: pango
at master `c0802d5`, and a read-only reference clone of
ytb-cli at exactly v0.6.0):

1. The current state is two yt-dlp call sites. Search runs
   `yt-dlp --dump-json --flat-playlist --no-download
--no-warnings "ytsearch10:<query>"` through
   `sys/cmd.YouTubeDlSearch` (`sys/cmd/youtubedl.go`), and the
   provider maps the flat JSON onto `youTubeResult`
   (`provider/youtube.go`). Download runs `yt-dlp --format
bestaudio/best --extract-audio --audio-format <ext>
--audio-quality 0 --output <stem>.%(ext)s --continue
--no-overwrites <url>` through `sys/cmd.YouTubeDl`, reached
   from `downloader/youtubedl.go`. `ValidateEnvironment`
   requires both `ffmpeg` and `yt-dlp` on PATH.
   `Dockerfile` installs both. The target file is always mp3
   (`entity.TrackFormat = "mp3"`).
2. Search maps onto `Client.Search(ctx, query, filters,
PageOptions, emit)` (`ytb/search.go`). It posts to the
   InnerTube `/search` endpoint as the WEB client, parses the
   response in draw order through `ParseSearchResults`, and
   streams each row to `emit` as a `Video`, `Channel`, or
   `Playlist` value. Returning `ErrStop` from `emit` halts
   iteration cleanly. `SearchFilters{Type: "video"}` restricts
   the rows to videos, the way `ytsearch10` did.
3. The scoring inputs do NOT all survive the switch. This is
   the critical finding, and it decides the design below:
   - `parseVideoRenderer` (`ytb/parse.go`) reads the video ID,
     the title, `descriptionSnippet`, the duration out of
     `lengthText`, the owner name and channel ID out of
     `ownerText`, and the view count out of `viewCountText`.
     It reads no badge of any kind: the `badges` array is
     consulted only for the LIVE state. The `Video` struct
     carries no verified or official-artist field at all.
   - `parseLockupViewModel` (`ytb/lockup.go`), the second row
     shape a search serves, reads the title, the channel, the
     rounded view count, the relative publish text, and the
     duration off a thumbnail badge. It carries NO
     description at all.
   - `parseChannelRenderer` (`ytb/channelparse.go`) likewise
     carries no badge: its `Channel` rows have no verified
     and no artist flag.
   - The badges exist in exactly one place: `Channel.IsVerified`
     and `Channel.IsArtist`, filled only by `channelBadges`
     (`ytb/channelparse.go`), which reads the
     `CHECK_CIRCLE_FILLED` and `AUDIO_BADGE` image names
     beside the title of a full channel page, reached through
     `Client.FetchChannel` (`ytb/channel.go`).
   - The view count is rounded. Both row parsers run the
     rendered text ("1.7B views") through `parseCountText`,
     where yt-dlp returned the exact integer. The views
     score counts digits, so rounding moves it only at a
     digit boundary.
   - There is no publish year. A search row carries only the
     relative `PublishedText` ("1 month ago"). The year gate
     was already dropped in cycle 002, so nothing is lost
     here.
4. The description question resolves in favour of the new
   backend. `descriptionSnippet` is the snippet the old
   scraper read, so the cycle 002 finding stands: the first
   line of the description is the scoring input, and the
   full-description credits penalty does not return. Where a
   row is a lockup, the description is absent rather than
   wrong, and an absent description scores from title plus
   owner only. No per-video detail fetch is needed to fill
   it: `FetchVideo` would return the full `shortDescription`
   and reintroduce the credits penalty, at one watch-page
   fetch per video.
5. The badge question resolves as a bounded finalist fetch,
   not as a drop and not as a fetch per result. The verified
   badge is worth at most 4 points of the 100-point score
   (`channelScore` maximum 30, weighted 15 percent), and the
   official-artist badge was already dead in cycle 002: it
   never appeared in search data. Fetching the channel page
   for every result would cost one 1.9 MB page per result.
   Fetching it for the top three compliant finalists costs
   at most three requests per search, deduplicated by
   channel ID, and restores the verified input exactly.
   `FetchChannel` with `NoAbout: true` reads the header on
   the first page, which is where the badges live, so one
   request per channel is enough. A fetch that fails degrades
   to no badge, never to a failed search.
6. Download maps onto four ytb calls, in order
   (`cli/download.go` shows the composition; pango uses the
   `ytb` package only, never the `cli` package):
   `Client.StreamManifest(ctx, url)` resolves the streams,
   leading with the ANDROID_VR player; `SelectFormat(streams,
"bestaudio/best")` resolves the yt-dlp-style selector,
   best audio-only with a muxed fallback; `ResolveStreamURL`
   returns the plain googlevideo URL; `Client.DownloadToFile`
   fetches it in ranged 1 MiB chunks with resume and a
   `Refresh` callback that re-reads the manifest for a fresh
   URL on expiry. `DownloadToFile` leaves a partial file
   behind on failure, so pango keeps its current cleanup
   contract and removes the partial file itself.
7. The transcode stays in pango, not in `ytb.ExtractAudio`.
   `ExtractAudio` takes only a `-b:a` quality value, while
   yt-dlp passes `-q:a 0` (best VBR) for `--audio-quality 0`,
   and it sends ffmpeg output to stderr instead of capturing
   it into the error. Pango already shells out to ffmpeg in
   `sys/cmd/ffmpeg.go` with captured output, so the transcode
   joins that pattern with the same `-q:a 0` quality.
8. One shared `ytb.Client` is safe for concurrent use. The
   HTTP client is goroutine-safe; the pacing state is guarded
   by a mutex (`rateLimit`, `ytb/client.go`); the ytcfg
   harvest cache has its own mutex (`ytb/ytcfg.go`); the
   cache is nil unless a caller attaches one, and pango
   attaches none. Construction is cheap and does no I/O: the
   first `Call` harvests the InnerTube key once per host from
   a 40 KB embed page, and the harvest is cached for the life
   of the process. No search bootstraps a session, and pango
   attaches no session: tier 0, no cookies.
9. The default 1.5 second pacing (`DefaultDelay`,
   `ytb/config.go`) is global to the client, across every
   request the client makes, and it serializes the two
   search slots: two concurrent searches still place their
   HTTP requests at least 1.5 seconds apart. The two-slot
   cap (`maxYouTubeConcurrentSearches = 2`) is kept: it
   bounds goroutines, the pacer bounds requests. Media
   bytes are not paced: `DownloadToFile` issues its ranged
   GETs straight through the HTTP client, with no
   `rateLimit` call on that path.
10. No yt-dlp fallback path is reachable. Package `ytb`
    contains no yt-dlp execution: the only `os/exec` in the
    package is `ytb/ffmpeg.go`, and the transcript read is
    native (`ytb/transcript.go` notes only that the tool
    "used to shell out to yt-dlp"). The yt-dlp delegation
    (`--use-yt-dlp`, `extract`) lives in the `cli` package,
    which pango never imports. Pango must not import it.
11. The dependency weight is real and accepted. Importing
    package `ytb` pulls in modernc.org's SQLite driver through
    `ytb/store.go` alone, plus the `any-cli` kit (and its
    UI libraries) and `goquery`, although pango uses none of
    the store, none of the CLI, and none of the SQL. There
    is no smaller import path: the search, download, and
    store code share one package. The cost is binary size
    and build time, to be watched in CI.
12. The Go version must move. ytb-cli `go.mod` requires
    go 1.26.11; pango pins go 1.26.2, and the local toolchain
    is go 1.26.2. The `go` directive moves to 1.26.11, and
    the implementation confirms that v0.6.0 exists as the
    latest stable release when it runs `go get`.
13. The license is compatible. ytb-cli carries Apache-2.0
    (`LICENSE` in the reference clone). Apache-2.0 is
    compatible with pango GPL-3.0 as a dependency. Pango
    stays GPL-3.0.

**Assumed cause check**: The cause of the migration is not a
failure of yt-dlp: search and download through yt-dlp work
today. The cause is the owner's order to remove the external
binary dependency and the NO AI policy around yt-dlp
upstream, which forbids patching, forking, or filing issues
against it. The migration replaces the transport, not the
ranking and not the pipeline.

**Outcome**: Specify the replacement of both yt-dlp call
sites with in-process ytb calls, the bounded badge fetch
that keeps the verified input, the ffmpeg transcode in
pango, and the removal of yt-dlp from the Dockerfile, the
environment check, the test fakes, and the docs. The
provider keeps its query, its compliance gate, and its
score weights.

## User Scenarios and Testing

### User Story 1 - A synchronization searches through ytb (Priority: P1)

The owner runs a synchronization. The YouTube provider
returns matches through `Client.Search`, where yt-dlp
returned them before.

**Why this priority**: Search is the entry of the decide
stage. A broken search drops every track it touches.

**Independent Test**: Run the provider search against a
stubbed ytb HTTP transport serving canned InnerTube JSON,
and against the live service on the comparison tracks. Both
return matches.

**Acceptance Scenarios**:

1. **Given** a stubbed transport serving a compliant video
   row, **When** the provider searches, **Then** it returns
   a match for that video.
2. **Given** a live network, **When** the provider searches
   for the four captcha-failing tracks of cycle 002,
   **Then** it returns matches for each, and the top match
   of each is the same video yt-dlp ranked first.
3. **Given** a search with no results, **When** the provider
   searches, **Then** it returns zero matches and no error:
   the `NoResults` answer of `Client.Search` maps to an
   empty result, not to a provider error.

### User Story 2 - Ranking is preserved, badges included (Priority: P1)

The owner runs a synchronization. The matches keep the
compliance filter and the score, including the verified
channel part, restored through the bounded finalist fetch.

**Why this priority**: The score decides which video gets
downloaded. A silent ranking change would install worse
videos across the whole library.

**Independent Test**: Feed the provider canned search rows
plus canned channel pages, with one compliant and one
non-compliant result, and check which one is returned and
with what score.

**Acceptance Scenarios**:

1. **Given** canned results with one compliant and one
   non-compliant entry, **When** the provider searches,
   **Then** only the compliant entry is returned.
2. **Given** a verified channel on a finalist, **When** the
   provider scores the result, **Then** the channel part of
   the score is the verified weight, maximum 30.
3. **Given** a channel page fetch that fails, **When** the
   provider scores the finalist, **Then** the channel part
   is zero and the search still succeeds.
4. **Given** a lockup row with no description, **When** the
   provider scores it, **Then** the description score uses
   title plus owner only, and the result is not dropped for
   the missing description.

### User Story 3 - A synchronization downloads through ytb (Priority: P1)

The owner runs a synchronization. The chosen video downloads
through `DownloadToFile` and transcodes to mp3 through
ffmpeg, where yt-dlp downloaded and transcoded it before.

**Why this priority**: Download is the collect stage. A
broken download installs nothing.

**Independent Test**: Run the downloader against a stubbed
ytb transport serving a canned manifest and ranged bytes,
and against the live service for one track. Both write a
valid mp3.

**Acceptance Scenarios**:

1. **Given** a stubbed manifest and ranged bytes, **When**
   the downloader downloads, **Then** it writes the mp3 at
   the target path.
2. **Given** a download that fails halfway, **When** the
   failure returns, **Then** no partial file is left at the
   target path or at the raw temporary path.
3. **Given** a target path that already exists, **When** the
   downloader is called, **Then** it does not overwrite it:
   the dispatcher serves the cached file, as today.
4. **Given** an interrupted raw download, **When** the
   download runs again, **Then** it resumes from the raw
   temporary file instead of starting over.

### User Story 4 - yt-dlp is gone everywhere (Priority: P1)

The owner builds the image, runs the environment check, or
reads the docs. No yt-dlp reference remains as a
requirement, a binary, or a test fake.

**Why this priority**: A leftover reference is a leftover
dependency: the next install would fetch a binary the code
no longer calls, or a test would script a binary that no
longer exists.

**Independent Test**: Search the tree for `yt-dlp` outside
the completed spec directories, and run the Docker build and
the environment check with yt-dlp absent.

**Acceptance Scenarios**:

1. **Given** a tree with no yt-dlp reference outside
   `specs/`, **When** the implementation lands, **Then**
   `grep` finds none.
2. **Given** an image built from the new `Dockerfile`,
   **When** a synchronization runs in it, **Then** it
   searches and downloads with no yt-dlp installed.
3. **Given** a PATH without yt-dlp, **When**
   `ValidateEnvironment` runs, **Then** it succeeds with
   ffmpeg present.

### User Story 5 - Failures stay truthful (Priority: P2)

The owner runs a synchronization. A rate limit surfaces as
a provider error with its cause, and a failing download
surfaces the ytb or ffmpeg cause, the way the yt-dlp errors
surfaced before.

**Why this priority**: The owner debugs outages from the
log. A failure hidden behind a generic message sends the
next outage back to guessing.

**Independent Test**: Stub a transport that answers 429 and
one that answers 500, and check the provider result of
each.

**Acceptance Scenarios**:

1. **Given** a transport that answers 429 past the ytb
   retries, **When** the provider searches, **Then** it
   returns a provider error carrying the rate-limit cause.
2. **Given** a manifest with no downloadable streams,
   **When** the downloader downloads, **Then** it returns an
   error naming the video, not a silent miss.
3. **Given** an ffmpeg transcode that fails, **When** the
   download returns, **Then** the error carries the ffmpeg
   output.

## Requirements

### Functional Requirements

- **FR-001**: The YouTube provider searches through
  `ytb.Client.Search` with `SearchFilters{Type: "video"}`,
  `PageOptions{MaxPages: 1}`, and an `emit` that collects
  `Video` rows until ten are held, then returns `ErrStop`.
  Channel and Playlist rows are ignored. The query
  construction stays unchanged: track title plus artists,
  through `sanitizeYouTubeQuery`. The
  `maxYouTubeConcurrentSearches = 2` semaphore still guards
  the call, and one shared `ytb.Client` serves every search
  and every download in the process.
- **FR-002**: The search maps each `Video` row onto the
  existing result: `VideoID`, `Title`, `ChannelTitle` as
  owner, the first line of `Description`, `ViewCount` as
  views, `DurationSeconds` as length in seconds. Only the
  first line of the description is kept, as in cycle 002.
  A lockup row with no description keeps an empty
  description and still scores.
- **FR-003**: `compliant()` stays unchanged: the non-empty
  ID and the artist and song word containment on owner plus
  title. The year gate stays dropped.
- **FR-004**: `score()` stays unchanged, including the
  weights. `channelScore()` stays verified-only, maximum 30. The verified flag comes from the finalist fetch of
  FR-005, never from the search row, which carries none.
- **FR-005**: After scoring without badges, the provider
  takes the top three compliant results, deduplicates their
  channel IDs, and calls `Client.FetchChannel` with
  `ChannelOptions{NoAbout: true}` once per distinct channel
  ID. A verified channel adds the verified weight; an
  official-artist channel adds nothing. A channel fetch
  that fails leaves the channel part at zero and never fails
  the search. The results are re-sorted by the final score.
- **FR-006**: A `NoResults` answer from `Client.Search`
  means zero matches and no error, the same as a genuine
  empty result today. Any other search error is a provider
  error, wrapped as `youtube: <cause>`. No retry loop runs
  inside the provider: the ytb retries inside the client
  and the circuit breaker above it cover recovery.
- **FR-007**: The downloader resolves the video through
  `Client.StreamManifest`, selects with
  `SelectFormat(streams, "bestaudio/best")`, resolves the
  URL with `ResolveStreamURL`, and fetches it with
  `Client.DownloadToFile` into a raw temporary file beside
  the target, with `Total` from the stream, the default
  1 MiB chunk size, one worker, the stream user agent, a
  `Refresh` that re-reads the manifest and picks the same
  itag, and `Resume: true`. A stream with no plain URL is
  an error, not a fallback.
- **FR-008**: The raw download transcodes to the target
  extension through ffmpeg in `sys/cmd`, with the same
  `-q:a 0` quality yt-dlp used, and the raw temporary file
  is removed on success and on failure. A failed run leaves
  no partial file at the target path or at the raw path.
  The dispatcher still serves an existing target file from
  the cache before any download, so the no-overwrite
  behaviour is preserved.
- **FR-009**: `go.mod` adds `github.com/tamnd/ytb-cli`
  pinned to exactly v0.6.0, and the `go` directive moves to
  go 1.26.11, which ytb-cli requires. No yt-dlp fallback
  path is used: never `download --use-yt-dlp`, never
  `extract`, never a transcript fallback through yt-dlp.
  The `cli` package of ytb-cli is never imported.
- **FR-010**: yt-dlp leaves the `Dockerfile` (the image
  installs ffmpeg only), `ValidateEnvironment` (ffmpeg
  only), the test fakes (`sys/cmd`, `provider`,
  `downloader`, `cmd`), the live-test workflow (no yt-dlp
  install step, no `TEST_YTDLP` gate), and
  `docs/installation.md` (ffmpeg is the only runtime
  prerequisite). ffmpeg stays everywhere it is required,
  including loudness detect and normalize.
- **FR-011**: `provider.Search` keeps its sort by score and
  its error bubbling. The exported surface of the `provider`
  package stays compatible. Unit tests stub every network
  dependency through a fake HTTP transport on the shared
  client; a test never calls a live provider and never
  shells out to yt-dlp.

### Key Entities

- **YouTube result**: one video from the search, held in the
  `youTubeResult` struct, with ID, title, owner, description,
  views, length, and verified channel, the last filled by the
  finalist fetch.
- **Shared ytb client**: one `ytb.Client` per process, built
  from `ytb.DefaultConfig()`, with no session, no disk cache,
  and no store attached.
- **Download**: one raw stream file plus its mp3 transcode,
  held only until the transcode succeeds.

## Success Criteria

### Measurable Outcomes

- **SC-001**: Unit tests prove the search mapping, the
  compliance filter, the empty result, the finalist badge
  fetch and its failure, the download, the resume, and the
  partial-file cleanup, all against stubbed transports.
- **SC-002**: The provider-level search tests, the command
  synchronization tests, and the downloader tests pass with
  no yt-dlp binary anywhere on PATH.
- **SC-003**: A pre-merge comparison proves ytb search and
  download match yt-dlp on the four captcha-failing tracks
  of cycle 002: same top match per track, same score within
  the verified-weight tolerance, and a valid mp3 of matching
  duration.
- **SC-004**: The full test suite passes with the per-package
  coverage gate at 90 percent, locally and in CI, with the
  race detector on.
- **SC-005**: The first overnight synchronization on bee
  after the merge searches and downloads with no yt-dlp on
  the host, and its log shows no new failure class against
  the previous nights. Live verification of the full library
  happens after the merge, not before.
