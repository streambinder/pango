package cmd

import (
	"bytes"
	"errors"
	"fmt"
	"image"
	"image/jpeg"
	"io"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/tamnd/ytb-cli/ytb"

	"github.com/streambinder/pango/entity"
	"github.com/streambinder/pango/spotify"
	"github.com/streambinder/pango/sys"
	"github.com/stretchr/testify/assert"
)

// This file holds the shared fakes every command test runs against.
// Every network boundary of the application bottoms out in
// http.DefaultTransport (the Spotify API and OAuth2 endpoints through the
// spotify client, genius and lrclib through http.DefaultClient, blob
// downloads through a client with a nil transport), and every external
// tool is invoked by bare name (ffmpeg, the platform opener). The
// YouTube boundary goes through the shared ytb client instead, so
// installWeb points that client at the same scripted transport.
// Command tests therefore run the real code end to end, scripting only
// transports, PATH binaries and the filesystem.

const (
	tokenJSON = `{"access_token":"access","token_type":"Bearer",` +
		`"refresh_token":"refresh","expires_in":3600}`
	meJSON = `{"id":"alice","display_name":"Alice"}`
)

// trackJSON mirrors a full Spotify track payload for the given id.
// The canonical fixture track (id 123) is titled "Title"; any other id
// gets a distinct title, so multi-track scenarios do not collide on the
// final filename.
func trackJSON(id string) string {
	title := "Title"
	if id != "123" {
		title = "Title " + id
	}
	return fmt.Sprintf(`{"id":%q,"name":%q,"artists":[{"name":"Artist"}],`+
		`"duration_ms":180000,"track_number":1,`+
		`"album":{"name":"Album","release_date":"1970","images":[{"url":"http://ima.ge"}]}}`, id, title)
}

// roundTripFunc adapts a function to http.RoundTripper.
type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(request *http.Request) (*http.Response, error) {
	return f(request)
}

// installTransport swaps http.DefaultTransport for the given handler for
// the duration of the test.
func installTransport(t *testing.T, transport http.RoundTripper) {
	t.Helper()
	previous := http.DefaultTransport
	http.DefaultTransport = transport
	t.Cleanup(func() { http.DefaultTransport = previous })
}

func jsonResponse(status int, body string) *http.Response {
	return &http.Response{
		Status:        fmt.Sprintf("%d %s", status, http.StatusText(status)),
		StatusCode:    status,
		Proto:         "HTTP/1.1",
		ProtoMajor:    1,
		ProtoMinor:    1,
		Header:        http.Header{"Content-Type": []string{"application/json"}},
		Body:          io.NopCloser(strings.NewReader(body)),
		ContentLength: int64(len(body)),
	}
}

func apiError(status int, message string) (int, string) {
	return status, fmt.Sprintf(`{"error":{"status":%d,"message":%q}}`, status, message)
}

// spotifyRouter serves the Spotify Web API endpoints the commands use.
// Individual paths can be overridden per test through the overrides map,
// keyed by path prefix; everything else gets the standard fixtures.
// Longer prefixes win, so /v1/me/tracks beats /v1/me.
type spotifyRouter struct {
	overrides map[string]func(*http.Request) (int, string)
	prefixes  []string
}

func newSpotifyRouter(overrides map[string]func(*http.Request) (int, string)) spotifyRouter {
	prefixes := make([]string, 0, len(overrides))
	for prefix := range overrides {
		prefixes = append(prefixes, prefix)
	}
	sort.Slice(prefixes, func(i, j int) bool { return len(prefixes[i]) > len(prefixes[j]) })
	return spotifyRouter{overrides: overrides, prefixes: prefixes}
}

