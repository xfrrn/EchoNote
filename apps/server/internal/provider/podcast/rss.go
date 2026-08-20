package podcast

import (
	"context"
	"encoding/xml"
	"mime"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	domain "github.com/Actify/echonote/apps/server/internal/domain/podcast"
)

type rssResolver struct {
	client *http.Client
}

type rssDocument struct {
	XMLName xml.Name   `xml:"rss"`
	Channel rssChannel `xml:"channel"`
}

type rssChannel struct {
	Title       string    `xml:"title"`
	Author      string    `xml:"http://www.itunes.com/dtds/podcast-1.0.dtd author"`
	Description string    `xml:"description"`
	Image       rssImage  `xml:"image"`
	ITunesImage rssImage  `xml:"http://www.itunes.com/dtds/podcast-1.0.dtd image"`
	Items       []rssItem `xml:"item"`
}

type rssItem struct {
	Title       string       `xml:"title"`
	Description string       `xml:"description"`
	Content     string       `xml:"http://purl.org/rss/1.0/modules/content/ encoded"`
	Link        string       `xml:"link"`
	GUID        string       `xml:"guid"`
	PublishedAt string       `xml:"pubDate"`
	Duration    string       `xml:"http://www.itunes.com/dtds/podcast-1.0.dtd duration"`
	Image       rssImage     `xml:"http://www.itunes.com/dtds/podcast-1.0.dtd image"`
	Enclosure   rssEnclosure `xml:"enclosure"`
}

type rssImage struct {
	URL  string `xml:"url"`
	Href string `xml:"href,attr"`
}

type rssEnclosure struct {
	URL  string `xml:"url,attr"`
	Type string `xml:"type,attr"`
}

func newRSSResolver(client *http.Client) *rssResolver {
	return &rssResolver{client: client}
}

func (r *rssResolver) CanResolve(rawURL string) bool {
	_, err := parsePublicURL(rawURL)
	return err == nil
}

func (r *rssResolver) Resolve(ctx context.Context, rawURL string) (*domain.ResolvedEpisode, error) {
	parsed, err := parsePublicURL(rawURL)
	if err != nil {
		return nil, domain.NewResolveError("IMPORT_INVALID_URL", "URL is invalid", false, err)
	}
	request, err := newRequest(ctx, http.MethodGet, parsed.String())
	if err != nil {
		return nil, domain.NewResolveError("IMPORT_RSS_FETCH_FAILED", "RSS feed is unavailable", true, err)
	}
	request.Header.Set("Accept", "application/rss+xml, application/xml, text/xml;q=0.9, */*;q=0.1")
	response, err := r.client.Do(request)
	if err != nil {
		return nil, domain.NewResolveError("IMPORT_RSS_FETCH_FAILED", "RSS feed is unavailable", true, err)
	}
	defer response.Body.Close()
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		retryable := response.StatusCode == http.StatusTooManyRequests || response.StatusCode >= 500
		return nil, domain.NewResolveError("IMPORT_RSS_FETCH_FAILED", "RSS feed could not be fetched", retryable, nil)
	}
	if strings.HasPrefix(strings.ToLower(response.Header.Get("Content-Type")), "audio/") {
		return nil, domain.ErrNotApplicable
	}
	body, err := readLimited(response.Body)
	if err != nil {
		return nil, domain.NewResolveError("IMPORT_RSS_INVALID", "RSS feed is too large or unreadable", false, err)
	}
	var document rssDocument
	if err := xml.Unmarshal(body, &document); err != nil || document.XMLName.Local != "rss" {
		if isXMLContentType(response.Header.Get("Content-Type")) {
			return nil, domain.NewResolveError("IMPORT_RSS_INVALID", "RSS feed is invalid", false, err)
		}
		return nil, domain.ErrNotApplicable
	}

	item, publishedAt := newestRSSItem(document.Channel.Items)
	if item == nil {
		return nil, domain.NewResolveError("IMPORT_EPISODE_NOT_FOUND", "RSS feed has no audio episode", false, nil)
	}
	feedURL := response.Request.URL
	audioURL, err := resolvePublicReference(feedURL, item.Enclosure.URL)
	if err != nil {
		return nil, domain.NewResolveError("IMPORT_AUDIO_URL_INVALID", "RSS feed contains an invalid audio URL", false, err)
	}
	if !isAudioResponse(item.Enclosure.Type, audioURL.Path) {
		return nil, domain.NewResolveError("IMPORT_AUDIO_URL_INVALID", "RSS enclosure is not supported audio", false, nil)
	}
	title := strings.TrimSpace(item.Title)
	if title == "" || strings.TrimSpace(document.Channel.Title) == "" {
		return nil, domain.NewResolveError("IMPORT_RSS_INVALID", "RSS feed is missing a podcast or episode title", false, nil)
	}
	canonicalURL := feedURL.String()
	if item.Link != "" {
		if itemURL, resolveErr := resolvePublicReference(feedURL, item.Link); resolveErr == nil {
			canonicalURL = itemURL.String()
		}
	}
	coverURL := firstNonEmpty(item.Image.Href, item.Image.URL, document.Channel.ITunesImage.Href, document.Channel.Image.URL)
	if coverURL != "" {
		if imageURL, resolveErr := resolvePublicReference(feedURL, coverURL); resolveErr == nil {
			coverURL = imageURL.String()
		} else {
			coverURL = ""
		}
	}
	description := firstNonEmpty(item.Content, item.Description)
	guid := strings.TrimSpace(item.GUID)
	return &domain.ResolvedEpisode{
		SourceType:         domain.SourceRSS,
		RSSGUID:            guid,
		PodcastTitle:       strings.TrimSpace(document.Channel.Title),
		PodcastAuthor:      strings.TrimSpace(document.Channel.Author),
		PodcastDescription: strings.TrimSpace(document.Channel.Description),
		PodcastCoverURL:    coverURL,
		EpisodeTitle:       title,
		Description:        strings.TrimSpace(description),
		PublishedAt:        publishedAt,
		DurationMS:         parseRSSDuration(item.Duration),
		CanonicalURL:       canonicalURL,
		FeedURL:            feedURL.String(),
		AudioURL:           audioURL.String(),
	}, nil
}

