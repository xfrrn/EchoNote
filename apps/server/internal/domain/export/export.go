package export

import (
	"errors"
	"fmt"
	"strings"
	"time"
	"unicode"

	aidomain "github.com/Actify/echonote/apps/server/internal/domain/ai"
)

const (
	ModeNotesOnly          = "notes_only"
	ModeOrganizedNote      = "organized_note"
	ModeSelectedTranscript = "selected_transcript"
	ModeFullTranscript     = "full_transcript"
	MaxContentBytes        = 4 << 20
	maxFilenameRunes       = 100
)

var (
	ErrInvalidOptions     = errors.New("export options are invalid")
	ErrArtifactNotReady   = errors.New("a current AI artifact is required")
	ErrTranscriptNotReady = errors.New("an active transcript is required")
	ErrSegmentsNotFound   = errors.New("one or more transcript segments were not found")
	ErrNoContent          = errors.New("the requested export has no content")
	ErrTooLarge           = errors.New("the generated export exceeds the size limit")
)

type Options struct {
	Mode                  string
	IncludeUserNotes      bool
	IncludeSummary        bool
	IncludeKeyPoints      bool
	IncludeWorthReviewing bool
	IncludeTranscript     bool
}

func (options Options) Validate() error {
	switch options.Mode {
	case ModeNotesOnly, ModeSelectedTranscript, ModeFullTranscript:
		if !options.hasOrganizedSections() {
			return nil
		}
	case ModeOrganizedNote:
		if options.hasOrganizedSections() {
			return nil
		}
	}
	return ErrInvalidOptions
}

func (options Options) hasOrganizedSections() bool {
	return options.IncludeUserNotes || options.IncludeSummary || options.IncludeKeyPoints || options.IncludeWorthReviewing || options.IncludeTranscript
}

type Episode struct {
	Title        string
	PodcastTitle string
	DurationMS   int64
	PublishedAt  *time.Time
	SourceURL    string
}

type Note struct {
	Content   string
	CreatedAt time.Time
}

type Segment struct {
	SpeakerName string
	StartMS     int64
	Text        string
}

type Input struct {
	Episode  Episode
	Notes    []Note
	Artifact *aidomain.ArtifactResult
	Segments []Segment
}

type Output struct {
	Title             string
	Text              string
	Markdown          string
	SuggestedFilename string
}