func (router spotifyRouter) RoundTrip(request *http.Request) (*http.Response, error) {
	for _, prefix := range router.prefixes {
		if strings.HasPrefix(request.URL.Path, prefix) {
			status, body := router.overrides[prefix](request)
			return jsonResponse(status, body), nil
		}
	}
	switch {
	case request.URL.Host == "accounts.spotify.com":
		return jsonResponse(200, tokenJSON), nil
	case request.URL.Path == "/v1/me":
		return jsonResponse(200, meJSON), nil
	case request.URL.Path == "/v1/me/tracks":
		if request.URL.Query().Get("offset") != "" {
			return jsonResponse(200, `{"items":[],"next":null,"total":1}`), nil
		}
		return jsonResponse(200, `{"items":[{"added_at":"2026-01-01T00:00:00Z","track":`+
			trackJSON("123")+`}],"next":null,"total":1}`), nil
	case strings.HasPrefix(request.URL.Path, "/v1/tracks/"):
		return jsonResponse(200, trackJSON(strings.TrimPrefix(request.URL.Path, "/v1/tracks/"))), nil
	case strings.HasPrefix(request.URL.Path, "/v1/albums/"):
		id := strings.TrimPrefix(request.URL.Path, "/v1/albums/")
		return jsonResponse(200, fmt.Sprintf(`{"id":%q,"name":"Album","artists":[{"name":"Artist"}],`+
			`"release_date":"1970","tracks":{"items":[%s],"next":null,"total":1}}`,
			id, simpleTrackJSON("123"))), nil
	case request.URL.Path == "/v1/me/playlists":
		return jsonResponse(200, `{"items":[{"id":"playlist123","name":"Mix"}],`+
			`"next":null,"total":1}`), nil
	case strings.HasPrefix(request.URL.Path, "/v1/playlists/"):
		id := strings.Trim(strings.TrimPrefix(request.URL.Path, "/v1/playlists/"), "/")
		if strings.HasSuffix(request.URL.Path, "/tracks") {
			return jsonResponse(200, `{"items":[{"track":`+trackJSON("123")+`}],"next":null,"total":1}`), nil
		}
		return jsonResponse(200, fmt.Sprintf(`{"id":%q,"name":"Mix",`+
			`"tracks":{"items":[{"track":%s}],"next":null,"total":1}}`, id, trackJSON("123"))), nil
	case request.URL.Path == "/v1/search":
		return jsonResponse(200, `{"tracks":{"items":[`+trackJSON("123")+`],"next":null,"total":1}}`), nil
	}
	return jsonResponse(404, `{"error":{"status":404,"message":"unknown"}}`), nil
}

func simpleTrackJSON(id string) string {
	return fmt.Sprintf(`{"id":%q,"name":"Title","artists":[{"name":"Artist"}],`+
		`"duration_ms":180000,"track_number":1}`, id)
}

// installSpotify installs the Spotify API router as the default transport.
func installSpotify(t *testing.T, overrides map[string]func(*http.Request) (int, string)) {
	t.Helper()
	installTransport(t, newSpotifyRouter(overrides))
}

// rawResponse builds a response with an explicit content type and body.
func rawResponse(status int, contentType string, body []byte) *http.Response {
	return &http.Response{
		Status:        fmt.Sprintf("%d %s", status, http.StatusText(status)),
		StatusCode:    status,
		Proto:         "HTTP/1.1",
		ProtoMajor:    1,
		ProtoMinor:    1,
		Header:        http.Header{"Content-Type": []string{contentType}},
		Body:          io.NopCloser(strings.NewReader(string(body))),
		ContentLength: int64(len(body)),
	}
}

// webScript scripts the non-Spotify web boundaries: the lyrics providers,
// artwork downloads, and the YouTube search and download. providerErr and
// noResults translate into the search transport behaviour, and
// downloadErr into the player transport behaviour.
type webScript struct {
	lyricsErr       bool // lrclib and genius both fail
	noLyrics        bool // lrclib answers with empty lyrics
	lyricsLeadBlank bool // lrclib answers lyrics with leading blank lines
	artworkErr      bool // the artwork URL answers 404
	providerErr     bool // the search provider fails
	noResults       bool // providers answer successfully but find nothing
	downloadErr     bool // the download manifest carries no streams
}

