package provider

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"math"
	"strconv"
	"strings"

	"github.com/streambinder/pango/entity"
	"github.com/streambinder/pango/sys"
	"github.com/streambinder/pango/sys/cmd"
)

type youTube struct{}

const maxYouTubeConcurrentSearches = 2

// youTubeSearchSlots caps concurrent YouTube searches no matter how
// many decide workers run, keeping 429s and captcha triggers at bay
var youTubeSearchSlots = make(chan struct{}, maxYouTubeConcurrentSearches)

// youTubeSearchEntry is one JSON line of a yt-dlp flat playlist
// search: the video id, title, uploader, description, duration in
// seconds, view count and verified channel flag.
type youTubeSearchEntry struct {
	ID              string `json:"id"`
	Title           string `json:"title"`
	Uploader        string `json:"uploader"`
	Description     string `json:"description"`
	Duration        int    `json:"duration"`
	ViewCount       int    `json:"view_count"`
	ChannelVerified bool   `json:"channel_is_verified"`
}

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

	output, err := cmd.YouTubeDlSearch(context.Background(), query)
	if err != nil {
		return nil, fmt.Errorf("youtube: %w", err)
	}

	return provider.parseResults(track, query, output)
}

func (provider youTube) parseResults(track *entity.Track, query string, output []byte) ([]*Match, error) {
	var matches []*Match
	for _, line := range bytes.Split(output, []byte("\n")) {
		line = bytes.TrimSpace(line)
		if len(line) == 0 {
			continue
		}
		var entry youTubeSearchEntry
		if err := json.Unmarshal(line, &entry); err != nil {
			return nil, fmt.Errorf("youtube: cannot parse search result: %w", err)
		}
		match := youTubeResult{
			track: track,
			query: query,
			id:    entry.ID,
			title: entry.Title,
			owner: entry.Uploader,
			// the scraper read the snippet, the opening lines of the
			// description. The full description carries credits where
			// misleading words like "studio" routinely appear, and
			// those would systematically penalize official videos.
			description:     sys.FirstLine(entry.Description),
			views:           entry.ViewCount,
			length:          entry.Duration,
			verifiedChannel: entry.ChannelVerified,
		}
		if match.compliant(track) {
			matches = append(matches, &Match{fmt.Sprintf("https://youtu.be/%s", match.id), match.score()})
		}
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
