package cmd

import (
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/streambinder/pango/sys"
	"github.com/stretchr/testify/assert"
)

func TestCmdUnsyncPlaylist(t *testing.T) {
	musicDir, _ := syncEnv(t, nil, webScript{})
	writeTaggedFile(t, filepath.Join(musicDir, "Artist - Title.mp3"), "123")
	assert.Nil(t, os.WriteFile(filepath.Join(musicDir, "Mix.m3u"), []byte("#EXTM3U\n"), 0o644))
	assert.Nil(t, os.WriteFile(filepath.Join(musicDir, "Mix.pls"), []byte("[playlist]\n"), 0o644))

	// testing: the playlist tracks and both playlist files are deleted
	assert.Nil(t, sys.ErrOnly(testExecute(cmdUnsync(), "-o", musicDir, "-p", "Mix")))
	_, err := os.Stat(filepath.Join(musicDir, "Artist - Title.mp3"))
	assert.True(t, os.IsNotExist(err))
	_, err = os.Stat(filepath.Join(musicDir, "Mix.m3u"))
	assert.True(t, os.IsNotExist(err))
	_, err = os.Stat(filepath.Join(musicDir, "Mix.pls"))
	assert.True(t, os.IsNotExist(err))
}

func TestCmdUnsyncPlaylistOneEncoding(t *testing.T) {
	musicDir, _ := syncEnv(t, nil, webScript{})
	writeTaggedFile(t, filepath.Join(musicDir, "Artist - Title.mp3"), "123")
	assert.Nil(t, os.WriteFile(filepath.Join(musicDir, "Mix.m3u"), []byte("#EXTM3U\n"), 0o644))

	// testing: a playlist file in one encoding only deletes without error
	assert.Nil(t, sys.ErrOnly(testExecute(cmdUnsync(), "-o", musicDir, "-p", "Mix")))
	_, err := os.Stat(filepath.Join(musicDir, "Artist - Title.mp3"))
	assert.True(t, os.IsNotExist(err))
	_, err = os.Stat(filepath.Join(musicDir, "Mix.m3u"))
	assert.True(t, os.IsNotExist(err))
}

func TestCmdUnsyncPlaylistNoLocalTracks(t *testing.T) {
	musicDir, _ := syncEnv(t, nil, webScript{})
	assert.Nil(t, os.WriteFile(filepath.Join(musicDir, "Mix.m3u"), []byte("#EXTM3U\n"), 0o644))
	assert.Nil(t, os.WriteFile(filepath.Join(musicDir, "Mix.pls"), []byte("[playlist]\n"), 0o644))

	// testing: a playlist with no local tracks reports the tracks as
	// absent and still deletes the playlist files
	output := captureStdout(t, func() {
		assert.Nil(t, sys.ErrOnly(testExecute(cmdUnsync(), "-o", musicDir, "-p", "Mix")))
	})
	assert.Contains(t, output, "absent Title by Artist")
	_, err := os.Stat(filepath.Join(musicDir, "Mix.m3u"))
	assert.True(t, os.IsNotExist(err))
	_, err = os.Stat(filepath.Join(musicDir, "Mix.pls"))
	assert.True(t, os.IsNotExist(err))
}

func TestCmdUnsyncAlbum(t *testing.T) {
	musicDir, _ := syncEnv(t, nil, webScript{})
	writeTaggedFile(t, filepath.Join(musicDir, "Artist - Title.mp3"), "123")

	// testing: the album track is deleted and no playlist file is touched
	assert.Nil(t, os.WriteFile(filepath.Join(musicDir, "Mix.m3u"), []byte("#EXTM3U\n"), 0o644))
	assert.Nil(t, sys.ErrOnly(testExecute(cmdUnsync(), "-o", musicDir, "-a", "album123")))
	_, err := os.Stat(filepath.Join(musicDir, "Artist - Title.mp3"))
	assert.True(t, os.IsNotExist(err))
	_, err = os.Stat(filepath.Join(musicDir, "Mix.m3u"))
	assert.Nil(t, err)
}

