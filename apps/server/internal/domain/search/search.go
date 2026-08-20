package search

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"sort"
	"strings"
	"unicode/utf8"
)

const (
	TargetChunkRunes  = 600
	ChunkOverlapRunes = 80
	rrfConstant       = 60
)

type EmbeddingInputType string

const (
	EmbeddingQuery    EmbeddingInputType = "query"
	EmbeddingDocument EmbeddingInputType = "document"
)

type EmbeddingProvider interface {
	Embed(context.Context, []string, EmbeddingInputType) ([][]float32, error)
	Model() string
	Dimensions() int
}

type Segment struct {
	SpeakerID string `json:"speaker_id"`
	StartMS   int64  `json:"start_ms"`
	EndMS     int64  `json:"end_ms"`
	Text      string `json:"text"`
}

type Chunk struct {
	Index     int    `json:"index"`
	SpeakerID string `json:"speaker_id"`
	StartMS   int64  `json:"start_ms"`
	EndMS     int64  `json:"end_ms"`
	Text      string `json:"text"`
}

func TranscriptChunks(segments []Segment) []Chunk {
	chunks := make([]Chunk, 0)
	current := make([]Segment, 0)
	flush := func(overlap bool) {
		if len(current) == 0 {
			return
		}
		chunks = append(chunks, chunkFrom(len(chunks), current))
		if overlap {
			current = trailingSegments(current, ChunkOverlapRunes)
		} else {
			current = current[:0]
		}
	}

	for _, segment := range segments {
		segment.Text = strings.TrimSpace(segment.Text)
		if segment.Text == "" {
			continue
		}
		if len(current) > 0 && current[0].SpeakerID != segment.SpeakerID {
			flush(false)
		}
		if len(current) > 0 && chunkRunes(current)+1+utf8.RuneCountInString(segment.Text) > TargetChunkRunes {
			flush(true)
		}
		current = append(current, segment)
	}
	flush(false)
	return chunks
}

func DocumentHash(documentType, sourceID, content string, chunks []Chunk) string {
	payload, _ := json.Marshal(struct {
		DocumentType string  `json:"document_type"`
		SourceID     string  `json:"source_id"`
		Content      string  `json:"content"`
		Chunks       []Chunk `json:"chunks"`
	}{documentType, sourceID, content, chunks})
	sum := sha256.Sum256(payload)
	return hex.EncodeToString(sum[:])
}

type Candidate struct {
	ID           string
	DocumentType string
	SourceID     string
	EpisodeID    string
	EpisodeTitle string
	PodcastTitle string
	SpeakerID    string
	SpeakerName  string
	StartMS      *int64
	EndMS        *int64
	Text         string
	Score        float64
}

func Fuse(keyword, semantic []Candidate, limit int) []Candidate {
	if limit <= 0 {
		return []Candidate{}
	}
	byID := make(map[string]Candidate, len(keyword)+len(semantic))
	scores := make(map[string]float64, len(keyword)+len(semantic))
	add := func(items []Candidate) {
		seen := make(map[string]struct{}, len(items))
		for index, item := range items {
			if _, duplicate := seen[item.ID]; duplicate {
				continue
			}
			seen[item.ID] = struct{}{}
			if _, exists := byID[item.ID]; !exists {
				byID[item.ID] = item
			}
			scores[item.ID] += 1 / float64(rrfConstant+index+1)
		}
	}
	add(keyword)
	add(semantic)

	result := make([]Candidate, 0, len(byID))
	for id, item := range byID {
		item.Score = scores[id]
		result = append(result, item)
	}
	sort.Slice(result, func(left, right int) bool {
		if result[left].Score == result[right].Score {
			return result[left].ID < result[right].ID
		}
		return result[left].Score > result[right].Score
	})
	if len(result) > limit {
		result = result[:limit]
	}
	return result
}

func Snippet(text, query string, maxRunes int) string {
	text, query = strings.TrimSpace(text), strings.TrimSpace(query)
	if maxRunes <= 0 || utf8.RuneCountInString(text) <= maxRunes {
		return text
	}
	start := 0
	lowerText, lowerQuery := strings.ToLower(text), strings.ToLower(query)
	if byteIndex := strings.Index(lowerText, lowerQuery); byteIndex >= 0 {
		matchRune := utf8.RuneCountInString(lowerText[:byteIndex])
		start = max(matchRune-maxRunes/3, 0)
	}
	runes := []rune(text)
	if start+maxRunes > len(runes) {
		start = len(runes) - maxRunes
	}
	end := min(start+maxRunes, len(runes))
	prefix, suffix := "", ""
	if start > 0 {
		prefix = "…"
	}
	if end < len(runes) {
		suffix = "…"
	}
	return prefix + string(runes[start:end]) + suffix
}

func chunkFrom(index int, segments []Segment) Chunk {
	parts := make([]string, len(segments))
	for i, segment := range segments {
		parts[i] = segment.Text
	}
	return Chunk{
		Index: index, SpeakerID: segments[0].SpeakerID,
		StartMS: segments[0].StartMS, EndMS: segments[len(segments)-1].EndMS,
		Text: strings.Join(parts, "\n"),
	}
}

func chunkRunes(segments []Segment) int {
	count := max(len(segments)-1, 0)
	for _, segment := range segments {
		count += utf8.RuneCountInString(segment.Text)
	}
	return count
}

func trailingSegments(segments []Segment, limit int) []Segment {
	start, count := len(segments), 0
	for index := len(segments) - 1; index >= 0; index-- {
		segmentRunes := utf8.RuneCountInString(segments[index].Text)
		if segmentRunes > limit || count+segmentRunes+boolInt(start < len(segments)) > limit {
			break
		}
		start, count = index, count+segmentRunes+boolInt(start < len(segments))
	}
	if start == len(segments) {
		return nil
	}
	return append([]Segment(nil), segments[start:]...)
}

func boolInt(value bool) int {
	if value {
		return 1
	}
	return 0
}
