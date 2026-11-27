package cmd

import (
	"context"
	"fmt"
	"os"
	"sort"

	"github.com/adrg/xdg"
	"github.com/arunsworld/nursery"
	"github.com/spf13/cobra"
	"github.com/streambinder/pango/entity"
	"github.com/streambinder/pango/entity/id3"
	"github.com/streambinder/pango/entity/playlist"
	"github.com/streambinder/pango/sys"
)

// unsyncParserWorkers is the parser pool size, the same count as sync's
// decide workers
const unsyncParserWorkers = 4

func init() {
	cmdRoot.AddCommand(cmdUnsync())
}

// unsyncFetchOutcome carries the result of the fetch stage: the fetched
// playlists in fetch order, and the fetch error, if any
type unsyncFetchOutcome struct {
	playlists []*playlist.Playlist
	err       error
}

// unsyncWork is one fetched track numbered by the sequencer in arrival
// order, so the pruner can restore fetch order after concurrent parsing
type unsyncWork struct {
	seq   int
	track *entity.Track
}

// unsyncVerdictKind is the outcome of parsing one fetched track
type unsyncVerdictKind int

const (
	verdictMatch unsyncVerdictKind = iota
	verdictDirectory
	verdictAbsent
	verdictSkip
)

// unsyncVerdict is one parsed track: the track, the resolved filename,
// the stored identifier, the verdict kind, the reason for a skip, and
// the sequence number
type unsyncVerdict struct {
	seq      int
	track    *entity.Track
	filename string
	stored   string
	kind     unsyncVerdictKind
	reason   string
}

// unsyncResults collects the outcome of an unsync run
type unsyncResults struct {
	deleted        int
	skipped        int
	absent         int
	alreadyDeleted int
	failed         int
	playlistFiles  []string
}

func cmdUnsync() *cobra.Command {
	cmd := &cobra.Command{
		Use:          "unsync",
		Short:        "Delete local tracks of given collections",
		SilenceUsage: true,
		Args:         cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			path := sys.ErrWrap(xdg.UserDirs.Music)(cmd.Flags().GetString("output"))
			playlists, albums, tracks := readCollectionFlags(cmd)
			dryRun := sys.ErrWrap(false)(cmd.Flags().GetBool("dry-run"))

			if len(playlists)+len(albums)+len(tracks) == 0 {
				fmt.Println("nothing to do")
				return nil
			}

			if err := os.Chdir(path); err != nil {
				return err
			}

			// the unsync runs as a gated pipeline: the fetcher streams
			// tracks into the parser while it still pages, and the
			// sequential pruner starts only after both queues drained
			// and closed. Nothing is deleted before every fetch
			// succeeded.
			if err := auditAuthenticate(); err != nil {
				return err
			}

			tracksCh := make(chan interface{}, pipelineBuffer)
			workCh := make(chan unsyncWork, pipelineBuffer)
			verdictsCh := make(chan unsyncVerdict, pipelineBuffer)
			fetchResult := make(chan unsyncFetchOutcome, 1)
			parseResult := make(chan error, 1)

			go unsyncFetchStream(playlists, albums, tracks, tracksCh, fetchResult)
			go unsyncSequence(tracksCh, workCh)
			go func() {
				defer close(verdictsCh)
				parseResult <- nursery.RunMultipleCopiesConcurrently(unsyncParserWorkers, func(ctx context.Context, _ chan error) {
					for {
						select {
						case <-ctx.Done():
							return
						case work, ok := <-workCh:
							if !ok {
								return
							}
							verdictsCh <- unsyncParse(work)
						}
					}
				})
			}()

			var parsed []unsyncVerdict
			for verdict := range verdictsCh {
				parsed = append(parsed, verdict)
			}
			outcome := <-fetchResult
			if outcome.err != nil {
				return outcome.err
			}
			if err := <-parseResult; err != nil {
				return err
			}

			sort.SliceStable(parsed, func(i, j int) bool { return parsed[i].seq < parsed[j].seq })

			results := &unsyncResults{}
			deleted := map[string]bool{}
			for _, verdict := range parsed {
				unsyncPrune(verdict, deleted, results, dryRun)
			}
			for _, fetchedPlaylist := range outcome.playlists {
				unsyncDeletePlaylistFiles(fetchedPlaylist, results, dryRun)
			}

			if dryRun {
				fmt.Printf("dry run complete: %d would be deleted, %d skipped, %d absent, %d already counted, %d playlist files would be removed\n",
					results.deleted, results.skipped, results.absent, results.alreadyDeleted, len(results.playlistFiles))
			} else {
				fmt.Printf("unsync complete: %d deleted, %d skipped, %d absent, %d already deleted, %d playlist files removed\n",
					results.deleted, results.skipped, results.absent, results.alreadyDeleted, len(results.playlistFiles))
			}
			if results.failed > 0 {
				return fmt.Errorf("unsync failed: %d deletions failed", results.failed)
			}
			return nil
		},
	}
	cmd.Flags().StringP("output", "o", xdg.UserDirs.Music, "Output synchronization path")
	cmd.Flags().StringArrayP("playlist", "p", []string{}, "Unsync playlist")
	cmd.Flags().StringArrayP("album", "a", []string{}, "Unsync album")
	cmd.Flags().StringArrayP("track", "t", []string{}, "Unsync track")
	cmd.Flags().Bool("dry-run", false, "Simulate the unsync without removing anything")
	return cmd
}