func Compose(input Input, options Options) (Output, error) {
	if err := options.Validate(); err != nil {
		return Output{}, err
	}
	title := exportTitle(input.Episode)
	if title == "" {
		return Output{}, ErrNoContent
	}
	plain := []string{title}
	markdown := []string{"# " + escapeMarkdown(title)}
	metadata := episodeMetadata(input.Episode)
	if metadata != "" {
		plain = append(plain, metadata)
		markdown = append(markdown, "", "> "+escapeMarkdown(metadata))
	}
	if input.Episode.SourceURL != "" {
		source := cleanInline(input.Episode.SourceURL)
		plain = append(plain, "原始链接："+source)
		markdown = append(markdown, "> 原始链接：<"+strings.ReplaceAll(source, ">", "%3E")+">")
	}

	sections := 0
	addSection := func(name string, plainLines, markdownLines []string) {
		if len(plainLines) == 0 {
			return
		}
		plain = append(plain, "", "【"+name+"】")
		plain = append(plain, plainLines...)
		markdown = append(markdown, "", "## "+name, "")
		markdown = append(markdown, markdownLines...)
		sections++
	}

	includeNotes := options.Mode == ModeNotesOnly || (options.Mode == ModeOrganizedNote && options.IncludeUserNotes)
	includeTranscript := options.Mode == ModeSelectedTranscript || options.Mode == ModeFullTranscript || (options.Mode == ModeOrganizedNote && options.IncludeTranscript)
	includeAI := options.Mode == ModeOrganizedNote && (options.IncludeSummary || options.IncludeKeyPoints || options.IncludeWorthReviewing)
	if includeAI && input.Artifact == nil {
		return Output{}, ErrArtifactNotReady
	}
	if includeTranscript && len(input.Segments) == 0 {
		return Output{}, ErrTranscriptNotReady
	}

	if options.Mode == ModeOrganizedNote && options.IncludeSummary {
		value := cleanBlock(input.Artifact.OneSentenceSummary)
		addSection("一句话总结", value, markdownParagraphs(value))
	}
	if options.Mode == ModeOrganizedNote && options.IncludeKeyPoints {
		plainLines, markdownLines := numberedLines(input.Artifact.KeyPoints)
		addSection("核心观点", plainLines, markdownLines)
	}
	if options.Mode == ModeOrganizedNote && options.IncludeWorthReviewing {
		plainLines, markdownLines := worthReviewingLines(input.Artifact.WorthReviewing)
		addSection("值得回顾", plainLines, markdownLines)
	}
	if includeNotes {
		plainLines, markdownLines := noteLines(input.Notes)
		addSection("我的笔记", plainLines, markdownLines)
	}
	if includeTranscript {
		plainLines, markdownLines := transcriptLines(input.Segments)
		name := "Transcript 节选"
		if options.Mode == ModeFullTranscript {
			name = "完整 Transcript"
		}
		addSection(name, plainLines, markdownLines)
	}
	if sections == 0 {
		return Output{}, ErrNoContent
	}

	plain = append(plain, "", "—— 用 EchoNote 整理")
	markdown = append(markdown, "", "---", "", "*用 EchoNote 整理*")
	textValue := strings.TrimSpace(strings.Join(plain, "\n"))
	markdownValue := strings.TrimSpace(strings.Join(markdown, "\n"))
	if len(textValue) > MaxContentBytes || len(markdownValue) > MaxContentBytes {
		return Output{}, ErrTooLarge
	}
	return Output{Title: title, Text: textValue, Markdown: markdownValue, SuggestedFilename: filename(title)}, nil
}

func episodeMetadata(episode Episode) string {
	parts := make([]string, 0, 3)
	if episode.DurationMS > 0 {
		minutes := (episode.DurationMS + int64(time.Minute) - 1) / int64(time.Minute)
		parts = append(parts, fmt.Sprintf("%d 分钟", minutes))
	}
	if episode.PublishedAt != nil {
		parts = append(parts, episode.PublishedAt.Format("2006-01-02"))
	}
	return strings.Join(parts, " · ")
}

func exportTitle(episode Episode) string {
	title, podcast := cleanInline(episode.Title), cleanInline(episode.PodcastTitle)
	if podcast == "" || strings.EqualFold(podcast, title) {
		return title
	}
	return podcast + "｜" + title
}

func numberedLines(values []string) ([]string, []string) {
	plain, markdown := make([]string, 0, len(values)), make([]string, 0, len(values))
	for _, value := range values {
		lines := cleanBlock(value)
		if len(lines) == 0 {
			continue
		}
		plain = append(plain, fmt.Sprintf("%d. %s", len(plain)+1, strings.Join(lines, " ")))
		markdown = append(markdown, fmt.Sprintf("%d. %s", len(markdown)+1, escapeMarkdown(strings.Join(lines, " "))))
	}
	return plain, markdown
}

func worthReviewingLines(values []aidomain.WorthReviewing) ([]string, []string) {
	plain, markdown := make([]string, 0, len(values)), make([]string, 0, len(values))
	for _, value := range values {
		quote, reason := cleanInline(value.Quote), cleanInline(value.Reason)
		if quote == "" {
			continue
		}
		stamp := formatTimestamp(value.StartMS)
		line := stamp + " 「" + quote + "」"
		if reason != "" {
			line += " — " + reason
		}
		plain = append(plain, line)
		markdown = append(markdown, "- **"+stamp+"** “"+escapeMarkdown(quote)+"”"+markdownReason(reason))
	}
	return plain, markdown
}

