package podcast

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"math"
	"net/http"
	"strings"
	"time"
	"unicode/utf8"

	domain "github.com/Actify/echonote/apps/server/internal/domain/podcast"
)

const snapAnyBaseURL = "https://api.snapany.com/openapi/v1"

type snapAnyResolver struct {
	client  *http.Client
	apiKey  string
	baseURL string
}

type snapAnyPost struct {
	Site      string         `json:"site"`
	Title     string         `json:"title"`
	Text      string         `json:"text"`
	Medias    []snapAnyMedia `json:"medias"`
	ID        string         `json:"id"`
	PostURL   string         `json:"post_url"`
	CreatedAt string         `json:"created_at"`
	Author    snapAnyProfile `json:"author"`
}

type snapAnyMedia struct {
	Type        string            `json:"media_type"`
	ResourceURL string            `json:"resource_url"`
	PreviewURL  string            `json:"preview_url"`
	Duration    float64           `json:"duration"`
	Headers     map[string]string `json:"headers"`
	Variants    []struct {
		AudioURL string `json:"audio_url"`
	} `json:"variants"`
}

type snapAnyProfile struct {
	Username    string `json:"username"`
	DisplayName string `json:"display_name"`
	AvatarURL   string `json:"avatar_url"`
}

type snapAnyPlaylist struct {
	Posts   []snapAnyPost  `json:"posts"`
	Profile snapAnyProfile `json:"profile"`
}

type snapAnyAPIError struct {
	status    int
	code      string
	message   string
	retryable bool
	cause     error
}

func (err *snapAnyAPIError) Error() string {
	if err.message != "" {
		return err.message
	}
	return "SnapAny request failed"
}

func newSnapAnyResolver(client *http.Client, apiKey string) *snapAnyResolver {
	return &snapAnyResolver{client: client, apiKey: strings.TrimSpace(apiKey), baseURL: snapAnyBaseURL}
}

func (r *snapAnyResolver) CanResolve(rawURL string) bool {
	_, err := parsePublicURL(rawURL)
	return err == nil
}

func (r *snapAnyResolver) Resolve(ctx context.Context, rawURL string) (*domain.ResolvedEpisode, error) {
	parsed, err := parsePublicURL(rawURL)
	if err != nil {
		return nil, domain.NewResolveError("IMPORT_INVALID_URL", "URL is invalid", false, err)
	}

	var post snapAnyPost
	if apiErr := r.extract(ctx, "/extract/post", map[string]string{"url": parsed.String()}, &post); apiErr != nil {
		if apiErr.code == "playlist_not_supported" {
			return r.resolvePlaylist(ctx, parsed.String())
		}
		if apiErr.code == "invalid_url" || apiErr.code == "unsupported_url" || apiErr.code == "unsupported_site" {
			return nil, domain.ErrNotApplicable
		}
		return nil, apiErr.resolveError()
	}
	if resolved := snapAnyEpisode(post, snapAnyProfile{}, parsed.String()); resolved != nil {
		return resolved, nil
	}
	return nil, domain.NewResolveError("IMPORT_SNAPANY_NO_MEDIA", "SnapAny found no audio or video to transcribe", false, nil)
}

func (r *snapAnyResolver) resolvePlaylist(ctx context.Context, rawURL string) (*domain.ResolvedEpisode, error) {
	var playlist snapAnyPlaylist
	if apiErr := r.extract(ctx, "/extract/playlist", map[string]string{"url": rawURL}, &playlist); apiErr != nil {
		return nil, apiErr.resolveError()
	}
	// ponytail: one transcription resolves one item; add batch imports before following playlist cursors.
	for _, post := range playlist.Posts {
		if resolved := snapAnyEpisode(post, playlist.Profile, rawURL); resolved != nil {
			return resolved, nil
		}
	}
	return nil, domain.NewResolveError("IMPORT_SNAPANY_NO_MEDIA", "SnapAny list page has no audio or video to transcribe", false, nil)
}