// installWeb installs the Spotify router plus the scripted web fixtures
// as the default transport, and points the shared ytb client at the
// same transport with no pacing and no retries.
func installWeb(t *testing.T, overrides map[string]func(*http.Request) (int, string), web webScript) {
	t.Helper()
	router := newSpotifyRouter(overrides)
	transport := roundTripFunc(func(request *http.Request) (*http.Response, error) {
		switch request.URL.Host {
		case "accounts.spotify.com", "api.spotify.com":
			return router.RoundTrip(request)
		case "lrclib.net":
			return lrclibResponse(web)
		case "api.genius.com":
			return geniusResponse(web), nil
		case "ima.ge":
			// the artwork blob download
			if web.artworkErr {
				return rawResponse(404, "text/plain", []byte("not found")), nil
			}
			return rawResponse(200, "image/jpeg", jpegBytes(t)), nil
		case "www.youtube.com":
			return youTubeResponse(web, request), nil
		case "googlevideo.example":
			return youTubeRangeResponse(request), nil
		default:
			// any other host is unreachable: in particular, blob
			// support probes (HEAD on a video page URL) must fail, so
			// downloads fall through to the downloader owning the URL
			return rawResponse(404, "text/plain", []byte("not found")), nil
		}
	})
	installTransport(t, transport)
	client := ytb.NewClient(ytb.Config{
		Workers: 1,
		Delay:   0,
		Timeout: 30 * time.Second,
		Retries: 0,
		HL:      "en",
		GL:      "US",
	})
	client.HTTP().Transport = transport
	previous := sys.YouTubeClient
	sys.YouTubeClient = client
	t.Cleanup(func() { sys.YouTubeClient = previous })
}

// youTubeAudioBytes is the canned stream the stub googlevideo host serves.
const youTubeAudioBytes = "0123456789"

// youTubeSearchJSON is a canned InnerTube search response with one
// video row matching the fixture track.
const youTubeSearchJSON = `{"contents":{"twoColumnSearchResultsRenderer":` +
	`{"primaryContents":{"sectionListRenderer":{"contents":` +
	`[{"itemSectionRenderer":{"contents":[{"videoRenderer":{` +
	`"videoId":"abc123","title":{"simpleText":"Title"},` +
	`"descriptionSnippet":{"simpleText":"some description"},` +
	`"lengthText":{"simpleText":"3:00"},` +
	`"publishedTimeText":{"simpleText":"1 month ago"},` +
	`"ownerText":{"runs":[{"text":"Artist","navigationEndpoint":` +
	`{"browseEndpoint":{"browseId":"UCartist"}}}]},` +
	`"viewCountText":{"simpleText":"1,000,000 views"}}}]}}]}}}}}`

// youTubeChannelPage is a canned channel page for the fixture
// channel, carrying the verified badge.
const youTubeChannelPage = `var ytInitialData = {"metadata":` +
	`{"channelMetadataRenderer":{"externalId":"UCartist","title":"Artist"}},` +
	`"header":{"pageHeaderRenderer":{"content":{"pageHeaderViewModel":` +
	`{"title":{"dynamicTextViewModel":{"text":{"attachmentRuns":` +
	`[{"element":{"type":{"imageType":{"image":{"sources":` +
	`[{"clientResource":{"imageName":"CHECK_CIRCLE_FILLED"}}]}}}}}]}}}}}}}};`

// youTubePlayerJSON is a canned player response carrying one
// audio-only stream of ten bytes.
const youTubePlayerJSON = `{"videoDetails":{"title":"Title","author":"Artist",` +
	`"lengthSeconds":"180"},"playabilityStatus":{"status":"OK"},` +
	`"streamingData":{"adaptiveFormats":[{"itag":140,` +
	`"mimeType":"audio/mp4; codecs=\"mp4a.40.2\"","bitrate":128000,` +
	`"audioQuality":"AUDIO_QUALITY_MEDIUM","contentLength":"10",` +
	`"url":"https://googlevideo.example/audio","audioChannels":2,` +
	`"audioSampleRate":"44100"}]}}`