func TestCmdUnsyncTrack(t *testing.T) {
	musicDir, _ := syncEnv(t, nil, webScript{})
	writeTaggedFile(t, filepath.Join(musicDir, "Artist - Title.mp3"), "123")

	// testing: the track is deleted
	assert.Nil(t, sys.ErrOnly(testExecute(cmdUnsync(), "-o", musicDir, "-t", "123")))
	_, err := os.Stat(filepath.Join(musicDir, "Artist - Title.mp3"))
	assert.True(t, os.IsNotExist(err))
}

func TestCmdUnsyncTrackMismatch(t *testing.T) {
	musicDir, _ := syncEnv(t, nil, webScript{})
	writeTaggedFile(t, filepath.Join(musicDir, "Artist - Title.mp3"), "other-id")

	// testing: a same-name file with a mismatching identifier survives
	output := captureStdout(t, func() {
		assert.Nil(t, sys.ErrOnly(testExecute(cmdUnsync(), "-o", musicDir, "-t", "123")))
	})
	assert.Contains(t, output, "skipping Title by Artist: spotify ID mismatch")
	_, err := os.Stat(filepath.Join(musicDir, "Artist - Title.mp3"))
	assert.Nil(t, err)
}

func TestCmdUnsyncTrackUntagged(t *testing.T) {
	musicDir, _ := syncEnv(t, nil, webScript{})
	writeTaggedFile(t, filepath.Join(musicDir, "Artist - Title.mp3"), "")

	// testing: a file with no stored identifier is skipped, never deleted
	assert.Nil(t, sys.ErrOnly(testExecute(cmdUnsync(), "-o", musicDir, "-t", "123")))
	_, err := os.Stat(filepath.Join(musicDir, "Artist - Title.mp3"))
	assert.Nil(t, err)
}

func TestCmdUnsyncTrackTwice(t *testing.T) {
	musicDir, _ := syncEnv(t, nil, webScript{})
	writeTaggedFile(t, filepath.Join(musicDir, "Artist - Title.mp3"), "123")

	// testing: a track in two requested collections is deleted once,
	// the second occurrence reports as already deleted
	output := captureStdout(t, func() {
		assert.Nil(t, sys.ErrOnly(testExecute(cmdUnsync(), "-o", musicDir, "-t", "123", "-p", "Mix")))
	})
	assert.Contains(t, output, "already deleted Title by Artist")
	_, err := os.Stat(filepath.Join(musicDir, "Artist - Title.mp3"))
	assert.True(t, os.IsNotExist(err))
}

func TestCmdUnsyncEmpty(t *testing.T) {
	musicDir, _ := syncEnv(t, nil, webScript{})
	writeTaggedFile(t, filepath.Join(musicDir, "Artist - Title.mp3"), "123")

	// testing: bare unsync prints that there is nothing to do, exits
	// zero, and changes no file
	output := captureStdout(t, func() {
		assert.Nil(t, sys.ErrOnly(testExecute(cmdUnsync(), "-o", musicDir)))
	})
	assert.Contains(t, output, "nothing to do")
	_, err := os.Stat(filepath.Join(musicDir, "Artist - Title.mp3"))
	assert.Nil(t, err)
}

func TestCmdUnsyncPathFailure(t *testing.T) {
	resetState(t)
	seedSession(t)
	fakeBinDir(t)
	installWeb(t, nil, webScript{})

	// testing: the output path does not exist, the chdir fails
	missing := filepath.Join(t.TempDir(), "missing")
	assert.NotNil(t, sys.ErrOnly(testExecute(cmdUnsync(), "-o", missing, "-t", "123")))
}

func TestCmdUnsyncPlaylistFailure(t *testing.T) {
	musicDir, _ := syncEnv(t, map[string]func(*http.Request) (int, string){
		"/v1/playlists/": func(*http.Request) (int, string) { return apiError(500, "ko") },
	}, webScript{})
	writeTaggedFile(t, filepath.Join(musicDir, "Artist - Title.mp3"), "123")

	// testing: a fetch failure deletes nothing
	assert.EqualError(t, sys.ErrOnly(testExecute(cmdUnsync(), "-o", musicDir, "-p", "Mix")), "playlist Mix: ko")
	_, err := os.Stat(filepath.Join(musicDir, "Artist - Title.mp3"))
	assert.Nil(t, err)
}

