package downloader

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strings"

	"github.com/tamnd/ytb-cli/ytb"

	"github.com/streambinder/pango/processor"
	"github.com/streambinder/pango/sys"
	"github.com/streambinder/pango/sys/cmd"
)

type youTube struct {
	Downloader
}

func init() {
	downloaders = append(downloaders, youTube{})
}

func (youTube) supports(_ context.Context, url string) bool {
	return strings.Contains(url, "://youtu.be") || strings.Contains(url, "://www.youtube.com")
}

func (youTube) download(ctx context.Context, url, path string, _ processor.Processor, channels ...chan []byte) error {
	// in this case, data won't be passed through channels
	// as too heavy
	for _, ch := range channels {
		ch <- nil
	}

	// the stream lands in a raw temporary file beside the target and
	// is transcoded to the target extension after the download: a
	// failed run must not leave a partial behind, since
	// downloader.Download would pick it up as if complete
	rawPath := sys.FileBaseStem(path) + ".raw"
	completed := false
	defer func() {
		os.Remove(rawPath)
		if !completed {
			os.Remove(path)
		}
	}()

	manifest, err := sys.YouTubeClient.StreamManifest(ctx, url)
	if err != nil {
		return fmt.Errorf("youtube: %w", err)
	}
	selection, err := ytb.SelectFormat(manifest.Streams, "bestaudio/best")
	if err != nil {
		return fmt.Errorf("youtube: %w", err)
	}
	streams := selection.Streams()
	if len(streams) == 0 {
		return fmt.Errorf("youtube: no downloadable streams for %s", url)
	}
	stream := streams[0]

	streamURL, err := sys.YouTubeClient.ResolveStreamURL(ctx, manifest, &stream)
	if err != nil {
		return fmt.Errorf("youtube: %w", err)
	}

	if err := downloadURL(ctx, streamURL, rawPath, stream.ContentLength, stream.UserAgent(), manifestRefresh(url, stream.ITag), true); err != nil {
		if !isStaleStreamURL(err) {
			return fmt.Errorf("youtube: %w", err)
		}
		// the CDN refused the ANDROID_VR-minted URL, and the merged
		// manifest carries only that client's streams, so re-reading
		// it cannot help: retry from another client instead,
		// restarting from byte 0
		if err = downloadFallback(ctx, url, rawPath, err); err != nil {
			return fmt.Errorf("youtube: %w", err)
		}
	}

	if err := cmd.FFmpeg().Transcode(rawPath, path); err != nil {
		return err
	}

	completed = true
	return nil
}

// downloadURL fetches streamURL into rawPath in ranged chunks, with
// refresh re-reading the stream URL when the CDN refuses it or it is
// about to expire. resume continues a part file from its size;
// without it the part file is truncated and the download restarts
// from byte 0.
func downloadURL(ctx context.Context, streamURL, rawPath string, total int64, userAgent string, refresh func(context.Context) (string, error), resume bool) error {
	return sys.YouTubeClient.DownloadToFile(ctx, streamURL, rawPath, ytb.DownloadOptions{
		Total:     total,
		Workers:   1,
		UserAgent: userAgent,
		Refresh:   refresh,
		Resume:    resume,
	})
}

// manifestRefresh re-reads the stream manifest for itag when the URL
// is about to expire or the CDN refuses it.
func manifestRefresh(url string, itag int) func(context.Context) (string, error) {
	return func(ctx context.Context) (string, error) {
		fresh, err := sys.YouTubeClient.StreamManifest(ctx, url)
		if err != nil {
			return "", err
		}
		freshSelection, err := ytb.SelectFormat(fresh.Streams, "bestaudio/best")
		if err != nil {
			return "", err
		}
		for _, freshStream := range freshSelection.Streams() {
			if freshStream.ITag == itag {
				return sys.YouTubeClient.ResolveStreamURL(ctx, fresh, &freshStream)
			}
		}
		return "", fmt.Errorf("stream %d is no longer available for %s", itag, url)
	}
}

// isStaleStreamURL reports whether err is ytb's CDN-refusal error.
// errStreamURLStale is unexported in ytb-cli, so the message text is
// the only signal: a ytb upgrade that rewords it shows up as a
// fallback that never fires, not as a crash.
func isStaleStreamURL(err error) bool {
	return err != nil && strings.Contains(err.Error(), "the CDN refused this stream URL")
}

// errFallbackNoSource reports that a fallback stage's source
// yielded no usable audio format, so the next stage can be tried.
var errFallbackNoSource = errors.New("fallback source has no usable audio format")

// downloadFallback retries a refused download from another client.
// VISIONOS goes first: its URLs need no proof-of-origin token, so
// the CDN serves them where the mobile clients' URLs are refused
// past the first chunk. ANDROID follows for the videos VISIONOS
// cannot serve, such as made-for-kids. Each stage restarts from
// byte 0: the part file holds bytes of the refused stream. When no
// stage can even start, staleErr is returned unchanged: the CDN
// refusal is still the reason the download failed.
func downloadFallback(ctx context.Context, url, rawPath string, staleErr error) error {
	var lastErr error
	for _, stage := range []func(context.Context, string, string) error{
		downloadVisionOS, downloadAndroid,
	} {
		err := stage(ctx, url, rawPath)
		if err == nil {
			return nil
		}
		if errors.Is(err, errFallbackNoSource) {
			continue
		}
		lastErr = err
	}
	if lastErr != nil {
		return lastErr
	}
	return staleErr
}

