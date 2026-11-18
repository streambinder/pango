# Feature Specification: YouTube Search through yt-dlp

**Feature Branch**: `feat/ytdlp-search`

**Created**: 2026-11-18

**Status**: Draft. The owner ordered this change on 2026-11-18,
after the investigation below, and ordered spec and implementation
in a single pull request as a single commit. The owner ratifies
this text at the review of the pull request.

**Input**: Report by the owner, 2026-11-18: the nightly
synchronization fails its YouTube searches, and the same failure
appears when the synchronization runs on his laptop.

## Hypothesis Investigation

**Hypothesis**: The YouTube search fails because the provider
scrapes the HTML search page, and YouTube captcha-blocks that
scrape.

**Evidence** (verified on 2026-11-18, on live sources):

1. On bee at 11:22 CET, with the error logging of
   [pango#282](https://github.com/streambinder/pango/pull/282),
   four searches failed with `youtube: blocked by google captcha`
   (Marianne, Saw You Stand, Play Your Games, Tear It Down),
   while two searches in the same run succeeded.
2. The same failure appears on the owner's laptop on hotel Wi-Fi
   the same day, so the block is not specific to the server.
3. Single searches succeed everywhere: one request from bee in
   the morning returned the results page, and bursts of four
   queries from this machine succeeded with both the Go user
   agent and a Chrome user agent (8 of 8). The trigger is the
   IP plus the burst, not the user agent and not the query.
4. yt-dlp search works where the scrape fails. With the binary
   at `~/workspace/tools/yt-dlp` (version 2026.08.19),
   `yt-dlp --dump-json --flat-playlist --no-download
--no-warnings "ytsearch5:Fontaines D.C. Marianne"` returns
   five JSON lines in about six seconds, no captcha. The first
   result is the official video, duration 223 seconds.
5. The variant that selects the Android player client also returns correct results, but whether it changes the search client as opposed to the playback client is unproven, so the plain command is the one this cycle adopts.
6. The ranking inputs survive the switch. The flat JSON carries the video identifier, `title`, `uploader`, `description`, `duration` in seconds, `view_count` as an integer, and `channel_is_verified`. The duration and the view count arrive as native integers, where the scraper parsed them out of strings.
7. The official-artist badge does not appear in search data at
   all: 0 occurrences in the whole search page across three
   queries (Fontaines D.C. Marianne, Taylor Swift Anti-Hero,
   The Strokes Last Nite), even though the official videos are
   present. The `officialArtistChannel` check in the scraper
   was effectively dead code, so dropping it loses nothing in
   practice. The official videos instead carry `AUDIO_BADGE`,
   which is the candidate marker if a future change wants a
   channel-credibility signal from search data.
8. The release year does not survive the switch. The flat JSON
   carries `release_year`, but it was empty on the official
   video. Full extraction returns `upload_date`, but it costs
   about fourteen seconds per single result, which is too slow
   for the synchronization path. The old code already passed
   the year gate when the date was unknown, so dropping the
   gate matches the existing behavior for unknown dates.
9. The description changes source, and that matters. The
   scraper read the snippet, the opening lines of the
   description; yt-dlp returns the full description. A live
   before/after comparison on the four captcha-failing tracks
   showed the difference: with the full description, the
   official Marianne video falls from first to third, because
   its credits contain the word "studio", which the misleading
   check penalizes. The other three tracks keep the same top
   pick under both implementations. The fix is to keep only
   the first line of the description, which approximates the
   snippet the scraper saw.

**Assumed cause check**: The cause is not the query and not the
user agent: both were ruled out by the burst test. The cause is
not a YouTube outage: single searches and yt-dlp searches
succeed at the same time. The cause is the scrape itself: an
unauthenticated HTML page fetch, fired in bursts, which
YouTube's bot detection answers with a captcha redirect.

**Outcome**: Specify a smaller change than a new search backend
would imply. The provider keeps its query, its compliance gate
minus the year, and its score minus the official-artist part,
and it replaces only the transport: a yt-dlp subprocess that
returns JSON lines.

## User Scenarios and Testing

### User Story 1 - A synchronization searches where the scrape is blocked (Priority: P1)

The owner runs a synchronization. The YouTube provider returns
matches through yt-dlp, where the scrape would have been
captcha-blocked.

**Why this priority**: Search is the entry of the decide stage.
A blocked search drops every track it touches and trips the
circuit breaker for the rest of the run.

**Independent Test**: Run the provider search against a stubbed
yt-dlp binary that prints canned JSON lines, and against the
real binary on live queries. Both return matches.

**Acceptance Scenarios**:

1. **Given** a stubbed yt-dlp that prints a compliant JSON
   line, **When** the provider searches, **Then** it returns a
   match for that video.
2. **Given** a live network, **When** the provider searches for
   the four tracks that failed with the captcha, **Then** it
   returns matches for each, and the top match of each is the
   same video the old scraper ranked first: the official video
   for three of them, and the behind-the-scenes video for Saw
   You Stand, which the old scraper also ranked first.

### User Story 2 - Ranking is preserved (Priority: P1)

The owner runs a synchronization. The matches the provider
returns keep the compliance filter and the score, apart from
the two documented drops: the official-artist badge and the
year gate.

**Why this priority**: The score decides which video gets
downloaded. A silent ranking change would install worse
videos across the whole library.

**Independent Test**: Feed the provider canned JSON lines with
one compliant and one non-compliant result, and check which
one is returned and with what score.

**Acceptance Scenarios**:

1. **Given** canned results with one compliant and one
   non-compliant entry, **When** the provider searches,
   **Then** only the compliant entry is returned.
2. **Given** a verified channel, **When** the provider scores
   the result, **Then** the channel part of the score is the
   verified weight only.

### User Story 3 - Search failures stay truthful (Priority: P2)

The owner runs a synchronization. An empty search reports the
track as not found, and a failing yt-dlp reports the provider
error, which the error logging of cycle 282 bubbles up.

**Why this priority**: The owner debugged this outage from the
log. A search that hides its cause behind a generic message
sends the next outage back to guessing.

**Independent Test**: Stub a yt-dlp that prints nothing, one
that exits non-zero, and a missing binary, and check the
provider result of each.

**Acceptance Scenarios**:

1. **Given** yt-dlp prints no lines, **When** the provider
   searches, **Then** it returns zero matches and no error.
2. **Given** yt-dlp exits non-zero, **When** the provider
   searches, **Then** it returns an error carrying the yt-dlp
   output.
3. **Given** the yt-dlp binary is missing, **When** the
   provider searches, **Then** it returns a clear error naming
   the missing binary.

## Requirements

### Functional Requirements

- **FR-001**: The YouTube provider searches through
  `sys/cmd.YouTubeDlSearch`, which runs `yt-dlp --dump-json
--flat-playlist --no-download --no-warnings
"ytsearch10:<query>"` as a subprocess, with an argument
  slice, never a shell string.
- **FR-002**: The query construction stays unchanged: track
  title plus artists, through `sanitizeYouTubeQuery`. The
  `maxYouTubeConcurrentSearches = 2` semaphore still guards
  the call.
- **FR-003**: The flat JSON fields map onto the existing
  result: `id`, `title`, `uploader` as owner, the first line
  of `description`, `view_count` as views, `duration` as
  length in seconds, `channel_is_verified` as the verified
  channel. Only the first line of the description is kept,
  because the scraper read the snippet, and the full
  description carries credits where misleading words like
  "studio" routinely appear.
- **FR-004**: `compliant()` keeps the non-empty ID and the
  artist and song word containment on owner plus title. The
  year gate is dropped.
- **FR-005**: `score()` stays unchanged, except
  `channelScore()` which becomes verified-only, maximum 30.
- **FR-006**: Empty JSON lines output means zero matches, the
  same as a genuine empty result today. A non-zero yt-dlp
  exit, or a missing binary, is a provider error.
- **FR-007**: No retry loop runs inside the provider. The
  circuit breaker and the nightly runs cover recovery.
- **FR-008**: `provider.Search` keeps its sort by score and
  its error bubbling. No new dependency: yt-dlp is already a
  documented runtime dependency.
- **FR-009**: The download path, the lyrics, the breaker
  threshold, and the sync command stay unchanged.

### Key Entities

- **YouTube result**: one video from the search, held in the
  `youTubeResult` struct, with ID, title, owner, description,
  views, length, and verified channel.

## Success Criteria

### Measurable Outcomes

- **SC-001**: Unit tests prove the JSON lines parsing, the
  compliant filter, the empty output, the non-zero exit, and
  the missing binary.
- **SC-002**: The provider-level search tests and the command
  synchronization tests pass.
- **SC-003**: A live run proves the search returns matches for
  the four captcha-failing tracks, and the top match of each is
  the same video the old scraper ranked first.
- **SC-004**: The full test suite passes with the per-package
  coverage gate at 90 percent, locally and in CI.
