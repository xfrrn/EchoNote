package ai

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"
	"unicode/utf8"
)

const (
	ArtifactTypeEpisodeSummary = "episode_summary"
	ArtifactPromptVersion      = "episode-summary-v1"
	CitationOpen               = "<ECHONOTE_CITATIONS>"
	CitationClose              = "</ECHONOTE_CITATIONS>"
	maxArtifactInputRunes      = 200_000
	maxAnswerRunes             = 30_000
	maxCitationEnvelopeBytes   = 8 << 10
)

type Usage struct {
	InputTokens  int `json:"input_tokens"`
	OutputTokens int `json:"output_tokens"`
}

type Message struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type StructuredGenerationRequest struct {
	Messages  []Message
	MaxTokens int
}

type StructuredGenerationResult struct {
	Content string
	Usage   Usage
}

type ChatRequest struct {
	Messages  []Message
	MaxTokens int
}

type ChatEvent struct {
	Delta string
	Usage *Usage
	Err   error
}

type LLMProvider interface {
	GenerateStructured(context.Context, StructuredGenerationRequest) (StructuredGenerationResult, error)
	StreamChat(context.Context, ChatRequest) (<-chan ChatEvent, error)
	Model() string
}

type Speaker struct {
	ID   string `json:"id"`
	Name string `json:"name"`
	Role string `json:"role,omitempty"`
}

type Segment struct {
	ID        string `json:"id"`
	SpeakerID string `json:"speaker_id"`
	StartMS   int64  `json:"start_ms"`
	EndMS     int64  `json:"end_ms"`
	Text      string `json:"text"`
}

type Note struct {
	ID      string `json:"id"`
	Content string `json:"content"`
}

type EpisodeInput struct {
	EpisodeID           string    `json:"episode_id"`
	EpisodeTitle        string    `json:"episode_title"`
	TranscriptVersionID string    `json:"transcript_version_id"`
	Speakers            []Speaker `json:"speakers"`
	Segments            []Segment `json:"segments"`
	Notes               []Note    `json:"notes"`
}

func (input EpisodeInput) Validate() error {
	if input.EpisodeID == "" || input.EpisodeTitle == "" || input.TranscriptVersionID == "" || len(input.Segments) == 0 {
		return errors.New("AI input requires an episode and active transcript")
	}
	raw, err := json.Marshal(input)
	if err != nil {
		return err
	}
	if utf8.RuneCount(raw) > maxArtifactInputRunes {
		return errors.New("AI input exceeds the supported size")
	}
	return nil
}

func (input EpisodeInput) NotesRevision() string {
	return hashJSON(input.Notes)
}

func (input EpisodeInput) InputHash() string {
	return hashJSON(input)
}

func (input EpisodeInput) JSON() ([]byte, error) {
	if err := input.Validate(); err != nil {
		return nil, err
	}
	return json.Marshal(input)
}

type SpeakerView struct {
	SpeakerID   string   `json:"speaker_id"`
	SpeakerName string   `json:"speaker_name"`
	Points      []string `json:"points"`
}

type WorthReviewing struct {
	TranscriptSegmentID string `json:"transcript_segment_id"`
	SpeakerID           string `json:"speaker_id"`
	SpeakerName         string `json:"speaker_name"`
	StartMS             int64  `json:"start_ms"`
	EndMS               int64  `json:"end_ms"`
	Quote               string `json:"quote"`
	Reason              string `json:"reason"`
}

type NoteConnection struct {
	NoteID  string `json:"note_id"`
	Note    string `json:"note"`
	Insight string `json:"insight"`
}

type ArtifactResult struct {
	OneSentenceSummary string           `json:"one_sentence_summary"`
	KeyPoints          []string         `json:"key_points"`
	SpeakerViews       []SpeakerView    `json:"speaker_views"`
	WorthReviewing     []WorthReviewing `json:"worth_reviewing"`
	NoteConnections    []NoteConnection `json:"note_connections"`
}

type artifactDraft struct {
	OneSentenceSummary string   `json:"one_sentence_summary"`
	KeyPoints          []string `json:"key_points"`
	SpeakerViews       []struct {
		SpeakerID string   `json:"speaker_id"`
		Points    []string `json:"points"`
	} `json:"speaker_views"`
	WorthReviewing []struct {
		TranscriptSegmentID string `json:"transcript_segment_id"`
		Reason              string `json:"reason"`
	} `json:"worth_reviewing"`
	NoteConnections []struct {
		NoteID  string `json:"note_id"`
		Insight string `json:"insight"`
	} `json:"note_connections"`
}

