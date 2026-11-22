package downloader

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/tamnd/ytb-cli/ytb"

	"github.com/streambinder/pango/sys"
	"github.com/stretchr/testify/assert"
)

// fakeFFmpeg installs a fake ffmpeg binary on PATH. It writes "audio"
// to its last argument, the output file the real tool would produce,
// or fails when fail is set.
func fakeFFmpeg(t *testing.T, fail bool) {
	t.Helper()
	dir := t.TempDir()
	script := "#!/bin/sh\nlast=\"\"\nfor arg in \"$@\"; do last=\"$arg\"; done\n"
	if fail {
		script += "echo \"ffmpeg blew up\" >&2\nexit 1\n"
	} else {
		script += "printf 'audio' > \"$last\"\nexit 0\n"
	}
	assert.Nil(t, os.WriteFile(filepath.Join(dir, "ffmpeg"), []byte(script), 0o755))
	t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))
}

// audioBytes is the canned stream the stub googlevideo host serves.
const audioBytes = "0123456789"

// playerJSON is a canned player response carrying one audio-only
// stream of ten bytes.
const playerJSON = `{"videoDetails":{"title":"Title","author":"Artist",` +
	`"lengthSeconds":"180"},"playabilityStatus":{"status":"OK"},` +
	`"streamingData":{"adaptiveFormats":[{"itag":140,` +
	`"mimeType":"audio/mp4; codecs=\"mp4a.40.2\"","bitrate":128000,` +
	`"audioQuality":"AUDIO_QUALITY_MEDIUM","contentLength":"10",` +
	`"url":"https://googlevideo.example/audio","audioChannels":2,` +
	`"audioSampleRate":"44100"}]}}`

// androidPlayerJSON is a canned ANDROID player response carrying one
// audio-only stream of ten bytes on the fallback host.
const androidPlayerJSON = `{"videoDetails":{"title":"Title","author":"Artist",` +
	`"lengthSeconds":"180"},"playabilityStatus":{"status":"OK"},` +
	`"streamingData":{"adaptiveFormats":[{"itag":140,` +
	`"mimeType":"audio/mp4; codecs=\"mp4a.40.2\"","bitrate":128000,` +
	`"audioQuality":"AUDIO_QUALITY_MEDIUM","contentLength":"10",` +
	`"url":"https://googlevideo-android.example/audio","audioChannels":2,` +
	`"audioSampleRate":"44100"}]}}`

// visionOSPlayerJSON is a canned VISIONOS player response carrying one
// audio-only stream of ten bytes on the fallback host.
const visionOSPlayerJSON = `{"videoDetails":{"title":"Title","author":"Artist",` +
	`"lengthSeconds":"180"},"playabilityStatus":{"status":"OK"},` +
	`"streamingData":{"adaptiveFormats":[{"itag":140,` +
	`"mimeType":"audio/mp4; codecs=\"mp4a.40.2\"","bitrate":128000,` +
	`"audioQuality":"AUDIO_QUALITY_MEDIUM","contentLength":"10",` +
	`"url":"https://googlevideo-visionos.example/audio","audioChannels":2,` +
	`"audioSampleRate":"44100"}]}}`

// videoOnlyVisionOSPlayerJSON is a canned VISIONOS player response
// carrying a video-only format and no audio.
const videoOnlyVisionOSPlayerJSON = `{"videoDetails":{},"playabilityStatus":{"status":"OK"},` +
	`"streamingData":{"adaptiveFormats":[{"itag":133,` +
	`"mimeType":"video/mp4; codecs=\"avc1.4d4015\"","bitrate":236000,` +
	`"contentLength":"10","url":"https://googlevideo-visionos.example/video"}]}}`