// youTubeResponse scripts the YouTube HTTP boundary the shared ytb
// client talks to: the embed harvest page, the InnerTube search and
// player endpoints, and channel pages.
func youTubeResponse(web webScript, request *http.Request) *http.Response {
	switch {
	case request.Method == http.MethodGet && strings.HasPrefix(request.URL.Path, "/embed/"):
		return rawResponse(200, "text/html", []byte(`"INNERTUBE_API_KEY":"test-key",`+
			`"INNERTUBE_CLIENT_VERSION":"2.0","VISITOR_DATA":"test-visitor"`))
	case request.Method == http.MethodPost && request.URL.Path == "/youtubei/v1/search":
		if web.providerErr {
			return jsonResponse(429, `{}`)
		}
		if web.noResults {
			return jsonResponse(200, `{}`)
		}
		return jsonResponse(200, youTubeSearchJSON)
	case request.Method == http.MethodGet && strings.HasPrefix(request.URL.Path, "/channel/"):
		return rawResponse(200, "text/html", []byte(youTubeChannelPage))
	case request.Method == http.MethodPost && request.URL.Path == "/youtubei/v1/player":
		if web.downloadErr {
			return jsonResponse(200, `{"videoDetails":{},`+
				`"playabilityStatus":{"status":"OK"},"streamingData":{}}`)
		}
		return jsonResponse(200, youTubePlayerJSON)
	}
	return jsonResponse(404, `{}`)
}

// youTubeRangeResponse answers a ranged GET out of the canned audio bytes.
func youTubeRangeResponse(request *http.Request) *http.Response {
	start, end := 0, len(youTubeAudioBytes)-1
	if span := request.Header.Get("Range"); span != "" {
		// a malformed Range falls back to the full byte range
		var rangeStart, rangeEnd int
		if _, err := fmt.Sscanf(span, "bytes=%d-%d", &rangeStart, &rangeEnd); err == nil {
			start, end = rangeStart, rangeEnd
		}
	}
	body := youTubeAudioBytes[start : end+1]
	response := rawResponse(206, "audio/mp4", []byte(body))
	response.Header.Set("Content-Range",
		fmt.Sprintf("bytes %d-%d/%d", start, end, len(youTubeAudioBytes)))
	return response
}

func lrclibResponse(web webScript) (*http.Response, error) {
	if web.lyricsErr {
		return nil, errors.New("ko")
	}
	if web.noLyrics {
		return jsonResponse(200, `{"plainLyrics":"","syncedLyrics":""}`), nil
	}
	if web.lyricsLeadBlank {
		return jsonResponse(200, "{\"plainLyrics\":\"\\n\\nsome lyrics\",\"syncedLyrics\":\"\"}"), nil
	}
	return jsonResponse(200, `{"plainLyrics":"some lyrics","syncedLyrics":""}`), nil
}

func geniusResponse(web webScript) *http.Response {
	if web.lyricsErr {
		return jsonResponse(500, `{}`)
	}
	return jsonResponse(200, `{"response":{"hits":[]}}`)
}

// jpegBytes encodes a real 1x1 JPEG: the artwork pipeline decodes the
// downloaded picture, so placeholder bytes would not pass.
func jpegBytes(t *testing.T) []byte {
	t.Helper()
	var buffer bytes.Buffer
	img := image.NewRGBA(image.Rect(0, 0, 1, 1))
	assert.Nil(t, jpeg.Encode(&buffer, img, nil))
	return buffer.Bytes()
}

// seedSession writes a valid, non-expired OAuth2 token to the session
// file the spotify package reads at startup, and sets the credentials the
// authenticator is built from, so spotify.Authenticate recovers the
// session without any browser flow.
func seedSession(t *testing.T) {
	t.Helper()
	t.Setenv("SPOTIFY_ID", "test-id")
	t.Setenv("SPOTIFY_KEY", "test-secret")

	path := sys.CacheFile(spotify.TokenBasename)
	assert.Nil(t, os.MkdirAll(filepath.Dir(path), 0o755))
	token := `{"access_token":"access","token_type":"Bearer",` +
		`"refresh_token":"refresh","expiry":"2999-01-01T00:00:00Z"}`
	assert.Nil(t, os.WriteFile(path, []byte(token), 0o600))
	t.Cleanup(func() { _ = os.Remove(path) })
}

// noSession makes sure no recoverable session exists and credentials are
// set, so Authenticate falls through to the interactive browser flow.
func noSession(t *testing.T) {
	t.Helper()
	t.Setenv("SPOTIFY_ID", "test-id")
	t.Setenv("SPOTIFY_KEY", "test-secret")
	path := sys.CacheFile(spotify.TokenBasename)
	_ = os.Remove(path)
	t.Cleanup(func() { _ = os.Remove(path) })
}

