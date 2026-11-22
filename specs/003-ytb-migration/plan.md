# Implementation Plan: YouTube Search and Download through ytb-cli

**Branch**: `feat/ytb-migration` | **Date**: 2026-11-22 |
**Spec**: `specs/003-ytb-migration/spec.md`

**Input**: Feature specification from
`/specs/003-ytb-migration/spec.md`

## Summary

The YouTube provider stops shelling out to yt-dlp and calls
the ytb-cli library in process, through one shared
`ytb.Client` built from `DefaultConfig()`. `search()` builds
the query exactly as today, acquires the existing two-slot
semaphore, and calls `Client.Search` with a video-only
filter, one page, and an emit that collects ten `Video` rows.
Each row maps onto the existing `youTubeResult` struct, with
the view count now rounded and the description still kept to
its first line. The verified badge, which no search row
carries, comes from a bounded finalist fetch: the top three
compliant results, one `FetchChannel` with `NoAbout: true`
per distinct channel ID, verified-only, degrading to zero on
failure. The downloader resolves the video through
`Client.StreamManifest`, selects `bestaudio/best`, fetches
the stream with `DownloadToFile` into a raw temporary file,
and transcodes it to mp3 through ffmpeg in `sys/cmd`, with
the same `-q:a 0` quality. yt-dlp leaves the Dockerfile, the
environment check, the test fakes, the live-test workflow,
and the docs. ffmpeg stays everywhere.

## Technical Context

**Language/Version**: Go, version from `go.mod` (moves to
1.26.11, which ytb-cli requires)

**Primary Dependencies**: `github.com/tamnd/ytb-cli` v0.6.0
exactly (Apache-2.0); ffmpeg stays as the only runtime
binary prerequisite

**Storage**: no new storage; the search has no cache, the
ytb disk cache and store stay unattached, the raw download
is a temporary file removed after the transcode

**Testing**: fake HTTP transport tests for the provider, the
downloader, and the command harness; a live comparison
against yt-dlp on the four cycle 002 tracks before the
merge; the overnight run on bee after the merge

**Constraints**: per-package coverage at 90 percent; unit
tests stub every network dependency and never shell out to
yt-dlp; the exported surface of the `provider` package stays
compatible; no live verification of the full library before
the merge

## Constitution Check

- Honest Reporting: an empty search still means not found,
  and a `NoResults` answer maps to zero matches, not to an
  error. A failing channel fetch degrades the channel score
  to zero instead of failing the search, and the degradation
  stays visible: the score is what it is without the badge.
  A missing description on a lockup row is an empty
  description, never a dropped result.
- Graceful Enrichment: a failing search still drops only the
  track, never the synchronization. The badge fetch is an
  enrichment of the score, and its failure degrades the
  score, never the search.
- Library Surface: `provider.Search` keeps its signature.
  The ranking change is the one documented input: the
  verified badge now arrives from a channel read instead of
  from flat JSON, at the same weight.
- Minimal Owned Surface: the transport moves to a
  specialized library. The transcode stays in pango because
  the library transcode cannot express the `-q:a 0` quality
  and does not capture ffmpeg output into the error.
- Specification Cycle: this document and the task list
  complete the plan and tasks steps of cycle 003. Spec is a
  draft until the owner ratifies it. Implementation follows
  in a later cycle.

## Decisions

- **One shared client, default pacing**: one `ytb.Client`
  per process, from `DefaultConfig()`, serves search and
  download. The 1.5 second pacing is global to the client
  and serializes the two search slots; the slots still bound
  goroutines. No session is attached (tier 0), no disk cache
  and no store is attached: pango needs neither, and the
  store would create SQLite files for a crawl pango never
  runs. Media bytes are not paced, so downloads run at line
  speed.
- **Video-only filter, ten rows, one page**: `Type: "video"`
  matches `ytsearch10`. `MaxPages: 1` bounds continuations.
  Ten `Video` rows are collected through `emit`, then
  `ErrStop` halts the stream. `Max` is not used for the
  count, because it counts Channel and Playlist rows too,
  and a channel card first would steal a video slot.
- **Badges from the top three finalists, verified-only**:
  the search row carries no badge, so the provider scores
  without badges, takes the top three compliant results,
  and fetches each distinct channel once with
  `FetchChannel` and `NoAbout: true`. The header on the
  first page carries the badges, so one request per channel
  is enough. Verified adds its 30 points; official-artist
  adds nothing, as cycle 002 decided. A failed fetch leaves
  the channel part at zero. Rejected: dropping verified
  (it would lose the only channel-credibility signal for a
  saving of seconds on a nightly job), and fetching every
  result (one 1.9 MB page per result for a 4-point input).
- **Description kept to its first line, lockup rows kept**:
  `descriptionSnippet` is the snippet the old scraper read,
  so the cycle 002 rule stands. A lockup row has no
  description; it scores from title plus owner only. No
  per-video `FetchVideo` fills it: that would return the
  full description and reintroduce the credits penalty, at
  one watch-page fetch per video.
- **Rounded views accepted**: the views score counts
  digits, so a rounded count moves it only at a digit
  boundary. No correction is attempted.
- **Year stays dropped**: no search row carries a publish
  year, in either backend. The cycle 002 decision stands.
