package ai

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestValidateArtifactAndHashes(t *testing.T) {
	input := EpisodeInput{
		EpisodeID: "episode", EpisodeTitle: "标题", TranscriptVersionID: "version",
		Speakers: []Speaker{{ID: "speaker", Name: "主讲人"}},
		Segments: []Segment{{ID: "segment", SpeakerID: "speaker", StartMS: 10, EndMS: 20, Text: "原始引文"}},
		Notes:    []Note{{ID: "note", Content: "我的笔记"}},
	}
	raw := []byte(`{
		"one_sentence_summary":"一句话",
		"key_points":["观点"],
		"speaker_views":[{"speaker_id":"speaker","points":["人物观点"]}],
		"worth_reviewing":[{"transcript_segment_id":"segment","reason":"值得回顾"}],
		"note_connections":[{"note_id":"note","insight":"笔记关联"}]
	}`)
	result, err := ValidateArtifact(raw, input)
	if err != nil || result.WorthReviewing[0].Quote != "原始引文" || result.SpeakerViews[0].SpeakerName != "主讲人" || !strings.Contains(result.SearchText(), "笔记关联") {
		t.Fatalf("result=%+v err=%v", result, err)
	}
	if input.InputHash() != input.InputHash() || input.NotesRevision() == (EpisodeInput{Notes: nil}).NotesRevision() {
		t.Fatal("input hashes are not deterministic or note-sensitive")
	}
	var invalid map[string]any
	_ = json.Unmarshal(raw, &invalid)
	invalid["worth_reviewing"] = []any{map[string]any{"transcript_segment_id": "invented", "reason": "x"}}
	invalidRaw, _ := json.Marshal(invalid)
	if _, err := ValidateArtifact(invalidRaw, input); err == nil {
		t.Fatal("expected an invented segment to fail")
	}
}

func TestCitationParserStreamsAndRejectsInventedIDs(t *testing.T) {
	allowed := map[string]CitationSource{
		"segment:one": {Key: "segment:one", SourceType: "transcript", SourceID: "one", Excerpt: "source"},
	}
	parser := &CitationParser{}
	var streamed strings.Builder
	for _, delta := range []string{"回答中", "文", CitationOpen[:8], CitationOpen[8:] + `{"ids":["segment:one"]}`, CitationClose} {
		part, err := parser.Push(delta)
		if err != nil {
			t.Fatal(err)
		}
		streamed.WriteString(part)
	}
	answer, citations, err := parser.Finish(allowed)
	if err != nil || answer != "回答中文" || streamed.String() != "回答中文" || len(citations) != 1 {
		t.Fatalf("answer=%q streamed=%q citations=%+v err=%v", answer, streamed.String(), citations, err)
	}

	parser = &CitationParser{}
	_, _ = parser.Push("回答" + CitationOpen + `{"ids":["segment:invented"]}` + CitationClose)
	if _, _, err := parser.Finish(allowed); err == nil {
		t.Fatal("expected an invented citation to fail")
	}

	parser = &CitationParser{}
	_, _ = parser.Push("回答" + CitationOpen + `{"ids":["segment:one"]}{}` + CitationClose)
	if _, _, err := parser.Finish(allowed); err == nil {
		t.Fatal("expected trailing citation data to fail")
	}
}