// unsyncFetchStream fetches every requested collection sequentially and
// streams each track into the tracks channel as it arrives; it closes the
// channel when done, even on error, and reports the outcome on the result
// channel. The parsed verdicts are discarded when the fetch fails, so
// nothing is deleted before every fetch succeeded.
func unsyncFetchStream(playlists, albums, tracks []string, out chan interface{}, result chan<- unsyncFetchOutcome) {
	defer close(out)
	var fetchedPlaylists []*playlist.Playlist
	fetchErr := func() error {
		for _, id := range albums {
			fmt.Println("fetching album", id)
			if _, err := spotifyClient.Album(id, out); err != nil {
				return fmt.Errorf("album %s: %w", id, err)
			}
		}
		for _, id := range tracks {
			fmt.Println("fetching track", id)
			if _, err := spotifyClient.Track(id, out); err != nil {
				return fmt.Errorf("track %s: %w", id, err)
			}
		}
		for _, name := range playlists {
			fmt.Println("fetching playlist", name)
			fetchedPlaylist, err := spotifyClient.Playlist(name, out)
			if err != nil {
				return fmt.Errorf("playlist %s: %w", name, err)
			}
			fetchedPlaylists = append(fetchedPlaylists, fetchedPlaylist)
		}
		return nil
	}()
	result <- unsyncFetchOutcome{playlists: fetchedPlaylists, err: fetchErr}
}

// unsyncSequence numbers fetched tracks in arrival order and forwards
// them to the parser; it closes the work channel when the tracks channel
// closes. Only the sequencer assigns sequence numbers.
func unsyncSequence(in <-chan interface{}, out chan<- unsyncWork) {
	defer close(out)
	seq := 0
	for event := range in {
		out <- unsyncWork{seq: seq, track: event.(*entity.Track)}
		seq++
	}
}

// unsyncParse resolves the expected local file of one fetched track and
// reads its stored Spotify identifier: it never deletes anything, prints
// nothing, and holds no shared state, so parser workers run concurrently.
// An absent, unreadable, untagged, or mismatching file becomes a verdict
// that the pruner reports as a skip, never a deletion.
func unsyncParse(work unsyncWork) unsyncVerdict {
	track := work.track
	filename := track.Path().Final()
	info, err := os.Stat(filename)
	if err != nil {
		if os.IsNotExist(err) {
			return unsyncVerdict{seq: work.seq, track: track, filename: filename, kind: verdictAbsent}
		}
		return unsyncVerdict{seq: work.seq, track: track, filename: filename, kind: verdictSkip, reason: err.Error()}
	}

	// a directory carrying a track filename is not a track file: the
	// pruner attempts its removal and reports the outcome, so a stuck
	// entry never looks like a clean deletion
	if info.IsDir() {
		return unsyncVerdict{seq: work.seq, track: track, filename: filename, kind: verdictDirectory}
	}

	tag, err := id3.OpenSpotifyID(filename)
	if err != nil {
		return unsyncVerdict{seq: work.seq, track: track, filename: filename, kind: verdictSkip, reason: err.Error()}
	}
	stored := tag.SpotifyID()
	_ = tag.Close()
	if len(stored) == 0 {
		return unsyncVerdict{seq: work.seq, track: track, filename: filename, kind: verdictSkip, reason: "no spotify ID set"}
	}
	if stored != track.ID {
		return unsyncVerdict{seq: work.seq, track: track, filename: filename, stored: stored, kind: verdictSkip, reason: "spotify ID mismatch"}
	}

	return unsyncVerdict{seq: work.seq, track: track, filename: filename, stored: stored, kind: verdictMatch}
}