func TestCmdUnsyncDeletionFailure(t *testing.T) {
	musicDir, _ := syncEnv(t, nil, webScript{})

	// the first track filename is a non-empty directory, so its removal
	// fails even as root; the second track is a regular tagged file
	stuck := filepath.Join(musicDir, "Artist - Title.mp3")
	assert.Nil(t, os.Mkdir(stuck, 0o755))
	assert.Nil(t, os.WriteFile(filepath.Join(stuck, "inner.txt"), []byte("stuck"), 0o644))
	writeTaggedFile(t, filepath.Join(musicDir, "Artist - Title 456.mp3"), "456")

	// testing: the failure is reported, the other track is still
	// deleted, and the exit code is non-zero
	err := sys.ErrOnly(testExecute(cmdUnsync(), "-o", musicDir, "-t", "123", "-t", "456"))
	assert.EqualError(t, err, "unsync failed: 1 deletions failed")
	_, statErr := os.Stat(filepath.Join(musicDir, "Artist - Title 456.mp3"))
	assert.True(t, os.IsNotExist(statErr))
	_, statErr = os.Stat(stuck)
	assert.Nil(t, statErr)
}

func TestCmdUnsyncDryRunPlaylist(t *testing.T) {
	musicDir, _ := syncEnv(t, nil, webScript{})
	writeTaggedFile(t, filepath.Join(musicDir, "Artist - Title.mp3"), "123")
	assert.Nil(t, os.WriteFile(filepath.Join(musicDir, "Mix.m3u"), []byte("#EXTM3U\n"), 0o644))
	assert.Nil(t, os.WriteFile(filepath.Join(musicDir, "Mix.pls"), []byte("[playlist]\n"), 0o644))

	// testing: the playlist tracks and both playlist files are listed
	// as would-be-removed and nothing is removed
	output := captureStdout(t, func() {
		assert.Nil(t, sys.ErrOnly(testExecute(cmdUnsync(), "--dry-run", "-o", musicDir, "-p", "Mix")))
	})
	assert.Contains(t, output, "would delete Title by Artist")
	assert.Contains(t, output, "would remove playlist file Mix.m3u")
	assert.Contains(t, output, "would remove playlist file Mix.pls")
	assert.Contains(t, output, "dry run complete: 1 would be deleted, 0 skipped, 0 absent, 0 already counted, 2 playlist files would be removed")
	_, err := os.Stat(filepath.Join(musicDir, "Artist - Title.mp3"))
	assert.Nil(t, err)
	_, err = os.Stat(filepath.Join(musicDir, "Mix.m3u"))
	assert.Nil(t, err)
	_, err = os.Stat(filepath.Join(musicDir, "Mix.pls"))
	assert.Nil(t, err)
}

func TestCmdUnsyncDryRunPlaylistOneEncoding(t *testing.T) {
	musicDir, _ := syncEnv(t, nil, webScript{})
	writeTaggedFile(t, filepath.Join(musicDir, "Artist - Title.mp3"), "123")
	assert.Nil(t, os.WriteFile(filepath.Join(musicDir, "Mix.m3u"), []byte("#EXTM3U\n"), 0o644))

	// testing: a playlist file in one encoding only reports without
	// error, and the missing encoding is not reported
	output := captureStdout(t, func() {
		assert.Nil(t, sys.ErrOnly(testExecute(cmdUnsync(), "--dry-run", "-o", musicDir, "-p", "Mix")))
	})
	assert.Contains(t, output, "would remove playlist file Mix.m3u")
	assert.NotContains(t, output, "Mix.pls")
	_, err := os.Stat(filepath.Join(musicDir, "Artist - Title.mp3"))
	assert.Nil(t, err)
	_, err = os.Stat(filepath.Join(musicDir, "Mix.m3u"))
	assert.Nil(t, err)
}

func TestCmdUnsyncDryRunAlbum(t *testing.T) {
	musicDir, _ := syncEnv(t, nil, webScript{})
	writeTaggedFile(t, filepath.Join(musicDir, "Artist - Title.mp3"), "123")
	assert.Nil(t, os.WriteFile(filepath.Join(musicDir, "Mix.m3u"), []byte("#EXTM3U\n"), 0o644))

	// testing: the album track reports as would-be-deleted, no
	// playlist file is reported, and every file stays in place
	output := captureStdout(t, func() {
		assert.Nil(t, sys.ErrOnly(testExecute(cmdUnsync(), "--dry-run", "-o", musicDir, "-a", "album123")))
	})
	assert.Contains(t, output, "would delete Title by Artist")
	assert.NotContains(t, output, "would remove playlist file")
	_, err := os.Stat(filepath.Join(musicDir, "Artist - Title.mp3"))
	assert.Nil(t, err)
	_, err = os.Stat(filepath.Join(musicDir, "Mix.m3u"))
	assert.Nil(t, err)
}