// youTubeDownloadStub scripts the YouTube HTTP boundary the shared
// client talks to for a download: the embed harvest page, the player
// endpoint, and the ranged googlevideo host.
type youTubeDownloadStub struct {
	playerStatus int
	playerBody   string
	// androidPlayerBody and androidPlayerStatus answer the player
	// endpoint for calls as the ANDROID client, whose stream list is
	// the download fallback's second stage; empty means the default body
	// and a 200.
	androidPlayerBody   string
	androidPlayerStatus int
	// androidPlayerCalls counts the player calls as the ANDROID
	// client when set, so a test can tell a FormatList re-read
	// from a single read.
	androidPlayerCalls  *int
	androidStreamStatus int // status every ranged GET on the ANDROID host answers with, 0 serves bytes
	// visionOSPlayerBody and visionOSPlayerStatus answer the player
	// endpoint for calls as the VISIONOS client, whose player
	// response is the download fallback's first stage; empty means
	// the default body and a 200.
	visionOSPlayerBody   string
	visionOSPlayerStatus int
	// visionOSPlayerCalls counts the player calls as the VISIONOS
	// client when set, so a test can tell a VISIONOS re-mint
	// from a single read.
	visionOSPlayerCalls  *int
	visionOSStreamStatus int // status every ranged GET on the VISIONOS host answers with, 0 serves bytes
	streamStatus         int
	stream403            bool // the first ranged GET answers 403, then serves bytes
	stream403Always      bool // every ranged GET on the ANDROID_VR host answers 403
	retries              int
	streamCalls          int
	// rangeLog records the host and Range header of every googlevideo
	// request when set, so a test can tell a fresh download from a
	// resume.
	rangeLog *[]string
}

func (stub *youTubeDownloadStub) roundTrip(request *http.Request) (*http.Response, error) {
	switch {
	case request.Method == http.MethodGet && strings.HasPrefix(request.URL.Path, "/embed/"):
		return jsonResponse(200, `"INNERTUBE_API_KEY":"test-key",`+
			`"INNERTUBE_CLIENT_VERSION":"2.0","VISITOR_DATA":"test-visitor"`), nil
	case request.Method == http.MethodPost && request.URL.Path == "/youtubei/v1/player":
		return stub.playerCallResponse(request), nil
	case request.URL.Host == "googlevideo.example" || request.URL.Host == "googlevideo-android.example" || request.URL.Host == "googlevideo-visionos.example":
		return stub.googlevideoResponse(request), nil
	}
	return jsonResponse(404, `{}`), nil
}

// playerCallResponse answers a player call for the client its user
// agent claims: ANDROID, VISIONOS, or the default body.
func (stub *youTubeDownloadStub) playerCallResponse(request *http.Request) *http.Response {
	if isAndroidPlayer(request) {
		if stub.androidPlayerCalls != nil {
			*stub.androidPlayerCalls++
		}
		body := stub.androidPlayerBody
		if body == "" {
			body = stub.playerBody
		}
		return stub.playerResponse(stub.androidPlayerStatus, body)
	}
	if isVisionOSPlayer(request) {
		if stub.visionOSPlayerCalls != nil {
			*stub.visionOSPlayerCalls++
		}
		body := stub.visionOSPlayerBody
		if body == "" {
			body = stub.playerBody
		}
		return stub.playerResponse(stub.visionOSPlayerStatus, body)
	}
	return stub.playerResponse(stub.playerStatus, stub.playerBody)
}

// playerResponse answers a player call with the scripted status and
// body, defaulting to 200 and the canned single-stream response.
func (stub *youTubeDownloadStub) playerResponse(status int, body string) *http.Response {
	if status == 0 {
		status = 200
	}
	if body == "" {
		body = playerJSON
	}
	return jsonResponse(status, body)
}

// googlevideoResponse answers a ranged GET on either googlevideo
// host, applying the scripted refusals for the host the request
// hits.
func (stub *youTubeDownloadStub) googlevideoResponse(request *http.Request) *http.Response {
	if stub.rangeLog != nil {
		*stub.rangeLog = append(*stub.rangeLog, request.URL.Host+" "+request.Header.Get("Range"))
	}
	if request.URL.Host == "googlevideo.example" {
		if stub.streamStatus != 0 {
			return rawResponse(stub.streamStatus, "text/plain", []byte("ko"))
		}
		if stub.stream403Always {
			return rawResponse(403, "text/plain", []byte("forbidden"))
		}
		if stub.stream403 && stub.streamCalls == 0 {
			stub.streamCalls++
			return rawResponse(403, "text/plain", []byte("forbidden"))
		}
	}
	if request.URL.Host == "googlevideo-android.example" && stub.androidStreamStatus != 0 {
		return rawResponse(stub.androidStreamStatus, "text/plain", []byte("ko"))
	}
	if request.URL.Host == "googlevideo-visionos.example" && stub.visionOSStreamStatus != 0 {
		return rawResponse(stub.visionOSStreamStatus, "text/plain", []byte("ko"))
	}
	stub.streamCalls++
	return rangeResponse(request)
}

