package sys

import (
	"github.com/tamnd/ytb-cli/ytb"
)

// YouTubeClient is the one shared ytb client serving every YouTube
// search and download in the process. It is built from DefaultConfig:
// no session is attached, and no disk cache or store is attached.
// The client is safe for concurrent use; the two-slot search
// semaphore in the provider bounds goroutines, the client paces
// requests.
//
// ytb-cli carries an Apache-2.0 license, compatible with pango
// GPL-3.0 as a dependency.
//
// Tests replace this variable with a client built with no pacing
// and a stubbed transport, and restore it on cleanup.
var YouTubeClient = ytb.NewClient(ytb.DefaultConfig())