func TestCmdUnsyncDryRunTrackMismatch(t *testing.T) {
	musicDir, _ := syncEnv(t, nil, webScript{})
	writeTaggedFile(t, filepath.Join(musicDir, "Artist - Title.mp3"), "other-id")

	// testing: a same-name file with a mismatching identifier is
	// skipped and survives the dry run
	output := captureStdout(t, func() {
		assert.Nil(t, sys.ErrOnly(testExecute(cmdUnsync(), "--dry-run", "-o", musicDir, "-t", "123")))
	})
	assert.Contains(t, output, "skipping Title by Artist: spotify ID mismatch")
	assert.NotContains(t, output, "would delete")
	_, err := os.Stat(filepath.Join(musicDir, "Artist - Title.mp3"))
	assert.Nil(t, err)
}

func TestCmdUnsyncDryRunTrackTwice(t *testing.T) {
	musicDir, _ := syncEnv(t, nil, webScript{})
	writeTaggedFile(t, filepath.Join(musicDir, "Artist - Title.mp3"), "123")
	assert.Nil(t, os.WriteFile(filepath.Join(musicDir, "Mix.m3u"), []byte("#EXTM3U\n"), 0o644))
	assert.Nil(t, os.WriteFile(filepath.Join(musicDir, "Mix.pls"), []byte("[playlist]\n"), 0o644))

	// testing: a track in two requested collections reports "would
	// delete" once and "already counted" once, and the file survives
	output := captureStdout(t, func() {
		assert.Nil(t, sys.ErrOnly(testExecute(cmdUnsync(), "--dry-run", "-o", musicDir, "-t", "123", "-p", "Mix")))
	})
	assert.Contains(t, output, "would delete Title by Artist")
	assert.Contains(t, output, "already counted Title by Artist")
	assert.Contains(t, output, "dry run complete: 1 would be deleted, 0 skipped, 0 absent, 1 already counted, 2 playlist files would be removed")
	_, err := os.Stat(filepath.Join(musicDir, "Artist - Title.mp3"))
	assert.Nil(t, err)
	_, err = os.Stat(filepath.Join(musicDir, "Mix.m3u"))
	assert.Nil(t, err)
	_, err = os.Stat(filepath.Join(musicDir, "Mix.pls"))
	assert.Nil(t, err)
}

func TestCmdUnsyncDryRunEmpty(t *testing.T) {
	musicDir, _ := syncEnv(t, nil, webScript{})
	writeTaggedFile(t, filepath.Join(musicDir, "Artist - Title.mp3"), "123")

	// testing: bare unsync with dry-run prints that there is nothing
	// to do, exits zero, and changes no file
	output := captureStdout(t, func() {
		assert.Nil(t, sys.ErrOnly(testExecute(cmdUnsync(), "--dry-run", "-o", musicDir)))
	})
	assert.Contains(t, output, "nothing to do")
	_, err := os.Stat(filepath.Join(musicDir, "Artist - Title.mp3"))
	assert.Nil(t, err)
}

func TestCmdUnsyncDryRunPlaylistFailure(t *testing.T) {
	musicDir, _ := syncEnv(t, map[string]func(*http.Request) (int, string){
		"/v1/playlists/": func(*http.Request) (int, string) { return apiError(500, "ko") },
	}, webScript{})
	writeTaggedFile(t, filepath.Join(musicDir, "Artist - Title.mp3"), "123")

	// testing: a fetch failure in a dry run errors with nothing deleted
	assert.EqualError(t, sys.ErrOnly(testExecute(cmdUnsync(), "--dry-run", "-o", musicDir, "-p", "Mix")), "playlist Mix: ko")
	_, err := os.Stat(filepath.Join(musicDir, "Artist - Title.mp3"))
	assert.Nil(t, err)
}

