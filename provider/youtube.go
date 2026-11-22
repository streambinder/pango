package provider

import (
	"context"
	"fmt"
	"math"
	"sort"
	"strconv"
	"strings"

	"github.com/tamnd/any-cli/kit/errs"
	"github.com/tamnd/ytb-cli/ytb"

	"github.com/streambinder/pango/entity"
	"github.com/streambinder/pango/sys"
)

type youTube struct{}

const maxYouTubeConcurrentSearches = 2

// youTubeSearchSlots caps concurrent YouTube searches no matter how
// many decide workers run, keeping 429s and captcha triggers at bay
var youTubeSearchSlots = make(chan struct{}, maxYouTubeConcurrentSearches)

// maxYouTubeSearchResults is the number of video rows a search
// collects before halting the stream.
const maxYouTubeSearchResults = 10

// maxYouTubeFinalists is the number of top compliant results whose
// channels are read to restore the verified badge.
const maxYouTubeFinalists = 3

// youTubeResult is one video from the search: the video id, title,
// owner, description, duration in seconds, view count, verified
// channel flag, and the channel id the badge fetch reads.
type youTubeResult struct {
	track           *entity.Track
	query           string
	id              string
	title           string
	owner           string
	description     string
	views           int
	length          int
	verifiedChannel bool
	channelID       string
}

func init() {
	providers = append(providers, youTube{})
}

func sanitizeYouTubeQuery(q string) string {
	// YouTube interprets -word as exclusion operator, filter out hyphens
	// to avoid unintended query filtering
	q = strings.ReplaceAll(q, "-", " ")
	return strings.Join(strings.Fields(q), " ")
}

func (provider youTube) search(track *entity.Track) ([]*Match, error) {
	query := track.Title
	for _, artist := range track.Artists {
		query = fmt.Sprintf("%s %s", query, artist)
	}
	query = sanitizeYouTubeQuery(query)

	youTubeSearchSlots <- struct{}{}
	defer func() { <-youTubeSearchSlots }()

	var results []youTubeResult
	err := sys.YouTubeClient.Search(
		context.Background(),
		query,
		ytb.SearchFilters{Type: "video"},
		ytb.PageOptions{MaxPages: 1},
		func(item any) error {
			video, ok := item.(ytb.Video)
			if !ok {
				return nil
			}
			results = append(results, youTubeResult{
				track: track,
				query: query,
				id:    video.VideoID,
				title: video.Title,
				owner: video.ChannelTitle,
				// the search row carries the snippet, the opening lines
				// of the description. The full description carries
				// credits where misleading words like "studio"
				// routinely appear, and those would systematically
				// penalize official videos.
				description: sys.FirstLine(video.Description),
				views:       int(video.ViewCount),
				length:      video.DurationSeconds,
				channelID:   video.ChannelID,
			})
			if len(results) >= maxYouTubeSearchResults {
				return ytb.ErrStop
			}
			return nil
		},
	)
	if err != nil {
		// a search with no results is an empty result, not an error
		if errs.KindOf(err) == errs.KindNoResults {
			return nil, nil
		}
		return nil, fmt.Errorf("youtube: %w", err)
	}

	return provider.rank(track, results)
}

// rank filters the compliant results, restores the verified badge
// for the top finalists, and returns the matches sorted by score.
func (provider youTube) rank(track *entity.Track, results []youTubeResult) ([]*Match, error) {
	var compliant []youTubeResult
	for _, result := range results {
		if result.compliant(track) {
			compliant = append(compliant, result)
		}
	}
	sort.SliceStable(compliant, func(i, j int) bool {
		return compliant[i].score() > compliant[j].score()
	})

	// the search row carries no badge, so the verified flag comes
	// from one channel read per distinct channel id among the top
	// finalists. A read that fails leaves the channel part at zero
	// and never fails the search.
	seen := map[string]bool{}
	for i := range compliant {
		if i >= maxYouTubeFinalists {
			break
		}
		channelID := compliant[i].channelID
		if channelID == "" || seen[channelID] {
			continue
		}
		seen[channelID] = true
		channel, err := sys.YouTubeClient.FetchChannel(
			context.Background(),
			channelID,
			ytb.ChannelOptions{NoAbout: true},
		)
		if err != nil || channel == nil || !channel.IsVerified {
			continue
		}
		for j := range compliant {
			if compliant[j].channelID == channelID {
				compliant[j].verifiedChannel = true
			}
		}
	}

	sort.SliceStable(compliant, func(i, j int) bool {
		return compliant[i].score() > compliant[j].score()
	})

	matches := make([]*Match, 0, len(compliant))
	for _, result := range compliant {
		matches = append(matches, &Match{fmt.Sprintf("https://youtu.be/%s", result.id), result.score()})
	}
	return matches, nil
}

// compliance check works as a barrier before checking on the result score
// so to ensure that only the results that pass certain pre-checks get returned
func (result youTubeResult) compliant(track *entity.Track) bool {
	spec := sys.UniqueFields(fmt.Sprintf("%s %s", result.owner, result.title))
	return result.id != "" &&
		sys.Contains(spec, strings.Split(sys.UniqueFields(track.Artist()), " ")...) &&
		sys.Contains(spec, strings.Split(sys.UniqueFields(track.Song()), " ")...)
}

// score goes from 0 to 100:
//
//	0–40% is derived from description score
//	0-30% is derived from duration score
//	0-15% is derived from views score
//	0-15% is derived from channel credibility score
func (result youTubeResult) score() int {
	var (
		descriptionScore = result.descriptionScore() * 40 / 100
		durationScore    = result.durationScore() * 30 / 100
		viewsScore       = result.viewsScore() * 15 / 100
		channelScore     = result.channelScore() * 15 / 100
	)
	return descriptionScore + durationScore + viewsScore + channelScore
}

// return a score for result description fields (i.e. owner, title, description)
func (result youTubeResult) descriptionScore() int {
	var (
		shortDescription = fmt.Sprintf("%s %s", result.title, result.owner)
		longDescription  = sys.Flatten(fmt.Sprintf("%s %s", shortDescription, result.description))
	)
	distance := sys.LevenshteinBoundedDistance(result.query, shortDescription)
	for _, word := range misleading {
		if sys.Contains(longDescription, word) && !sys.Contains(result.query, word) {
			distance += 30
		}
	}

	// return the inverse of the proportion of the distance
	// on a percentage scale to 50
	return 100 - int(math.Min(float64(distance), 50.0)*100/50)
}

// return a score for result duration
func (result youTubeResult) durationScore() int {
	distance := int(math.Min(math.Abs(float64(result.length)-float64(result.track.Duration)), 60.0))
	// boost results with super close duration delta
	if distance < 5 {
		distance = 0
	}
	// return the inverse of the proportion of the distance
	// on a percentage scale to 60
	return 100 - (distance * 100 / 60)
}

// return a score for result's number of views
//
//	ie percentage of the number of digits of views on a scale to 11
//	(11 digits is for views of the order of 10.000.000.000, the highest reached on YouTube so far)
func (result youTubeResult) viewsScore() int {
	digits := len(strconv.Itoa(result.views))
	// boost results with more than a million views
	if digits > 6 {
		digits += 2
	}
	return int(math.Min(float64(digits), 11.0)) * 100 / 11
}

// return a score for result's channel according to its badges
//
//	0-30% is assigned if it's verified. The official-artist badge
//	is not part of the score: it never appears in search data, so
//	there is nothing to check against.
func (result youTubeResult) channelScore() int {
	return sys.Ternary(result.verifiedChannel, 30, 0)
}