func newestRSSItem(items []rssItem) (*rssItem, *time.Time) {
	selected := -1
	var selectedTime *time.Time
	for index := range items {
		enclosureURL, err := url.Parse(strings.TrimSpace(items[index].Enclosure.URL))
		if err != nil || !isAudioResponse(items[index].Enclosure.Type, enclosureURL.Path) {
			continue
		}
		publishedAt := parseRSSTime(items[index].PublishedAt)
		if selected == -1 || (publishedAt != nil && (selectedTime == nil || publishedAt.After(*selectedTime))) {
			selected = index
			selectedTime = publishedAt
		}
	}
	if selected == -1 {
		return nil, nil
	}
	return &items[selected], selectedTime
}

func parseRSSTime(value string) *time.Time {
	for _, layout := range []string{time.RFC1123Z, time.RFC1123, time.RFC822Z, time.RFC822, time.RFC3339, "Mon, 2 Jan 2006 15:04:05 -0700"} {
		if parsed, err := time.Parse(layout, strings.TrimSpace(value)); err == nil {
			return &parsed
		}
	}
	return nil
}

func parseRSSDuration(value string) int64 {
	parts := strings.Split(strings.TrimSpace(value), ":")
	if len(parts) == 0 || len(parts) > 3 {
		return 0
	}
	seconds := float64(0)
	for _, part := range parts {
		number, err := strconv.ParseFloat(strings.TrimSpace(part), 64)
		if err != nil || number < 0 {
			return 0
		}
		seconds = seconds*60 + number
	}
	return int64(seconds * 1000)
}

func resolvePublicReference(base *url.URL, reference string) (*url.URL, error) {
	parsed, err := url.Parse(strings.TrimSpace(reference))
	if err != nil {
		return nil, err
	}
	resolved := base.ResolveReference(parsed)
	if err := validateURL(resolved); err != nil {
		return nil, err
	}
	return resolved, nil
}

func isXMLContentType(contentType string) bool {
	mediaType, _, err := mime.ParseMediaType(contentType)
	if err != nil {
		return false
	}
	return strings.Contains(mediaType, "xml") || strings.Contains(mediaType, "rss")
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if value = strings.TrimSpace(value); value != "" {
			return value
		}
	}
	return ""
}