func ValidateArtifact(raw []byte, input EpisodeInput) (ArtifactResult, error) {
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	var draft artifactDraft
	if err := decoder.Decode(&draft); err != nil {
		return ArtifactResult{}, fmt.Errorf("decode AI artifact: %w", err)
	}
	if err := decoder.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		return ArtifactResult{}, errors.New("AI artifact contains trailing data")
	}
	if !validText(draft.OneSentenceSummary, 1_000) || len(draft.KeyPoints) < 1 || len(draft.KeyPoints) > 12 {
		return ArtifactResult{}, errors.New("AI artifact summary or key points are invalid")
	}
	for _, point := range draft.KeyPoints {
		if !validText(point, 2_000) {
			return ArtifactResult{}, errors.New("AI artifact contains an invalid key point")
		}
	}

	speakers := make(map[string]Speaker, len(input.Speakers))
	for _, speaker := range input.Speakers {
		speakers[speaker.ID] = speaker
	}
	segments := make(map[string]Segment, len(input.Segments))
	for _, segment := range input.Segments {
		segments[segment.ID] = segment
	}
	notes := make(map[string]Note, len(input.Notes))
	for _, note := range input.Notes {
		notes[note.ID] = note
	}

	result := ArtifactResult{
		OneSentenceSummary: strings.TrimSpace(draft.OneSentenceSummary),
		KeyPoints:          trimStrings(draft.KeyPoints),
		SpeakerViews:       make([]SpeakerView, 0, len(draft.SpeakerViews)),
		WorthReviewing:     make([]WorthReviewing, 0, len(draft.WorthReviewing)),
		NoteConnections:    make([]NoteConnection, 0, len(draft.NoteConnections)),
	}
	if len(draft.SpeakerViews) > 20 || len(draft.WorthReviewing) > 12 || len(draft.NoteConnections) > 12 {
		return ArtifactResult{}, errors.New("AI artifact contains too many items")
	}
	seenSpeakers := map[string]struct{}{}
	for _, view := range draft.SpeakerViews {
		speaker, ok := speakers[view.SpeakerID]
		if !ok || len(view.Points) == 0 || len(view.Points) > 8 {
			return ArtifactResult{}, errors.New("AI artifact references an invalid speaker")
		}
		if _, duplicate := seenSpeakers[view.SpeakerID]; duplicate {
			return ArtifactResult{}, errors.New("AI artifact repeats a speaker")
		}
		seenSpeakers[view.SpeakerID] = struct{}{}
		for _, point := range view.Points {
			if !validText(point, 2_000) {
				return ArtifactResult{}, errors.New("AI artifact contains an invalid speaker point")
			}
		}
		result.SpeakerViews = append(result.SpeakerViews, SpeakerView{
			SpeakerID: speaker.ID, SpeakerName: speaker.Name, Points: trimStrings(view.Points),
		})
	}
	seenSegments := map[string]struct{}{}
	for _, item := range draft.WorthReviewing {
		segment, ok := segments[item.TranscriptSegmentID]
		if !ok || !validText(item.Reason, 2_000) {
			return ArtifactResult{}, errors.New("AI artifact references an invalid transcript segment")
		}
		if _, duplicate := seenSegments[item.TranscriptSegmentID]; duplicate {
			return ArtifactResult{}, errors.New("AI artifact repeats a transcript segment")
		}
		seenSegments[item.TranscriptSegmentID] = struct{}{}
		speaker := speakers[segment.SpeakerID]
		result.WorthReviewing = append(result.WorthReviewing, WorthReviewing{
			TranscriptSegmentID: segment.ID, SpeakerID: speaker.ID, SpeakerName: speaker.Name,
			StartMS: segment.StartMS, EndMS: segment.EndMS, Quote: segment.Text, Reason: strings.TrimSpace(item.Reason),
		})
	}
	seenNotes := map[string]struct{}{}
	for _, item := range draft.NoteConnections {
		note, ok := notes[item.NoteID]
		if !ok || !validText(item.Insight, 2_000) {
			return ArtifactResult{}, errors.New("AI artifact references an invalid note")
		}
		if _, duplicate := seenNotes[item.NoteID]; duplicate {
			return ArtifactResult{}, errors.New("AI artifact repeats a note")
		}
		seenNotes[item.NoteID] = struct{}{}
		result.NoteConnections = append(result.NoteConnections, NoteConnection{
			NoteID: note.ID, Note: note.Content, Insight: strings.TrimSpace(item.Insight),
		})
	}
	return result, nil
}

func (result ArtifactResult) SearchText() string {
	parts := []string{result.OneSentenceSummary}
	parts = append(parts, result.KeyPoints...)
	for _, view := range result.SpeakerViews {
		parts = append(parts, view.SpeakerName, strings.Join(view.Points, "\n"))
	}
	for _, item := range result.WorthReviewing {
		parts = append(parts, item.Quote, item.Reason)
	}
	for _, item := range result.NoteConnections {
		parts = append(parts, item.Note, item.Insight)
	}
	return strings.Join(parts, "\n")
}