// unsyncPrune applies one parsed verdict in fetch order: it claims the
// identifier at first occurrence, re-stats before it removes, and prints
// every per-track line itself, so the report reads the same on every run.
func unsyncPrune(verdict unsyncVerdict, deleted map[string]bool, results *unsyncResults, dryRun bool) {
	track := verdict.track
	if deleted[track.ID] {
		results.alreadyDeleted++
		if dryRun {
			fmt.Printf("already counted %s by %s\n", track.Title, track.Artist())
		} else {
			fmt.Printf("already deleted %s by %s\n", track.Title, track.Artist())
		}
		return
	}

	switch verdict.kind {
	case verdictAbsent:
		results.absent++
		fmt.Printf("absent %s by %s\n", track.Title, track.Artist())
	case verdictSkip:
		// the file may be gone already: a first same-filename occurrence deleted it
		if unsyncCheckGone(track, verdict.filename, results) {
			return
		}
		results.skipped++
		fmt.Printf("skipping %s by %s: %s\n", track.Title, track.Artist(), verdict.reason)
	case verdictDirectory, verdictMatch:
		unsyncRemoveFile(track, verdict.filename, deleted, results, dryRun)
	}
}

// unsyncCheckGone reports whether the file is gone or unreadable at prune
// time, printing the absent or skip line. It returns true when the caller
// must stop with this verdict.
func unsyncCheckGone(track *entity.Track, filename string, results *unsyncResults) bool {
	if _, err := os.Stat(filename); err != nil {
		if os.IsNotExist(err) {
			results.absent++
			fmt.Printf("absent %s by %s\n", track.Title, track.Artist())
			return true
		}
		results.skipped++
		fmt.Printf("skipping %s by %s: %s\n", track.Title, track.Artist(), err)
		return true
	}
	return false
}

// unsyncRemoveFile removes one local file and records the outcome: a
// failure is reported and the run continues. It re-stats before it
// removes, so a file already gone reports absent instead of a deletion
// failure. In dry-run mode it records the would-be deletion and returns
// without removing the file.
func unsyncRemoveFile(track *entity.Track, filename string, deleted map[string]bool, results *unsyncResults, dryRun bool) {
	if dryRun {
		deleted[track.ID] = true
		results.deleted++
		fmt.Printf("would delete %s by %s\n", track.Title, track.Artist())
		return
	}
	if unsyncCheckGone(track, filename, results) {
		return
	}
	if err := os.Remove(filename); err != nil {
		if os.IsNotExist(err) {
			results.absent++
			fmt.Printf("absent %s by %s\n", track.Title, track.Artist())
			return
		}
		results.failed++
		fmt.Printf("deletion failed for %s by %s: %s\n", track.Title, track.Artist(), err)
		return
	}
	deleted[track.ID] = true
	results.deleted++
	fmt.Printf("deleted %s by %s\n", track.Title, track.Artist())
}

// unsyncDeletePlaylistFiles removes the generated playlist files of
// one fetched playlist, in both encodings when present: a missing file
// is not an error. Each line prints once, here at the removal site. In
// dry-run mode it reports the files that would be removed and removes none.
func unsyncDeletePlaylistFiles(fetchedPlaylist *playlist.Playlist, results *unsyncResults, dryRun bool) {
	for _, encoding := range []string{"m3u", "pls"} {
		filename := sys.LegalizeFilename(fetchedPlaylist.Name) + "." + encoding
		if dryRun {
			if _, err := os.Stat(filename); err != nil {
				if os.IsNotExist(err) {
					continue
				}
				fmt.Printf("skipping playlist file %s: %s\n", filename, err)
				continue
			}
			results.playlistFiles = append(results.playlistFiles, filename)
			fmt.Printf("would remove playlist file %s\n", filename)
			continue
		}
		if err := os.Remove(filename); err != nil {
			if os.IsNotExist(err) {
				continue
			}
			results.failed++
			fmt.Printf("deletion failed for playlist file %s: %s\n", filename, err)
			continue
		}
		results.playlistFiles = append(results.playlistFiles, filename)
		fmt.Printf("removed playlist file %s\n", filename)
	}
}