// isVisionOSPlayer reports whether the request is an InnerTube player
// call as the VISIONOS client: its user agent is a desktop Safari
// one carrying "Version/26.0 Safari", where the web Safari one
// carries "Version/15.5 Safari".
func isVisionOSPlayer(request *http.Request) bool {
	return strings.Contains(request.Header.Get("User-Agent"), "Version/26.0 Safari")
}

// isAndroidPlayer reports whether the request is an InnerTube player
// call as the ANDROID client: its user agent is
// "com.google.android.youtube/…", where the ANDROID_VR one carries
// "com.google.android.apps.youtube.vr.oculus/…".
func isAndroidPlayer(request *http.Request) bool {
	return strings.Contains(request.Header.Get("User-Agent"), "com.google.android.youtube/")
}

// rangeResponse answers a ranged GET out of the canned audio bytes.
func rangeResponse(request *http.Request) *http.Response {
	start, end := 0, len(audioBytes)-1
	if span := request.Header.Get("Range"); span != "" {
		// a malformed Range falls back to the full byte range
		var rangeStart, rangeEnd int
		if _, err := fmt.Sscanf(span, "bytes=%d-%d", &rangeStart, &rangeEnd); err == nil {
			start, end = rangeStart, rangeEnd
		}
	}
	body := audioBytes[start : end+1]
	response := rawResponse(206, "audio/mp4", []byte(body))
	response.Header.Set("Content-Range",
		fmt.Sprintf("bytes %d-%d/%d", start, end, len(audioBytes)))
	return response
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

// stubYouTube replaces the shared client with one built with no
// pacing and no retries, talking to the stub transport, for the
// duration of the test.
func stubYouTube(t *testing.T, stub youTubeDownloadStub) {
	t.Helper()
	client := ytb.NewClient(ytb.Config{
		Workers: 1,
		Delay:   0,
		Timeout: 30 * time.Second,
		Retries: stub.retries,
		HL:      "en",
		GL:      "US",
	})
	client.HTTP().Transport = roundTripFunc(stub.roundTrip)
	previous := sys.YouTubeClient
	sys.YouTubeClient = client
	t.Cleanup(func() { sys.YouTubeClient = previous })
}

// rawPath is the temporary raw file a download of path leaves beside
// the target.
func rawPath(path string) string {
	return sys.FileBaseStem(path) + ".raw"
}

// unreachableTransport makes every blob support probe fail, so the
// download dispatch moves past the blob downloader.
func unreachableTransport(t *testing.T) {
	t.Helper()
	stubTransport(t, func(*http.Request) (*http.Response, error) {
		return nil, errors.New("ko")
	})
}

func BenchmarkDownload(b *testing.B) {
	for i := 0; i < b.N; i++ {
		TestDownload(&testing.T{})
	}
}

func TestDownload(t *testing.T) {
	unreachableTransport(t)
	stubYouTube(t, youTubeDownloadStub{})
	fakeFFmpeg(t, false)

	// testing: a missing target file is downloaded through ytb and
	// transcoded to the target, leaving no raw file behind
	path := filepath.Join(t.TempDir(), "fname.txt")
	ch := make(chan []byte, 1)
	defer close(ch)
	assert.Nil(t, Download(context.TODO(), "http://youtu.be", path, nil, ch))
	content, err := os.ReadFile(path)
	assert.Nil(t, err)
	assert.Equal(t, []byte("audio"), content)
	_, err = os.Stat(rawPath(path))
	assert.ErrorIs(t, err, os.ErrNotExist)
}

func TestDownloadEmpty(t *testing.T) {
	assert.Nil(t, Download(context.TODO(), "", "fname.txt", nil))
}

func TestDownloadAlreadyExists(t *testing.T) {
	// testing: an existing target file is served from the cache and
	// its content is fed to the channels
	path := filepath.Join(t.TempDir(), "fname.txt")
	assert.Nil(t, os.WriteFile(path, []byte("cached"), 0o644))
	ch := make(chan []byte, 1)
	defer close(ch)
	assert.Nil(t, Download(context.TODO(), "http://youtu.be", path, nil, ch))
	assert.Equal(t, []byte("cached"), <-ch)
}

func TestDownloadMakeDirFailure(t *testing.T) {
	unreachableTransport(t)

	// the target's parent path is an existing file: MkdirAll fails
	blocker := filepath.Join(t.TempDir(), "blocker")
	assert.Nil(t, os.WriteFile(blocker, []byte{}, 0o644))
	path := filepath.Join(blocker, "sub", "fname.txt")

	// testing
	assert.NotNil(t, Download(context.TODO(), "http://youtu.be", path, nil))
}

func TestDownloadUnsupported(t *testing.T) {
	// the blob probe answers a plain text page: no downloader supports
	// the URL
	stubTransport(t, func(*http.Request) (*http.Response, error) {
		return blobResponse(200, "text/plain", ""), nil
	})

	// testing
	path := filepath.Join(t.TempDir(), "fname.txt")
	assert.ErrorContains(t, Download(context.TODO(), "http://davidepucci.it", path, nil), "unsupported url")
}

func TestDownloadYouTubeFailure(t *testing.T) {
	unreachableTransport(t)
	stubYouTube(t, youTubeDownloadStub{streamStatus: 500})
	fakeFFmpeg(t, false)

	// testing: a failing stream download leaves no partial file at
	// the target path or at the raw path
	path := filepath.Join(t.TempDir(), "fname.txt")
	assert.ErrorContains(t, Download(context.TODO(), "http://youtu.be", path, nil), "youtube:")
	_, err := os.Stat(path)
	assert.ErrorIs(t, err, os.ErrNotExist)
	_, err = os.Stat(rawPath(path))
	assert.ErrorIs(t, err, os.ErrNotExist)
}

func TestDownloadYouTubeNoStreams(t *testing.T) {
	unreachableTransport(t)
	stubYouTube(t, youTubeDownloadStub{playerBody: `{"videoDetails":{},` +
		`"playabilityStatus":{"status":"OK"},"streamingData":{}}`})
	fakeFFmpeg(t, false)

	// testing: a manifest with no downloadable streams is an error
	// naming the video, not a silent miss
	path := filepath.Join(t.TempDir(), "fname.txt")
	assert.ErrorContains(t, Download(context.TODO(), "http://youtu.be", path, nil), "no downloadable streams")
}

func TestDownloadYouTubeManifestFailure(t *testing.T) {
	unreachableTransport(t)
	stubYouTube(t, youTubeDownloadStub{playerStatus: 500})
	fakeFFmpeg(t, false)

	// testing: a manifest that resolves to no streams is a provider
	// error, and leaves no partial file behind
	path := filepath.Join(t.TempDir(), "fname.txt")
	assert.ErrorContains(t, Download(context.TODO(), "http://youtu.be", path, nil), "youtube:")
	_, err := os.Stat(path)
	assert.ErrorIs(t, err, os.ErrNotExist)
	_, err = os.Stat(rawPath(path))
	assert.ErrorIs(t, err, os.ErrNotExist)
}

func TestDownloadYouTubeUnresolvableStream(t *testing.T) {
	unreachableTransport(t)
	// the only stream carries a signature cipher and no plain URL:
	// resolving it is an error, not a fallback
	stubYouTube(t, youTubeDownloadStub{playerBody: `{"videoDetails":{},` +
		`"playabilityStatus":{"status":"OK"},"streamingData":` +
		`{"adaptiveFormats":[{"itag":140,` +
		`"mimeType":"audio/mp4; codecs=\"mp4a.40.2\"","bitrate":128000,` +
		`"signatureCipher":"s=abc&url=https%3A%2F%2Fgooglevideo.example%2Faudio&sp=sig"}]}}`})
	fakeFFmpeg(t, false)

	// testing
	path := filepath.Join(t.TempDir(), "fname.txt")
	assert.ErrorContains(t, Download(context.TODO(), "http://youtu.be", path, nil), "signatureCipher")
	_, err := os.Stat(path)
	assert.ErrorIs(t, err, os.ErrNotExist)
}

func TestDownloadRefresh(t *testing.T) {
	unreachableTransport(t)
	// the CDN refuses the first ranged GET: the downloader re-reads
	// the manifest for the same itag and finishes the download
	stubYouTube(t, youTubeDownloadStub{stream403: true, retries: 1})
	fakeFFmpeg(t, false)

	// testing
	path := filepath.Join(t.TempDir(), "fname.txt")
	ch := make(chan []byte, 1)
	defer close(ch)
	assert.Nil(t, Download(context.TODO(), "http://youtu.be", path, nil, ch))
	content, err := os.ReadFile(path)
	assert.Nil(t, err)
	assert.Equal(t, []byte("audio"), content)
	_, err = os.Stat(rawPath(path))
	assert.ErrorIs(t, err, os.ErrNotExist)
}

func TestDownloadYouTubeFallback(t *testing.T) {
	unreachableTransport(t)
	rangeLog := []string{}
	stubYouTube(t, youTubeDownloadStub{
		stream403Always:    true,
		visionOSPlayerBody: visionOSPlayerJSON,
		rangeLog:           &rangeLog,
	})
	fakeFFmpeg(t, false)

	// testing: a CDN refusal of the ANDROID_VR URL falls back to a
	// VISIONOS mint and restarts from byte 0 instead of resuming
	// into the refused stream's part file
	path := filepath.Join(t.TempDir(), "fname.txt")
	assert.Nil(t, os.WriteFile(rawPath(path), []byte(audioBytes[:4]), 0o600))
	ch := make(chan []byte, 1)
	defer close(ch)
	assert.Nil(t, Download(context.TODO(), "http://youtu.be", path, nil, ch))
	content, err := os.ReadFile(path)
	assert.Nil(t, err)
	assert.Equal(t, []byte("audio"), content)
	_, err = os.Stat(rawPath(path))
	assert.ErrorIs(t, err, os.ErrNotExist)
	assert.Contains(t, rangeLog, "googlevideo-visionos.example bytes=0-9")
}

func TestDownloadYouTubeFallbackVisionOSNoAudio(t *testing.T) {
	unreachableTransport(t)
	rangeLog := []string{}
	stubYouTube(t, youTubeDownloadStub{
		stream403Always:    true,
		visionOSPlayerBody: videoOnlyVisionOSPlayerJSON,
		androidPlayerBody:  androidPlayerJSON,
		rangeLog:           &rangeLog,
	})
	fakeFFmpeg(t, false)

	// testing: a VISIONOS response with no audio format falls
	// through to the ANDROID list, which serves the download
	path := filepath.Join(t.TempDir(), "fname.txt")
	ch := make(chan []byte, 1)
	defer close(ch)
	assert.Nil(t, Download(context.TODO(), "http://youtu.be", path, nil, ch))
	content, err := os.ReadFile(path)
	assert.Nil(t, err)
	assert.Equal(t, []byte("audio"), content)
	_, err = os.Stat(rawPath(path))
	assert.ErrorIs(t, err, os.ErrNotExist)
	assert.Contains(t, rangeLog, "googlevideo-android.example bytes=0-9")
}

func TestDownloadYouTubeFallbackNoAudio(t *testing.T) {
	unreachableTransport(t)
	stubYouTube(t, youTubeDownloadStub{
		stream403Always: true,
		// neither the VISIONOS response nor the ANDROID list
		// carries an audio format
		visionOSPlayerBody: videoOnlyVisionOSPlayerJSON,
		androidPlayerBody: `{"videoDetails":{},"playabilityStatus":{"status":"OK"},` +
			`"streamingData":{"adaptiveFormats":[{"itag":133,` +
			`"mimeType":"video/mp4; codecs=\"avc1.4d4015\"","bitrate":236000,` +
			`"contentLength":"10","url":"https://googlevideo-android.example/video"}]}}`,
	})
	fakeFFmpeg(t, false)

	// testing: a CDN refusal with no audio format in either
	// fallback source fails with the original refusal, leaving no
	// partial file
	path := filepath.Join(t.TempDir(), "fname.txt")
	assert.ErrorContains(t, Download(context.TODO(), "http://youtu.be", path, nil), "the CDN refused this stream URL")
	_, err := os.Stat(path)
	assert.ErrorIs(t, err, os.ErrNotExist)
	_, err = os.Stat(rawPath(path))
	assert.ErrorIs(t, err, os.ErrNotExist)
}

func TestDownloadYouTubeFallbackListFailure(t *testing.T) {
	unreachableTransport(t)
	stubYouTube(t, youTubeDownloadStub{
		stream403Always:      true,
		visionOSPlayerStatus: 500,
		androidPlayerStatus:  500,
	})
	fakeFFmpeg(t, false)

	// testing: a CDN refusal with both fallback sources failing
	// fails with the original refusal, leaving no partial file
	path := filepath.Join(t.TempDir(), "fname.txt")
	assert.ErrorContains(t, Download(context.TODO(), "http://youtu.be", path, nil), "the CDN refused this stream URL")
	_, err := os.Stat(path)
	assert.ErrorIs(t, err, os.ErrNotExist)
	_, err = os.Stat(rawPath(path))
	assert.ErrorIs(t, err, os.ErrNotExist)
}

func TestDownloadYouTubeFallbackRefresh(t *testing.T) {
	unreachableTransport(t)
	visionOSPlayerCalls := 0
	androidPlayerCalls := 0
	stubYouTube(t, youTubeDownloadStub{
		stream403Always:      true,
		visionOSPlayerBody:   visionOSPlayerJSON,
		visionOSPlayerCalls:  &visionOSPlayerCalls,
		visionOSStreamStatus: 403,
		androidPlayerBody:    androidPlayerJSON,
		androidPlayerCalls:   &androidPlayerCalls,
		androidStreamStatus:  403,
		retries:              1,
	})
	fakeFFmpeg(t, false)

	// testing: a CDN refusal of a fallback URL re-reads that
	// stage's source for the same itag — one read for the stage,
	// one for its refresh — before failing with the refusal,
	// leaving no partial file
	path := filepath.Join(t.TempDir(), "fname.txt")
	assert.ErrorContains(t, Download(context.TODO(), "http://youtu.be", path, nil), "the CDN refused this stream URL")
	assert.Equal(t, 2, visionOSPlayerCalls)
	assert.Equal(t, 2, androidPlayerCalls)
	_, err := os.Stat(path)
	assert.ErrorIs(t, err, os.ErrNotExist)
	_, err = os.Stat(rawPath(path))
	assert.ErrorIs(t, err, os.ErrNotExist)
}

func TestDownloadTranscodeFailure(t *testing.T) {
	unreachableTransport(t)
	stubYouTube(t, youTubeDownloadStub{})
	fakeFFmpeg(t, true)

	// testing: a failing transcode leaves no partial file at the
	// target path or at the raw path
	path := filepath.Join(t.TempDir(), "fname.txt")
	assert.ErrorContains(t, Download(context.TODO(), "http://youtu.be", path, nil), "ffmpeg blew up")
	_, err := os.Stat(path)
	assert.ErrorIs(t, err, os.ErrNotExist)
	_, err = os.Stat(rawPath(path))
	assert.ErrorIs(t, err, os.ErrNotExist)
}

func TestDownloadResume(t *testing.T) {
	unreachableTransport(t)
	stubYouTube(t, youTubeDownloadStub{})
	fakeFFmpeg(t, false)

	// testing: an interrupted raw download resumes from the raw
	// temporary file instead of starting over
	path := filepath.Join(t.TempDir(), "fname.txt")
	assert.Nil(t, os.WriteFile(rawPath(path), []byte(audioBytes[:4]), 0o600))
	ch := make(chan []byte, 1)
	defer close(ch)
	assert.Nil(t, Download(context.TODO(), "http://youtu.be", path, nil, ch))
	content, err := os.ReadFile(path)
	assert.Nil(t, err)
	assert.Equal(t, []byte("audio"), content)
	_, err = os.Stat(rawPath(path))
	assert.ErrorIs(t, err, os.ErrNotExist)
}

func TestDownloadEmptyCacheFile(t *testing.T) {
	unreachableTransport(t)
	stubYouTube(t, youTubeDownloadStub{})
	fakeFFmpeg(t, false)

	// testing: an empty cache file is dropped and downloaded again
	path := filepath.Join(t.TempDir(), "cached.mp3")
	assert.Nil(t, os.WriteFile(path, []byte{}, 0o600))
	ch := make(chan []byte, 1)
	defer close(ch)
	assert.Nil(t, Download(context.TODO(), "http://youtu.be", path, nil, ch))
}