- **No retry loop in the provider**: the ytb client retries
  transient 429 and 5xx responses itself (three retries
  with backoff), and the circuit breaker above the provider
  covers recovery, as it did for the scrape and for yt-dlp.
- **`NoResults` is an empty result**: `Client.Search`
  returns a coded no-results error when the first page is
  empty. The provider translates it to zero matches and no
  error. The exact detector comes from the `kit/errs`
  package of the fetched module; the implementation confirms
  it from the module source, never from a string match on
  the message.
- **Download is manifest, select, fetch, transcode**: the
  composition follows the ytb-cli native download path:
  `StreamManifest` (ANDROID_VR first), `SelectFormat` with
  `"bestaudio/best"`, `ResolveStreamURL`, `DownloadToFile`
  with resume and a manifest re-read as `Refresh`. One
  worker keeps the download sequential; the writer is
  in-order either way, so the part-file size stays the
  resume offset. A stream with no plain URL is an error.
- **CDN refusal falls back to another client**: the
  merged manifest carries only ANDROID_VR-minted URLs (its
  merge keeps one stream per itag, the ANDROID_VR one on a
  tie), so when the CDN refuses one, re-reading the manifest
  hands back another refused URL. On the refusal error the
  downloader retries from another client, restarting from
  byte 0: the part file holds bytes of the refused stream, so
  resuming into it would corrupt the file. VisionOS goes
  first: its URLs need no proof-of-origin token, so the CDN
  serves them where the mobile clients' URLs are refused
  past the first chunk. Android follows for the videos
  VisionOS cannot serve, such as made-for-kids. Each stage's
  own `Refresh` re-mints from the same client, never the
  manifest. ANDROID_VR stays first: ytb itself prefers it
  for the download path, and the refusal varies by video,
  mint, and egress, so the primary is only abandoned when it
  actually fails. The refusal is detected by the message text
  of ytb's unexported stale-URL error, the only signal the
  library exposes; a ytb upgrade that rewords it shows up as
  a fallback that never fires, not as a crash.
- **Transcode in pango, not in ytb**: `ytb.ExtractAudio`
  cannot express `-q:a 0` (its quality flag maps to `-b:a`)
  and sends ffmpeg output to stderr instead of capturing it
  into the error. A new ffmpeg call in `sys/cmd` keeps the
  `-q:a 0` quality and the captured-output error contract
  of `LoudnessDetect` and `LoudnessNormalize`.
- **No partial file left behind**: `DownloadToFile` does not
  clean up after itself. Pango removes the raw temporary
  file and the target file on any failure, the way the
  yt-dlp wrapper removed the stem file. The dispatcher
  still serves an existing target from the cache first, so
  the no-overwrite behaviour is preserved at that layer.
- **Dependency weight accepted**: importing package `ytb`
  pulls in modernc.org's SQLite driver (through `store.go` alone)
  and the `any-cli` kit, which pango never uses. There is no
  smaller import path: search, download, and store share
  one package. The cost is binary size and build time, to be
  watched in CI. The `cli` package, which holds the yt-dlp
  delegation, is never imported.
- **Go directive moves to 1.26.11**: ytb-cli requires it.
  The implementation runs `go get
github.com/tamnd/ytb-cli@v0.6.0`, confirms v0.6.0 is the
  latest stable release, and records the resulting
  `go.sum` entries.
- **Test seam is the HTTP transport**: unit tests replace
  the shared client's transport (`Client.HTTP().Transport`)
  with a stub that serves canned InnerTube JSON, the embed
  harvest page, channel pages, and ranged googlevideo bytes,
  and they build the client with `Delay: 0` so the pacer
  does not slow the suite. The command harness scripts the
  search through that transport instead of through control
  files for a fake binary.
- **Live test renamed and re-gated**: the nightly workflow
  no longer installs yt-dlp and no longer sets `TEST_YTDLP`.
  The live search test runs against the real service with no
  binary prerequisite. The full-library verification is the
  first overnight run on bee after the merge, not a
  pre-merge gate.

## Project Structure

### Documentation (this feature)

```text
specs/003-ytb-migration/
├── spec.md
├── plan.md
└── tasks.md
```

### Source Code (repository root)

```text
go.mod                         # ytb-cli v0.6.0, go 1.26.11
go.sum                         # ytb-cli entries
provider/youtube.go            # search through Client.Search, finalist badge fetch
provider/youtube_test.go       # stubbed-transport tests, live test
provider/provider_test.go      # Search-level tests on the stubbed transport
provider/youtube_client.go     # shared ytb.Client construction (new, if needed)
sys/cmd/youtubedl.go           # deleted: both yt-dlp commands leave
sys/cmd/youtubedl_test.go      # deleted
sys/cmd/youtube.go             # ffmpeg transcode for the download (new)
sys/cmd/environment.go         # ffmpeg only
sys/cmd/fakebin_test.go        # yt-dlp name leaves
downloader/youtubedl.go        # deleted, replaced by a ytb downloader
downloader/downloader_test.go  # stubbed-transport tests, fake yt-dlp leaves
cmd/harness_test.go            # fake yt-dlp leaves, search scripted by transport
docs/installation.md           # ffmpeg is the only runtime prerequisite
Dockerfile                     # ffmpeg only
.github/workflows/scrape.yml   # live test name, no yt-dlp install step
```
