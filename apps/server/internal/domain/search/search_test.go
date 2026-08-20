package search

import (
	"strings"
	"testing"
	"unicode/utf8"
)

func TestTranscriptChunksPreserveSpeakerAndOverlap(t *testing.T) {
	segments := make([]Segment, 0, 11)
	for index := range 10 {
		segments = append(segments, Segment{
			SpeakerID: "speaker-a", StartMS: int64(index * 1000), EndMS: int64((index + 1) * 1000),
			Text: strings.Repeat("中", 60),
		})
	}
	segments = append(segments, Segment{SpeakerID: "speaker-b", StartMS: 10000, EndMS: 11000, Text: "切换说话人"})
	chunks := TranscriptChunks(segments)
	if len(chunks) != 3 {
		t.Fatalf("chunks=%d want=3", len(chunks))
	}
	if chunks[0].SpeakerID != "speaker-a" || chunks[1].SpeakerID != "speaker-a" || chunks[2].SpeakerID != "speaker-b" {
		t.Fatalf("speaker boundaries were mixed: %+v", chunks)
	}
	if chunks[1].StartMS != 8000 || chunks[1].EndMS != 10000 {
		t.Fatalf("overlap time=%d-%d want=8000-10000", chunks[1].StartMS, chunks[1].EndMS)
	}
	for _, chunk := range chunks {
		if utf8.RuneCountInString(chunk.Text) > TargetChunkRunes {
			t.Fatalf("chunk %d has %d runes", chunk.Index, utf8.RuneCountInString(chunk.Text))
		}
	}
	firstHash := DocumentHash("transcript", "source", "content", chunks)
	if firstHash != DocumentHash("transcript", "source", "content", chunks) || firstHash == DocumentHash("transcript", "source", "changed", chunks) {
		t.Fatal("document hash is not deterministic or content-sensitive")
	}
}

func TestFuseAndSnippet(t *testing.T) {
	keyword := []Candidate{{ID: "a", Text: "alpha"}, {ID: "b", Text: "beta"}}
	semantic := []Candidate{{ID: "b", Text: "beta"}, {ID: "c", Text: "gamma"}}
	result := Fuse(keyword, semantic, 3)
	if len(result) != 3 || result[0].ID != "b" || result[0].Score <= result[1].Score {
		t.Fatalf("fused=%+v", result)
	}
	text := strings.Repeat("前", 150) + "融资策略" + strings.Repeat("后", 150)
	snippet := Snippet(text, "融资", 60)
	if !strings.Contains(snippet, "融资") || utf8.RuneCountInString(strings.Trim(snippet, "…")) > 60 {
		t.Fatalf("snippet=%q", snippet)
	}
}