// fakeBinDir installs fake ffmpeg and opener binaries on PATH and
// returns their directory. A binary's behavior is driven by control files
// in the same directory: creating "<name>.fail" makes it fail, and the
// opener appends its arguments to "open.log". The YouTube search and
// download are scripted through the transport in installWeb instead:
// the provider and the downloader talk to the shared ytb client, not
// to a binary.
func fakeBinDir(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()

	ffmpeg := `#!/bin/sh
if [ -f "` + dir + `/ffmpeg.fail" ]; then echo "ffmpeg blew up" >&2; exit 1; fi
last=""
for arg in "$@"; do last="$arg"; done
case "$*" in
*measured_I*)
  if [ -f "` + dir + `/ffmpeg.normfail" ]; then echo "normalize blew up" >&2; exit 1; fi
  ;;
*print_format=json*)
  if [ -f "` + dir + `/ffmpeg.badjson" ]; then
    echo "no loudness summary here" >&2
  elif [ -f "` + dir + `/ffmpeg.far" ]; then
    printf '%s\n' '{"input_i":"-30.00","input_tp":"-1.50","input_lra":"11.00","input_thresh":"-25.00","target_offset":"16.00"}' >&2
  else
    printf '%s\n' '{"input_i":"-14.00","input_tp":"-1.50","input_lra":"11.00","input_thresh":"-25.00","target_offset":"0.00"}' >&2
  fi
  ;;
*-q:a*)
  # a transcode produces the target file: leave it empty, so the
  # downstream tag writer opens a clean file, the way the old
  # download fake left it
  : > "$last"
  exit 0
  ;;
esac
case "$last" in
null|"") ;;
*) printf 'audio' > "$last" ;;
esac
exit 0
`
	opener := `#!/bin/sh
printf '%s\n' "$*" >> "` + dir + `/open.log"
if [ -f "` + dir + `/open.fail" ]; then exit 1; fi
exit 0
`
	for name, script := range map[string]string{
		"ffmpeg":   ffmpeg,
		"xdg-open": opener, "open": opener, "rundll32": opener,
	} {
		assert.Nil(t, os.WriteFile(filepath.Join(dir, name), []byte(script), 0o755))
	}
	t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))
	return dir
}

// noOpenerPath restricts PATH to an empty directory, so the platform
// opener binary cannot be started at all: sys/cmd.Open uses Start
// without Wait, so a failing opener would otherwise look successful
// and the browser flow would wait for a callback forever.
func noOpenerPath(t *testing.T) {
	t.Helper()
	t.Setenv("PATH", t.TempDir())
}

// feedStdin replaces os.Stdin with a pipe carrying the given input, for
// the interactive prompts of manual-mode syncs.
func feedStdin(t *testing.T, input string) {
	t.Helper()
	reader, writer, err := os.Pipe()
	assert.Nil(t, err)
	previous := os.Stdin
	os.Stdin = reader
	t.Cleanup(func() {
		os.Stdin = previous
		_ = reader.Close()
	})
	_, err = writer.WriteString(input)
	assert.Nil(t, err)
	assert.Nil(t, writer.Close())
}

// occupyAuthPort binds the OAuth2 loopback port for the duration of the
// test. Authenticate's nursery waits for every job, so a flow whose
// callback never arrives would hang forever; a server that cannot bind
// fails the flow fast and deterministically instead.
func occupyAuthPort(t *testing.T) {
	t.Helper()
	listener, err := net.Listen("tcp", "127.0.0.1:65535")
	assert.Nil(t, err)
	t.Cleanup(func() { _ = listener.Close() })
}

// resetState restores the package-level sync state between command runs.
func resetState(t *testing.T) {
	t.Helper()
	t.Cleanup(cleanup)
	cleanup()
}

// clearTrackCache removes the per-track cache files (download, artwork,
// lyrics) for the given ids, so cached state never leaks between tests.
func clearTrackCache(t *testing.T, ids ...string) {
	t.Helper()
	for _, id := range ids {
		track := &entity.Track{ID: id, Artwork: entity.Artwork{URL: "http://ima.ge"}}
		_ = os.Remove(track.Path().Download())
		_ = os.Remove(track.Path().Artwork())
		_ = os.Remove(track.Path().Lyrics())
	}
}
