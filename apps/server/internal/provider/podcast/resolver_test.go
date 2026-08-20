package podcast

import (
	"context"
	"io"
	"net/http"
	"strings"
	"testing"

	domain "github.com/Actify/echonote/apps/server/internal/domain/podcast"
)

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(request *http.Request) (*http.Response, error) {
	return f(request)
}

func TestAppleResolver(t *testing.T) {
	client := &http.Client{Transport: roundTripFunc(func(request *http.Request) (*http.Response, error) {
		if request.URL.Query().Get("id") != "123" || request.URL.Query().Get("entity") != "podcastEpisode" {
			t.Fatalf("unexpected lookup query: %s", request.URL.RawQuery)
		}
		body := `{"results":[
            {"kind":"podcast","collectionId":123,"collectionName":"Show","artistName":"Host","feedUrl":"https://feeds.example.com/show.xml","artworkUrl600":"https://cdn.example.com/cover.jpg"},
            {"kind":"podcast-episode","trackId":456,"collectionId":123,"trackName":"Episode","collectionName":"Show","episodeUrl":"https://cdn.example.com/e.mp3","episodeGuid":"guid-456","releaseDate":"2026-08-20T01:02:03Z","trackTimeMillis":123000,"trackViewUrl":"https://podcasts.apple.com/show/id123?i=456"}
        ]}`
		return response(request, http.StatusOK, "application/json", body), nil
	})}
	resolver := newAppleResolver(client)
	resolver.lookupURL = "https://itunes.example.com/lookup"
	resolved, err := resolver.Resolve(context.Background(), "https://podcasts.apple.com/us/podcast/show/id123?i=456")
	if err != nil {
		t.Fatal(err)
	}
	if resolved.SourceType != domain.SourceApple || resolved.ExternalID != "456" || resolved.RSSGUID != "guid-456" || resolved.DurationMS != 123000 {
		t.Fatalf("unexpected resolved episode: %+v", resolved)
	}
}

func TestResolverChoosesDirectAudioOrNewestRSSItem(t *testing.T) {
	feed := `<?xml version="1.0"?><rss version="2.0" xmlns:itunes="http://www.itunes.com/dtds/podcast-1.0.dtd"><channel>
        <title>Show</title><itunes:author>Host</itunes:author>
        <item><title>Old</title><pubDate>Wed, 19 Aug 2026 10:00:00 +0000</pubDate><enclosure url="https://cdn.example.com/old.mp3" type="audio/mpeg"/></item>
        <item><title>New</title><guid>new-guid</guid><pubDate>Thu, 20 Aug 2026 10:00:00 +0000</pubDate><itunes:duration>1:02</itunes:duration><enclosure url="https://cdn.example.com/new.mp3" type="audio/mpeg"/></item>
        <item><title>Video</title><pubDate>Fri, 21 Aug 2026 10:00:00 +0000</pubDate><enclosure url="https://cdn.example.com/video.mp4" type="video/mp4"/></item>
    </channel></rss>`
	client := &http.Client{Transport: roundTripFunc(func(request *http.Request) (*http.Response, error) {
		switch {
		case request.URL.Path == "/episode.mp3" && request.Method == http.MethodHead:
			result := response(request, http.StatusOK, "audio/mpeg", "")
			result.Header.Set("Content-Disposition", `attachment; filename="Direct Episode.mp3"`)
			return result, nil
		case request.URL.Path == "/show.xml" && request.Method == http.MethodHead:
			return response(request, http.StatusOK, "application/rss+xml", ""), nil
		case request.URL.Path == "/show.xml" && request.Method == http.MethodGet:
			return response(request, http.StatusOK, "application/rss+xml", feed), nil
		default:
			t.Fatalf("unexpected request: %s %s", request.Method, request.URL)
			return nil, nil
		}
	})}
	resolver := NewResolver(client)

	direct, err := resolver.Resolve(context.Background(), "https://media.example.com/episode.mp3")
	if err != nil || direct.SourceType != domain.SourceDirectAudio || direct.EpisodeTitle != "Direct Episode" {
		t.Fatalf("direct=%+v err=%v", direct, err)
	}
	rss, err := resolver.Resolve(context.Background(), "https://feeds.example.com/show.xml")
	if err != nil {
		t.Fatal(err)
	}
	if rss.SourceType != domain.SourceRSS || rss.EpisodeTitle != "New" || rss.RSSGUID != "new-guid" || rss.DurationMS != 62000 {
		t.Fatalf("unexpected RSS episode: %+v", rss)
	}
}

func TestPublicURLValidationRejectsSSRFAddresses(t *testing.T) {
	for _, rawURL := range []string{
		"file:///etc/passwd",
		"http://localhost/audio.mp3",
		"http://127.0.0.1/audio.mp3",
		"http://10.0.0.1/audio.mp3",
		"http://100.64.0.1/audio.mp3",
		"http://[::1]/audio.mp3",
	} {
		t.Run(strings.ReplaceAll(rawURL, "/", "_"), func(t *testing.T) {
			if _, err := parsePublicURL(rawURL); err == nil {
				t.Fatalf("expected %q to be rejected", rawURL)
			}
		})
	}
	if _, err := parsePublicURL("https://example.com/audio.mp3"); err != nil {
		t.Fatal(err)
	}
}

func response(request *http.Request, status int, contentType, body string) *http.Response {
	return &http.Response{
		StatusCode: status,
		Header:     http.Header{"Content-Type": []string{contentType}},
		Body:       io.NopCloser(strings.NewReader(body)),
		Request:    request,
	}
}
