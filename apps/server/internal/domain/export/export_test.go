package export

import (
	"errors"
	"strings"
	"testing"
	"time"

	aidomain "github.com/Actify/echonote/apps/server/internal/domain/ai"
)

func TestComposeOrganizedNote(t *testing.T) {
	published := time.Date(2026, 8, 20, 0, 0, 0, 0, time.UTC)
	input := Input{
		Episode: Episode{Title: `E1: <Agent> / Demo`, PodcastTitle: "节目", DurationMS: 61_000, PublishedAt: &published, SourceURL: "https://example.com/e1"},
		Notes:   []Note{{Content: "我的 *笔记*", CreatedAt: published}},
		Artifact: &aidomain.ArtifactResult{
			OneSentenceSummary: "一句话总结", KeyPoints: []string{"核心观点"},
			WorthReviewing: []aidomain.WorthReviewing{{StartMS: 65_000, Quote: "原始引文", Reason: "值得回顾"}},
		},
		Segments: []Segment{{SpeakerName: "主讲人", StartMS: 1_000, Text: "第一段 <正文>"}},
	}
	result, err := Compose(input, Options{
		Mode: ModeOrganizedNote, IncludeUserNotes: true, IncludeSummary: true,
		IncludeKeyPoints: true, IncludeWorthReviewing: true, IncludeTranscript: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	for _, value := range []string{"【一句话总结】", "【核心观点】", "【值得回顾】", "【我的笔记】", "【Transcript 节选】"} {
		if !strings.Contains(result.Text, value) {
			t.Fatalf("plain text omitted %q: %s", value, result.Text)
		}
	}
	if !strings.Contains(result.Markdown, `我的 \*笔记\*`) || !strings.Contains(result.Markdown, "00:01:05") {
		t.Fatalf("markdown was not escaped or timestamped: %s", result.Markdown)
	}
	if result.SuggestedFilename != "节目｜E1- -Agent- - Demo.md" {
		t.Fatalf("filename=%q", result.SuggestedFilename)
	}
}

func TestComposeRequiresRequestedSources(t *testing.T) {
	input := Input{Episode: Episode{Title: "Episode"}}
	if _, err := Compose(input, Options{Mode: ModeOrganizedNote, IncludeSummary: true}); !errors.Is(err, ErrArtifactNotReady) {
		t.Fatalf("artifact error=%v", err)
	}
	if _, err := Compose(input, Options{Mode: ModeFullTranscript}); !errors.Is(err, ErrTranscriptNotReady) {
		t.Fatalf("transcript error=%v", err)
	}
	if _, err := Compose(input, Options{Mode: ModeNotesOnly}); !errors.Is(err, ErrNoContent) {
		t.Fatalf("content error=%v", err)
	}
}

func TestComposeRejectsOversizedContent(t *testing.T) {
	input := Input{
		Episode: Episode{Title: "Episode"},
		Notes:   []Note{{Content: strings.Repeat("a", MaxContentBytes+1), CreatedAt: time.Now()}},
	}
	if _, err := Compose(input, Options{Mode: ModeNotesOnly}); !errors.Is(err, ErrTooLarge) {
		t.Fatalf("size error=%v", err)
	}
}

func TestFilenameAvoidsWindowsDeviceNames(t *testing.T) {
	if got := filename("CON.txt"); got != "echonote-CON.txt.md" {
		t.Fatalf("filename=%q", got)
	}
}