func TestCmdUnsyncDryRunDeletionFailure(t *testing.T) {
	musicDir, _ := syncEnv(t, nil, webScript{})

	// the track filename is a non-empty directory, so its removal
	// would fail even as root
	stuck := filepath.Join(musicDir, "Artist - Title.mp3")
	assert.Nil(t, os.Mkdir(stuck, 0o755))
	assert.Nil(t, os.WriteFile(filepath.Join(stuck, "inner.txt"), []byte("stuck"), 0o644))

	// testing: the dry run reports "would delete", exits zero, and
	// the directory stays intact
	output := captureStdout(t, func() {
		assert.Nil(t, sys.ErrOnly(testExecute(cmdUnsync(), "--dry-run", "-o", musicDir, "-t", "123")))
	})
	assert.Contains(t, output, "would delete Title by Artist")
	_, err := os.Stat(stuck)
	assert.Nil(t, err)
}

func TestCmdUnsyncTargetedLookup(t *testing.T) {
	musicDir, _ := syncEnv(t, nil, webScript{})
	writeTaggedFile(t, filepath.Join(musicDir, "Artist - Title.mp3"), "123")
	writeTaggedFile(t, filepath.Join(musicDir, "Artist - Title 456.mp3"), "456")
	writeTaggedFile(t, filepath.Join(musicDir, "Artist - Title 789.mp3"), "789")
	assert.Nil(t, os.WriteFile(filepath.Join(musicDir, "garbage.mp3"), []byte("garbage"), 0o644))

	// testing: the target file is deleted, every unrelated file stays
	// in place, and the output holds no indexing line
	output := captureStdout(t, func() {
		assert.Nil(t, sys.ErrOnly(testExecute(cmdUnsync(), "-o", musicDir, "-t", "123")))
	})
	assert.NotContains(t, output, "indexing library")
	assert.NotContains(t, output, "indexed ")
	_, err := os.Stat(filepath.Join(musicDir, "Artist - Title.mp3"))
	assert.True(t, os.IsNotExist(err))
	_, err = os.Stat(filepath.Join(musicDir, "Artist - Title 456.mp3"))
	assert.Nil(t, err)
	_, err = os.Stat(filepath.Join(musicDir, "Artist - Title 789.mp3"))
	assert.Nil(t, err)
	_, err = os.Stat(filepath.Join(musicDir, "garbage.mp3"))
	assert.Nil(t, err)
}

func TestCmdUnsyncPlaylistFilePrintsOnce(t *testing.T) {
	musicDir, _ := syncEnv(t, nil, webScript{})
	writeTaggedFile(t, filepath.Join(musicDir, "Artist - Title.mp3"), "123")
	assert.Nil(t, os.WriteFile(filepath.Join(musicDir, "Mix.m3u"), []byte("#EXTM3U\n"), 0o644))
	assert.Nil(t, os.WriteFile(filepath.Join(musicDir, "Mix.pls"), []byte("[playlist]\n"), 0o644))

	// testing: each playlist file line prints exactly once in real mode
	output := captureStdout(t, func() {
		assert.Nil(t, sys.ErrOnly(testExecute(cmdUnsync(), "-o", musicDir, "-p", "Mix")))
	})
	assert.Equal(t, 1, strings.Count(output, "removed playlist file Mix.m3u"))
	assert.Equal(t, 1, strings.Count(output, "removed playlist file Mix.pls"))
	assert.Contains(t, output, "unsync complete: 1 deleted, 0 skipped, 0 absent, 0 already deleted, 2 playlist files removed")
}

func TestCmdUnsyncDryRunPlaylistFilePrintsOnce(t *testing.T) {
	musicDir, _ := syncEnv(t, nil, webScript{})
	writeTaggedFile(t, filepath.Join(musicDir, "Artist - Title.mp3"), "123")
	assert.Nil(t, os.WriteFile(filepath.Join(musicDir, "Mix.m3u"), []byte("#EXTM3U\n"), 0o644))
	assert.Nil(t, os.WriteFile(filepath.Join(musicDir, "Mix.pls"), []byte("[playlist]\n"), 0o644))

	// testing: each playlist file line prints exactly once in dry-run mode
	output := captureStdout(t, func() {
		assert.Nil(t, sys.ErrOnly(testExecute(cmdUnsync(), "--dry-run", "-o", musicDir, "-p", "Mix")))
	})
	assert.Equal(t, 1, strings.Count(output, "would remove playlist file Mix.m3u"))
	assert.Equal(t, 1, strings.Count(output, "would remove playlist file Mix.pls"))
}