// visionOSSpec is the InnerTube identity the fallback mints stream
// URLs as: VISIONOS 1.02 on a RealityDevice17,1, the identity yt-dlp
// uses for its visionos client, which carries no proof-of-origin
// token policy. ytb-cli has no such client, so the spec is spelled
// out here from yt-dlp's client table.
func visionOSSpec() ytb.ClientSpec {
	return ytb.ClientSpec{
		Name:      "VISIONOS",
		Num:       101,
		Version:   "1.02",
		UserAgent: "Mozilla/5.0 (Macintosh; Intel Mac OS X 15_7_3) AppleWebKit/605.1.15 (KHTML, like Gecko) Version/26.0 Safari/605.1.15",
		Host:      "www.youtube.com",
		Extra: map[string]any{
			"deviceMake":  "Apple",
			"deviceModel": "RealityDevice17,1",
			"osName":      "visionOS",
			"osVersion":   "26.5.23O471",
		},
	}
}

// visionOSPlayer reads the player response as VISIONOS, with the
// same body ytb's own player reads use.
func visionOSPlayer(ctx context.Context, videoID string) (map[string]any, error) {
	return sys.YouTubeClient.Call(ctx, visionOSSpec(), "player", map[string]any{
		"videoId": videoID,
		"playbackContext": map[string]any{
			"contentPlaybackContext": map[string]any{
				"html5Preference": "HTML5_PREF_WANTS",
			},
		},
		"contentCheckOk": true,
		"racyCheckOk":    true,
	}, "video "+videoID)
}

// downloadVisionOS retries the download with the best audio format
// from a VISIONOS player response.
func downloadVisionOS(ctx context.Context, url, rawPath string) error {
	videoID := ytb.ExtractVideoID(url)
	if videoID == "" {
		videoID = url
	}
	pr, err := visionOSPlayer(ctx, videoID)
	if err != nil {
		return errFallbackNoSource
	}
	format, ok := bestAudioFormat(ytb.ParseVideoFormats(pr, videoID))
	if !ok {
		return errFallbackNoSource
	}
	os.Remove(rawPath)
	return downloadURL(ctx, format.URL, rawPath, format.ContentLength, visionOSSpec().UserAgent, visionOSRefresh(url, format.ITag), false)
}

// visionOSRefresh re-mints the stream URL for itag as VISIONOS when
// the fallback URL is about to expire or the CDN refuses it: the
// manifest would hand back the refused ANDROID_VR URL for the same
// itag.
func visionOSRefresh(url string, itag int) func(context.Context) (string, error) {
	return func(ctx context.Context) (string, error) {
		videoID := ytb.ExtractVideoID(url)
		if videoID == "" {
			videoID = url
		}
		pr, err := visionOSPlayer(ctx, videoID)
		if err != nil {
			return "", err
		}
		for _, f := range ytb.ParseVideoFormats(pr, videoID) {
			if f.ITag == itag && f.URL != "" {
				return f.URL, nil
			}
		}
		return "", fmt.Errorf("stream %d is no longer available for %s", itag, url)
	}
}

// downloadAndroid retries the download with the best audio format
// from the ANDROID player's stream list.
func downloadAndroid(ctx context.Context, url, rawPath string) error {
	list, err := sys.YouTubeClient.FormatList(ctx, url)
	if err != nil {
		return errFallbackNoSource
	}
	format, ok := bestAudioFormat(list.Formats)
	if !ok {
		return errFallbackNoSource
	}
	os.Remove(rawPath)
	return downloadURL(ctx, format.URL, rawPath, format.ContentLength, ytb.ClientANDROID().UserAgent, formatListRefresh(url, format.ITag), false)
}

// bestAudioFormat picks the highest-bitrate audio format carrying a
// URL out of a stream list.
func bestAudioFormat(formats []ytb.VideoFormat) (ytb.VideoFormat, bool) {
	var best ytb.VideoFormat
	found := false
	for _, f := range formats {
		if f.MediaKind != "audio" || f.URL == "" {
			continue
		}
		if !found || f.Bitrate > best.Bitrate {
			best, found = f, true
		}
	}
	return best, found
}

// formatListRefresh re-reads the ANDROID stream list for itag when
// the fallback URL is about to expire or the CDN refuses it: the
// manifest would hand back the refused ANDROID_VR URL for the same
// itag.
func formatListRefresh(url string, itag int) func(context.Context) (string, error) {
	return func(ctx context.Context) (string, error) {
		list, err := sys.YouTubeClient.FormatList(ctx, url)
		if err != nil {
			return "", err
		}
		for _, f := range list.Formats {
			if f.ITag == itag && f.URL != "" {
				return f.URL, nil
			}
		}
		return "", fmt.Errorf("stream %d is no longer available for %s", itag, url)
	}
}