func (r *snapAnyResolver) extract(ctx context.Context, path string, payload any, result any) *snapAnyAPIError {
	body, err := json.Marshal(payload)
	if err != nil {
		return &snapAnyAPIError{code: "request_invalid", message: "SnapAny request could not be encoded", cause: err}
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, r.baseURL+path, bytes.NewReader(body))
	if err != nil {
		return &snapAnyAPIError{code: "request_invalid", message: "SnapAny request could not be created", cause: err}
	}
	request.Header.Set("Authorization", "Bearer "+r.apiKey)
	request.Header.Set("Accept", "application/json")
	request.Header.Set("Accept-Language", "zh")
	request.Header.Set("Content-Type", "application/json")
	response, err := r.client.Do(request)
	if err != nil {
		return &snapAnyAPIError{code: "unavailable", message: "SnapAny is unavailable", retryable: true, cause: err}
	}
	defer response.Body.Close()
	responseBody, err := readLimited(response.Body)
	if err != nil {
		return &snapAnyAPIError{status: response.StatusCode, code: "response_invalid", message: "SnapAny response is too large or unreadable", retryable: response.StatusCode >= 500, cause: err}
	}
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		var failure struct {
			Code    string `json:"code"`
			Message string `json:"message"`
			Detail  string `json:"detail"`
		}
		_ = json.Unmarshal(responseBody, &failure)
		message := firstNonEmpty(failure.Message, failure.Detail, fmt.Sprintf("SnapAny returned HTTP %d", response.StatusCode))
		return &snapAnyAPIError{
			status: response.StatusCode, code: failure.Code, message: message,
			retryable: response.StatusCode == http.StatusTooManyRequests || response.StatusCode >= 500 || snapAnyTemporaryError(failure.Code),
		}
	}
	if err := json.Unmarshal(responseBody, result); err != nil {
		return &snapAnyAPIError{status: response.StatusCode, code: "response_invalid", message: "SnapAny returned an invalid response", cause: err}
	}
	return nil
}

func (err *snapAnyAPIError) resolveError() *domain.ResolveError {
	code := strings.ToUpper(strings.TrimSpace(err.code))
	if code == "" {
		code = "FAILED"
	}
	return domain.NewResolveError("IMPORT_SNAPANY_"+code, err.Error(), err.retryable, err.cause)
}

func snapAnyTemporaryError(code string) bool {
	switch code {
	case "extract_failed", "retryable", "timeout", "unknown":
		return true
	default:
		return false
	}
}

func snapAnyEpisode(post snapAnyPost, profile snapAnyProfile, rawURL string) *domain.ResolvedEpisode {
	media, audioURL := snapAnyAudio(post.Medias)
	if media == nil {
		return nil
	}
	canonicalURL := rawURL
	if parsed, err := parsePublicURL(post.PostURL); err == nil {
		canonicalURL = parsed.String()
	}
	var publishedAt *time.Time
	if parsed, err := time.Parse(time.RFC3339, strings.TrimSpace(post.CreatedAt)); err == nil {
		publishedAt = &parsed
	}
	durationMS := int64(0)
	if media.Duration > 0 && !math.IsInf(media.Duration, 0) && !math.IsNaN(media.Duration) {
		durationMS = int64(math.Round(media.Duration * 1000))
	}
	author := firstNonEmpty(post.Author.DisplayName, post.Author.Username, profile.DisplayName, profile.Username)
	externalID := strings.TrimSpace(post.ID)
	if site := strings.ToLower(strings.TrimSpace(post.Site)); site != "" && externalID != "" {
		externalID = site + ":" + externalID
	}
	return &domain.ResolvedEpisode{
		SourceType: domain.SourceSnapAny, ExternalID: externalID,
		PodcastTitle: firstNonEmpty(profile.DisplayName, profile.Username), PodcastAuthor: author,
		PodcastCoverURL: firstNonEmpty(media.PreviewURL, post.Author.AvatarURL, profile.AvatarURL),
		EpisodeTitle:    snapAnyTitle(post, author), Description: strings.TrimSpace(post.Text),
		PublishedAt: publishedAt, DurationMS: durationMS, CanonicalURL: canonicalURL,
		AudioURL: audioURL, AudioHeaders: media.Headers,
	}
}

func snapAnyAudio(medias []snapAnyMedia) (*snapAnyMedia, string) {
	for _, mediaType := range []string{"audio", "video", "live", "file"} {
		for index := range medias {
			media := &medias[index]
			if media.Type != mediaType {
				continue
			}
			candidates := make([]string, 0, len(media.Variants)+1)
			if mediaType == "audio" {
				candidates = append(candidates, media.ResourceURL)
			}
			for _, variant := range media.Variants {
				candidates = append(candidates, variant.AudioURL)
			}
			if mediaType != "audio" {
				candidates = append(candidates, media.ResourceURL)
			}
			for _, candidate := range candidates {
				if parsed, err := parsePublicURL(candidate); err == nil {
					return media, parsed.String()
				}
			}
		}
	}
	return nil, ""
}

func snapAnyTitle(post snapAnyPost, author string) string {
	title := firstNonEmpty(post.Title, post.Text)
	if title == "" && author != "" {
		title = author + " post"
	}
	if title == "" && strings.TrimSpace(post.ID) != "" {
		title = "Post " + strings.TrimSpace(post.ID)
	}
	if title == "" {
		title = "Imported media"
	}
	title = strings.Join(strings.Fields(title), " ")
	if utf8.RuneCountInString(title) <= 120 {
		return title
	}
	return string([]rune(title)[:119]) + "…"
}