func TestCmdUnsyncSameNameTwice(t *testing.T) {
	musicDir, _ := syncEnv(t, map[string]func(*http.Request) (int, string){
		"/v1/albums/": func(*http.Request) (int, string) {
			return 200, fmt.Sprintf(`{"id":"album123","name":"Album","artists":[{"name":"Artist"}],`+
				`"release_date":"1970","tracks":{"items":[%s,%s],"next":null,"total":2}}`,
				simpleTrackJSON("123"), simpleTrackJSON("456"))
		},
	}, webScript{})
	writeTaggedFile(t, filepath.Join(musicDir, "Artist - Title.mp3"), "123")

	// testing: the first occurrence deletes the file, the second reports
	// absent, and no deletion failure is reported
	output := captureStdout(t, func() {
		assert.Nil(t, sys.ErrOnly(testExecute(cmdUnsync(), "-o", musicDir, "-a", "album123")))
	})
	assert.Contains(t, output, "deleted Title by Artist")
	assert.Contains(t, output, "absent Title by Artist")
	assert.NotContains(t, output, "deletion failed")
	_, err := os.Stat(filepath.Join(musicDir, "Artist - Title.mp3"))
	assert.True(t, os.IsNotExist(err))
}

// orderedDeletedLines runs unsync against a scripted three-track
// playlist and returns the deleted lines in output order.
func orderedDeletedLines(t *testing.T, overrides map[string]func(*http.Request) (int, string)) []string {
	t.Helper()
	musicDir, _ := syncEnv(t, overrides, webScript{})
	writeTaggedFile(t, filepath.Join(musicDir, "Artist - Title.mp3"), "123")
	writeTaggedFile(t, filepath.Join(musicDir, "Artist - Title 456.mp3"), "456")
	writeTaggedFile(t, filepath.Join(musicDir, "Artist - Title 789.mp3"), "789")
	output := captureStdout(t, func() {
		assert.Nil(t, sys.ErrOnly(testExecute(cmdUnsync(), "-o", musicDir, "-p", "Mix")))
	})
	var deleted []string
	for _, line := range strings.Split(output, "\n") {
		if strings.HasPrefix(line, "deleted ") {
			deleted = append(deleted, line)
		}
	}
	return deleted
}

// countLines counts the lines exactly equal to the given line.
func countLines(lines []string, wanted string) int {
	count := 0
	for _, line := range lines {
		if line == wanted {
			count++
		}
	}
	return count
}

func TestCmdUnsyncFetchOrder(t *testing.T) {
	overrides := map[string]func(*http.Request) (int, string){
		"/v1/playlists/": func(*http.Request) (int, string) {
			return 200, fmt.Sprintf(`{"id":"playlist123","name":"Mix",`+
				`"tracks":{"items":[{"track":%s},{"track":%s},{"track":%s}],"next":null,"total":3}}`,
				trackJSON("123"), trackJSON("456"), trackJSON("789"))
		},
	}

	// testing: the deleted lines appear in fetch order, identically
	// across runs
	first := orderedDeletedLines(t, overrides)
	second := orderedDeletedLines(t, overrides)
	assert.Equal(t, []string{
		"deleted Title by Artist",
		"deleted Title 456 by Artist",
		"deleted Title 789 by Artist",
	}, first)
	assert.Equal(t, first, second)

	// testing: a track in two requested collections is deleted once,
	// the second occurrence reports as already deleted
	musicDir, _ := syncEnv(t, nil, webScript{})
	writeTaggedFile(t, filepath.Join(musicDir, "Artist - Title.mp3"), "123")
	output := captureStdout(t, func() {
		assert.Nil(t, sys.ErrOnly(testExecute(cmdUnsync(), "-o", musicDir, "-t", "123", "-p", "Mix")))
	})
	lines := strings.Split(output, "\n")
	assert.Equal(t, 1, countLines(lines, "deleted Title by Artist"))
	assert.Equal(t, 1, countLines(lines, "already deleted Title by Artist"))
}
