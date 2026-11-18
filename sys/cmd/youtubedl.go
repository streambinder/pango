package cmd

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/streambinder/pango/sys"
)

func YouTubeDl(ctx context.Context, url, path string) error {
	var (
		output bytes.Buffer
		ext    = filepath.Ext(path)[1:]
		stem   = strings.TrimSuffix(sys.FileBaseStem(path), "."+ext)
		cmd    = exec.CommandContext(ctx,
			"yt-dlp",
			"--format", "bestaudio/best",
			"--extract-audio",
			"--audio-format", ext,
			"--audio-quality", "0",
			"--output", stem+".%(ext)s",
			"--continue",
			"--no-overwrites",
			url,
		)
	)
	cmd.Stdout = &output
	cmd.Stderr = &output
	if err := cmd.Run(); err != nil {
		// a failed run must not leave a partial behind:
		// downloader.Download would pick it up as if complete
		os.Remove(stem + "." + ext)
		return errors.New(output.String())
	}
	return nil
}

// YouTubeDlSearch runs a yt-dlp flat playlist search for the given
// query and returns its JSON-lines output.
func YouTubeDlSearch(ctx context.Context, query string) ([]byte, error) {
	var (
		output bytes.Buffer
		//nolint:gosec // the query is one argv element, never a shell string.
		cmd = exec.CommandContext(ctx,
			"yt-dlp",
			"--dump-json", "--flat-playlist", "--no-download", "--no-warnings",
			"ytsearch10:"+query,
		)
	)
	cmd.Stdout = &output
	cmd.Stderr = &output
	if err := cmd.Run(); err != nil {
		if errors.Is(err, exec.ErrNotFound) || errors.Is(err, os.ErrNotExist) {
			return nil, errors.New("yt-dlp binary not found in PATH")
		}
		if msg := strings.TrimSpace(output.String()); msg != "" {
			return nil, fmt.Errorf("yt-dlp search failed: %s", msg)
		}
		return nil, fmt.Errorf("yt-dlp search failed: %w", err)
	}
	return output.Bytes(), nil
}
