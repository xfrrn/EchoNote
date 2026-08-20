package podcast

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	domain "github.com/Actify/echonote/apps/server/internal/domain/podcast"
)

const appleLookupURL = "https://itunes.apple.com/lookup"

type appleResolver struct {
	client    *http.Client
	lookupURL string
}

type appleLookupResponse struct {
	Results []appleLookupResult `json:"results"`
}

type appleLookupResult struct {
	Kind            string `json:"kind"`
	TrackID         int64  `json:"trackId"`
	CollectionID    int64  `json:"collectionId"`
	TrackName       string `json:"trackName"`
	CollectionName  string `json:"collectionName"`
	ArtistName      string `json:"artistName"`
	Description     string `json:"description"`
	FeedURL         string `json:"feedUrl"`
	EpisodeURL      string `json:"episodeUrl"`
	EpisodeGUID     string `json:"episodeGuid"`
	ReleaseDate     string `json:"releaseDate"`
	TrackTimeMillis int64  `json:"trackTimeMillis"`
	ArtworkURL600   string `json:"artworkUrl600"`
	TrackViewURL    string `json:"trackViewUrl"`
}

func newAppleResolver(client *http.Client) *appleResolver {
	return &appleResolver{client: client, lookupURL: appleLookupURL}
}

func (r *appleResolver) CanResolve(rawURL string) bool {
	parsed, err := url.Parse(strings.TrimSpace(rawURL))
	if err != nil || (parsed.Scheme != "http" && parsed.Scheme != "https") {
		return false
	}
	host := strings.ToLower(parsed.Hostname())
	return host == "podcasts.apple.com" || host == "itunes.apple.com"
}

func (r *appleResolver) Resolve(ctx context.Context, rawURL string) (*domain.ResolvedEpisode, error) {
	parsed, err := parsePublicURL(rawURL)
	if err != nil || !r.CanResolve(rawURL) {
		return nil, domain.NewResolveError("IMPORT_INVALID_APPLE_URL", "Apple Podcasts URL is invalid", false, err)
	}
	showID, episodeID, err := appleIDs(parsed)
	if err != nil {
		return nil, domain.NewResolveError("IMPORT_INVALID_APPLE_URL", err.Error(), false, nil)
	}

	lookup, err := url.Parse(r.lookupURL)
	if err != nil {
		return nil, domain.NewResolveError("APPLE_LOOKUP_FAILED", "Apple Podcasts lookup is unavailable", true, err)
	}
	query := lookup.Query()
	query.Set("id", showID)
	query.Set("entity", "podcastEpisode")
	query.Set("limit", "200")
	lookup.RawQuery = query.Encode()

	request, err := newRequest(ctx, http.MethodGet, lookup.String())
	if err != nil {
		return nil, domain.NewResolveError("APPLE_LOOKUP_FAILED", "Apple Podcasts lookup is unavailable", true, err)
	}
	response, err := r.client.Do(request)
	if err != nil {
		return nil, domain.NewResolveError("APPLE_LOOKUP_FAILED", "Apple Podcasts lookup is unavailable", true, err)
	}
	defer response.Body.Close()
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		retryable := response.StatusCode == http.StatusTooManyRequests || response.StatusCode >= 500
		return nil, domain.NewResolveError("APPLE_LOOKUP_FAILED", "Apple Podcasts lookup failed", retryable, nil)
	}
	body, err := readLimited(response.Body)
	if err != nil {
		return nil, domain.NewResolveError("APPLE_LOOKUP_FAILED", "Apple Podcasts lookup returned invalid data", true, err)
	}
	var lookupResponse appleLookupResponse
	if err := json.Unmarshal(body, &lookupResponse); err != nil {
		return nil, domain.NewResolveError("APPLE_LOOKUP_FAILED", "Apple Podcasts lookup returned invalid data", true, err)
	}

	var show *appleLookupResult
	var episode *appleLookupResult
	for index := range lookupResponse.Results {
		result := &lookupResponse.Results[index]
		if result.Kind == "podcast" && strconv.FormatInt(result.CollectionID, 10) == showID {
			show = result
		}
		if result.Kind == "podcast-episode" && strconv.FormatInt(result.TrackID, 10) == episodeID {
			episode = result
		}
	}
	if episode == nil {
		return nil, domain.NewResolveError("IMPORT_EPISODE_NOT_FOUND", "Apple Podcasts episode was not found", false, nil)
	}
	if strings.TrimSpace(episode.TrackName) == "" {
		return nil, domain.NewResolveError("APPLE_LOOKUP_FAILED", "Apple Podcasts returned an episode without a title", false, nil)
	}
	if _, err := parsePublicURL(episode.EpisodeURL); err != nil {
		return nil, domain.NewResolveError("IMPORT_AUDIO_URL_INVALID", "Apple Podcasts returned an invalid audio URL", false, err)
	}

	feedURL := episode.FeedURL
	podcastTitle := episode.CollectionName
	podcastAuthor := episode.ArtistName
	podcastDescription := ""
	coverURL := episode.ArtworkURL600
	if show != nil {
		if show.FeedURL != "" {
			feedURL = show.FeedURL
		}
		if show.CollectionName != "" {
			podcastTitle = show.CollectionName
		}
		if show.ArtistName != "" {
			podcastAuthor = show.ArtistName
		}
		podcastDescription = show.Description
		if show.ArtworkURL600 != "" {
			coverURL = show.ArtworkURL600
		}
	}
	if feedURL != "" {
		if _, err := parsePublicURL(feedURL); err != nil {
			return nil, domain.NewResolveError("IMPORT_FEED_URL_INVALID", "Apple Podcasts returned an invalid RSS URL", false, err)
		}
	}

	canonicalURL := episode.TrackViewURL
	if canonicalURL == "" {
		canonicalURL = parsed.String()
	}
	var publishedAt *time.Time
	if parsedTime, parseErr := time.Parse(time.RFC3339, episode.ReleaseDate); parseErr == nil {
		publishedAt = &parsedTime
	}
	return &domain.ResolvedEpisode{
		SourceType:         domain.SourceApple,
		ExternalID:         episodeID,
		RSSGUID:            strings.TrimSpace(episode.EpisodeGUID),
		PodcastTitle:       strings.TrimSpace(podcastTitle),
		PodcastAuthor:      strings.TrimSpace(podcastAuthor),
		PodcastDescription: strings.TrimSpace(podcastDescription),
		PodcastCoverURL:    strings.TrimSpace(coverURL),
		EpisodeTitle:       strings.TrimSpace(episode.TrackName),
		Description:        strings.TrimSpace(episode.Description),
		PublishedAt:        publishedAt,
		DurationMS:         max(episode.TrackTimeMillis, 0),
		CanonicalURL:       canonicalURL,
		FeedURL:            feedURL,
		AudioURL:           episode.EpisodeURL,
	}, nil
}

func appleIDs(parsed *url.URL) (string, string, error) {
	var showID string
	for _, segment := range strings.Split(parsed.Path, "/") {
		if strings.HasPrefix(segment, "id") {
			candidate := strings.TrimPrefix(segment, "id")
			if _, err := strconv.ParseUint(candidate, 10, 64); err == nil {
				showID = candidate
			}
		}
	}
	episodeID := parsed.Query().Get("i")
	if showID == "" {
		return "", "", fmt.Errorf("Apple Podcasts URL has no show ID")
	}
	if _, err := strconv.ParseUint(episodeID, 10, 64); err != nil {
		return "", "", fmt.Errorf("Apple Podcasts URL must identify one episode with the i parameter")
	}
	return showID, episodeID, nil
}