type CitationSource struct {
	Key         string `json:"key"`
	SourceType  string `json:"source_type"`
	SourceID    string `json:"source_id"`
	Excerpt     string `json:"excerpt"`
	SpeakerID   string `json:"speaker_id,omitempty"`
	SpeakerName string `json:"speaker_name,omitempty"`
	StartMS     *int64 `json:"start_ms,omitempty"`
	EndMS       *int64 `json:"end_ms,omitempty"`
}

type CitationParser struct {
	pending  string
	answer   strings.Builder
	envelope strings.Builder
	inBlock  bool
	closed   bool
}

func (parser *CitationParser) Push(delta string) (string, error) {
	if parser.closed {
		if strings.TrimSpace(delta) != "" {
			return "", errors.New("chat response contains text after citations")
		}
		return "", nil
	}
	if parser.inBlock {
		parser.envelope.WriteString(delta)
		return "", parser.closeEnvelope()
	}
	parser.pending += delta
	if index := strings.Index(parser.pending, CitationOpen); index >= 0 {
		content := parser.pending[:index]
		parser.answer.WriteString(content)
		if utf8.RuneCountInString(parser.answer.String()) > maxAnswerRunes {
			return "", errors.New("chat answer exceeds size limit")
		}
		parser.envelope.WriteString(parser.pending[index+len(CitationOpen):])
		parser.pending = ""
		parser.inBlock = true
		return content, parser.closeEnvelope()
	}
	hold := len(CitationOpen) - 1
	if len(parser.pending) <= hold {
		return "", nil
	}
	cut := len(parser.pending) - hold
	for cut > 0 && !utf8.ValidString(parser.pending[:cut]) {
		cut--
	}
	content := parser.pending[:cut]
	parser.pending = parser.pending[cut:]
	parser.answer.WriteString(content)
	if utf8.RuneCountInString(parser.answer.String()) > maxAnswerRunes {
		return "", errors.New("chat answer exceeds size limit")
	}
	return content, nil
}

func (parser *CitationParser) Finish(allowed map[string]CitationSource) (string, []CitationSource, error) {
	if !parser.closed {
		return "", nil, errors.New("chat response omitted a complete citation block")
	}
	answer := strings.TrimSpace(parser.answer.String())
	if answer == "" || utf8.RuneCountInString(answer) > maxAnswerRunes {
		return "", nil, errors.New("chat response has an invalid answer")
	}
	var payload struct {
		IDs []string `json:"ids"`
	}
	decoder := json.NewDecoder(strings.NewReader(parser.envelope.String()))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&payload); err != nil || len(payload.IDs) == 0 || len(payload.IDs) > 8 {
		return "", nil, errors.New("chat response has invalid citations")
	}
	if err := decoder.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		return "", nil, errors.New("chat response has invalid citations")
	}
	citations := make([]CitationSource, 0, len(payload.IDs))
	seen := make(map[string]struct{}, len(payload.IDs))
	for _, id := range payload.IDs {
		source, ok := allowed[id]
		if !ok {
			return "", nil, errors.New("chat response cited material outside the retrieval set")
		}
		if _, duplicate := seen[id]; duplicate {
			continue
		}
		seen[id] = struct{}{}
		citations = append(citations, source)
	}
	if len(citations) == 0 {
		return "", nil, errors.New("chat response has no valid citations")
	}
	return answer, citations, nil
}

func (parser *CitationParser) closeEnvelope() error {
	if parser.envelope.Len() > maxCitationEnvelopeBytes {
		return errors.New("chat citation block exceeds size limit")
	}
	value := parser.envelope.String()
	index := strings.Index(value, CitationClose)
	if index < 0 {
		return nil
	}
	if strings.TrimSpace(value[index+len(CitationClose):]) != "" {
		return errors.New("chat response contains text after citations")
	}
	parser.envelope.Reset()
	parser.envelope.WriteString(strings.TrimSpace(value[:index]))
	parser.closed = true
	return nil
}

func hashJSON(value any) string {
	raw, _ := json.Marshal(value)
	sum := sha256.Sum256(raw)
	return hex.EncodeToString(sum[:])
}

func validText(value string, maxRunes int) bool {
	value = strings.TrimSpace(value)
	count := utf8.RuneCountInString(value)
	return count > 0 && count <= maxRunes
}

func trimStrings(values []string) []string {
	trimmed := make([]string, len(values))
	for index, value := range values {
		trimmed[index] = strings.TrimSpace(value)
	}
	return trimmed
}