func noteLines(values []Note) ([]string, []string) {
	plain, markdown := make([]string, 0, len(values)), make([]string, 0, len(values))
	for _, value := range values {
		content := cleanInline(value.Content)
		if content == "" {
			continue
		}
		stamp := value.CreatedAt.Format("2006-01-02 15:04")
		plain = append(plain, stamp+"  "+content)
		markdown = append(markdown, "- **"+stamp+"**  "+escapeMarkdown(content))
	}
	return plain, markdown
}

func transcriptLines(values []Segment) ([]string, []string) {
	plain, markdown := make([]string, 0, len(values)), make([]string, 0, len(values)*3)
	for _, value := range values {
		content, speaker := cleanBlock(value.Text), cleanInline(value.SpeakerName)
		if len(content) == 0 {
			continue
		}
		if speaker == "" {
			speaker = "未知 Speaker"
		}
		stamp := formatTimestamp(value.StartMS)
		plain = append(plain, speaker+" "+stamp+"："+strings.Join(content, " "))
		if len(markdown) > 0 {
			markdown = append(markdown, "")
		}
		markdown = append(markdown, "> **"+escapeMarkdown(speaker)+" · "+stamp+"**", ">")
		for _, line := range content {
			markdown = append(markdown, "> "+escapeMarkdown(line))
		}
	}
	return plain, markdown
}

func markdownParagraphs(lines []string) []string {
	result := make([]string, len(lines))
	for index, line := range lines {
		result[index] = escapeMarkdown(line)
	}
	return result
}

func markdownReason(reason string) string {
	if reason == "" {
		return ""
	}
	return " — " + escapeMarkdown(reason)
}

func formatTimestamp(milliseconds int64) string {
	if milliseconds < 0 {
		milliseconds = 0
	}
	seconds := milliseconds / 1000
	return fmt.Sprintf("%02d:%02d:%02d", seconds/3600, (seconds/60)%60, seconds%60)
}

func cleanInline(value string) string {
	return strings.Join(cleanBlock(value), " ")
}

func cleanBlock(value string) []string {
	value = strings.ReplaceAll(strings.ReplaceAll(value, "\r\n", "\n"), "\r", "\n")
	value = strings.Map(func(r rune) rune {
		if r == '\n' || r == '\t' || !unicode.IsControl(r) {
			return r
		}
		return -1
	}, value)
	lines := strings.Split(value, "\n")
	result := make([]string, 0, len(lines))
	for _, line := range lines {
		line = strings.Join(strings.Fields(line), " ")
		if line == "" {
			continue
		}
		result = append(result, line)
	}
	return result
}

func escapeMarkdown(value string) string {
	replacer := strings.NewReplacer(
		"\\", "\\\\", "`", "\\`", "*", "\\*", "_", "\\_",
		"[", "\\[", "]", "\\]", "<", "&lt;", ">", "&gt;",
	)
	return replacer.Replace(value)
}

func filename(title string) string {
	var builder strings.Builder
	lastDash := false
	for _, r := range cleanInline(title) {
		forbidden := unicode.IsControl(r) || strings.ContainsRune(`<>:"/\|?*`, r)
		if forbidden {
			if !lastDash {
				builder.WriteRune('-')
				lastDash = true
			}
			continue
		}
		builder.WriteRune(r)
		lastDash = false
	}
	base := strings.Trim(builder.String(), " .-")
	runes := []rune(base)
	if len(runes) > maxFilenameRunes {
		base = strings.TrimRight(string(runes[:maxFilenameRunes]), " .-")
	}
	if base == "" {
		base = "echonote-episode"
	}
	deviceName := strings.ToLower(strings.SplitN(base, ".", 2)[0])
	reserved := deviceName == "con" || deviceName == "prn" || deviceName == "aux" || deviceName == "nul"
	if len(deviceName) == 4 && (strings.HasPrefix(deviceName, "com") || strings.HasPrefix(deviceName, "lpt")) && deviceName[3] >= '1' && deviceName[3] <= '9' {
		reserved = true
	}
	if reserved {
		base = "echonote-" + base
	}
	return base + ".md"
}
