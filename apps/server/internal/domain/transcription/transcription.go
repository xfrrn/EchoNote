package transcription

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"sort"
	"strings"
	"time"
)

const (
	CoreWindowMS      int64 = 90 * 60 * 1000
	OverlapMS         int64 = 5 * 60 * 1000
	MinAlignmentMS    int64 = 2000
	MinAlignmentRatio       = 0.50
)

type Window struct {
	Sequence      int   `json:"sequence"`
	CoreStartMS   int64 `json:"core_start_ms"`
	CoreEndMS     int64 `json:"core_end_ms"`
	RenderStartMS int64 `json:"render_start_ms"`
	RenderEndMS   int64 `json:"render_end_ms"`
}

func Plan(durationMS int64) ([]Window, error) {
	if durationMS <= 0 {
		return nil, errors.New("audio duration must be positive")
	}
	windows := make([]Window, 0, (durationMS+CoreWindowMS-1)/CoreWindowMS)
	for coreStart, sequence := int64(0), 0; coreStart < durationMS; coreStart, sequence = coreStart+CoreWindowMS, sequence+1 {
		coreEnd := min(durationMS, coreStart+CoreWindowMS)
		renderStart, renderEnd := coreStart, coreEnd
		if durationMS > CoreWindowMS {
			renderStart = max(0, coreStart-OverlapMS)
			renderEnd = min(durationMS, coreEnd+OverlapMS)
		}
		windows = append(windows, Window{
			Sequence: sequence, CoreStartMS: coreStart, CoreEndMS: coreEnd,
			RenderStartMS: renderStart, RenderEndMS: renderEnd,
		})
	}
	return windows, nil
}

type Word struct {
	StartMS     int64  `json:"start_ms"`
	EndMS       int64  `json:"end_ms"`
	Text        string `json:"text"`
	Punctuation string `json:"punctuation,omitempty"`
}

type Segment struct {
	LocalSequence int    `json:"local_sequence"`
	LocalSpeaker  string `json:"local_speaker"`
	StartMS       int64  `json:"start_ms"`
	EndMS         int64  `json:"end_ms"`
	Text          string `json:"text"`
	Words         []Word `json:"words"`
}

type Result struct {
	Segments []Segment `json:"segments"`
}

type RawResult struct {
	Raw      json.RawMessage
	Segments []Segment
}

func ToEpisodeTime(result RawResult, renderStartMS, durationMS int64) Result {
	segments := make([]Segment, 0, len(result.Segments))
	for _, segment := range result.Segments {
		segment.Text = strings.TrimSpace(segment.Text)
		segment.StartMS = max(0, min(durationMS, renderStartMS+segment.StartMS))
		segment.EndMS = max(0, min(durationMS, renderStartMS+segment.EndMS))
		if segment.Text == "" || segment.EndMS <= segment.StartMS {
			continue
		}
		for index := range segment.Words {
			segment.Words[index].StartMS = max(0, min(durationMS, renderStartMS+segment.Words[index].StartMS))
			segment.Words[index].EndMS = max(0, min(durationMS, renderStartMS+segment.Words[index].EndMS))
		}
		segments = append(segments, segment)
	}
	sort.SliceStable(segments, func(i, j int) bool {
		if segments[i].StartMS != segments[j].StartMS {
			return segments[i].StartMS < segments[j].StartMS
		}
		return segments[i].LocalSequence < segments[j].LocalSequence
	})
	return Result{Segments: segments}
}

type Chunk struct {
	ID         string            `json:"id"`
	Window     Window            `json:"window"`
	Segments   []Segment         `json:"segments"`
	SpeakerMap map[string]string `json:"speaker_map,omitempty"`
}

type Speaker struct {
	StableKey   string `json:"stable_key"`
	DisplayName string `json:"display_name"`
}

type Alignment struct {
	Speakers           []Speaker           `json:"speakers"`
	SpeakerMaps        []map[string]string `json:"speaker_maps"`
	LowConfidenceChunk []int               `json:"low_confidence_chunks"`
}

type alignmentCandidate struct {
	local, global string
	matched       int64
	ratio         float64
}

func Align(chunks []Chunk) (Alignment, error) {
	if len(chunks) == 0 {
		return Alignment{}, errors.New("at least one chunk is required")
	}
	sort.Slice(chunks, func(i, j int) bool { return chunks[i].Window.Sequence < chunks[j].Window.Sequence })
	result := Alignment{SpeakerMaps: make([]map[string]string, len(chunks))}
	newSpeaker := func() string {
		key := fmt.Sprintf("speaker-%03d", len(result.Speakers)+1)
		result.Speakers = append(result.Speakers, Speaker{StableKey: key, DisplayName: "Speaker " + alphabeticLabel(len(result.Speakers))})
		return key
	}

	for index, chunk := range chunks {
		locals := localSpeakers(chunk.Segments)
		mapping := make(map[string]string, len(locals))
		lowConfidence := false
		if index > 0 {
			previous := chunks[index-1]
			overlapStart := max(chunk.Window.RenderStartMS, previous.Window.RenderStartMS)
			overlapEnd := min(chunk.Window.RenderEndMS, previous.Window.RenderEndMS)
			candidates := alignmentCandidates(chunk.Segments, previous.Segments, result.SpeakerMaps[index-1], overlapStart, overlapEnd)
			usedGlobals := make(map[string]bool)
			for _, candidate := range candidates {
				if _, used := mapping[candidate.local]; used || usedGlobals[candidate.global] {
					continue
				}
				if candidate.matched < MinAlignmentMS || candidate.ratio < MinAlignmentRatio {
					continue
				}
				mapping[candidate.local] = candidate.global
				usedGlobals[candidate.global] = true
			}
		}
		for _, local := range locals {
			if _, exists := mapping[local]; !exists {
				mapping[local] = newSpeaker()
				lowConfidence = index > 0
			}
		}
		if lowConfidence {
			result.LowConfidenceChunk = append(result.LowConfidenceChunk, chunk.Window.Sequence)
		}
		result.SpeakerMaps[index] = mapping
	}
	return result, nil
}

func alignmentCandidates(current, previous []Segment, previousMap map[string]string, overlapStart, overlapEnd int64) []alignmentCandidate {
	evidence := make(map[string]int64)
	scores := make(map[string]map[string]int64)
	for _, currentSegment := range current {
		currentDuration := clippedDuration(currentSegment.StartMS, currentSegment.EndMS, overlapStart, overlapEnd)
		if currentDuration == 0 {
			continue
		}
		evidence[currentSegment.LocalSpeaker] += currentDuration
		for _, previousSegment := range previous {
			global := previousMap[previousSegment.LocalSpeaker]
			if global == "" {
				continue
			}
			intersection := intervalIntersection(
				max(currentSegment.StartMS, overlapStart), min(currentSegment.EndMS, overlapEnd),
				max(previousSegment.StartMS, overlapStart), min(previousSegment.EndMS, overlapEnd),
			)
			if intersection == 0 {
				continue
			}
			if scores[currentSegment.LocalSpeaker] == nil {
				scores[currentSegment.LocalSpeaker] = make(map[string]int64)
			}
			scores[currentSegment.LocalSpeaker][global] += intersection
		}
	}

	var candidates []alignmentCandidate
	for local, byGlobal := range scores {
		for global, matched := range byGlobal {
			ratio := min(1, float64(matched)/float64(evidence[local]))
			candidates = append(candidates, alignmentCandidate{local: local, global: global, matched: matched, ratio: ratio})
		}
	}
	sort.Slice(candidates, func(i, j int) bool {
		if candidates[i].matched != candidates[j].matched {
			return candidates[i].matched > candidates[j].matched
		}
		if candidates[i].ratio != candidates[j].ratio {
			return candidates[i].ratio > candidates[j].ratio
		}
		if candidates[i].local != candidates[j].local {
			return candidates[i].local < candidates[j].local
		}
		return candidates[i].global < candidates[j].global
	})
	return candidates
}

func localSpeakers(segments []Segment) []string {
	first := make(map[string]int64)
	for _, segment := range segments {
		if _, exists := first[segment.LocalSpeaker]; !exists {
			first[segment.LocalSpeaker] = segment.StartMS
		}
	}
	locals := make([]string, 0, len(first))
	for local := range first {
		locals = append(locals, local)
	}
	sort.Slice(locals, func(i, j int) bool {
		if first[locals[i]] != first[locals[j]] {
			return first[locals[i]] < first[locals[j]]
		}
		return locals[i] < locals[j]
	})
	return locals
}

type MergedSegment struct {
	SpeakerKey    string `json:"speaker_key"`
	Sequence      int    `json:"sequence"`
	StartMS       int64  `json:"start_ms"`
	EndMS         int64  `json:"end_ms"`
	Text          string `json:"text"`
	Words         []Word `json:"words"`
	SourceChunkID string `json:"source_chunk_id"`
	chunkSequence int
	localSequence int
}

func Merge(chunks []Chunk) ([]MergedSegment, error) {
	var merged []MergedSegment
	for _, chunk := range chunks {
		if len(chunk.SpeakerMap) == 0 {
			return nil, fmt.Errorf("chunk %d has no speaker map", chunk.Window.Sequence)
		}
		for _, segment := range chunk.Segments {
			if !ownedByCore(chunk.Window, segment) {
				continue
			}
			speaker := chunk.SpeakerMap[segment.LocalSpeaker]
			if speaker == "" {
				return nil, fmt.Errorf("chunk %d local speaker %q is not mapped", chunk.Window.Sequence, segment.LocalSpeaker)
			}
			merged = append(merged, MergedSegment{
				SpeakerKey: speaker, StartMS: segment.StartMS, EndMS: segment.EndMS,
				Text: segment.Text, Words: segment.Words, SourceChunkID: chunk.ID,
				chunkSequence: chunk.Window.Sequence, localSequence: segment.LocalSequence,
			})
		}
	}
	sort.SliceStable(merged, func(i, j int) bool {
		if merged[i].StartMS != merged[j].StartMS {
			return merged[i].StartMS < merged[j].StartMS
		}
		if merged[i].EndMS != merged[j].EndMS {
			return merged[i].EndMS < merged[j].EndMS
		}
		if merged[i].chunkSequence != merged[j].chunkSequence {
			return merged[i].chunkSequence < merged[j].chunkSequence
		}
		return merged[i].localSequence < merged[j].localSequence
	})
	for index := range merged {
		merged[index].Sequence = index
	}
	return merged, nil
}

func SpeakersFromChunks(chunks []Chunk) []Speaker {
	keys := make(map[string]bool)
	for _, chunk := range chunks {
		for _, segment := range chunk.Segments {
			if !ownedByCore(chunk.Window, segment) {
				continue
			}
			key := chunk.SpeakerMap[segment.LocalSpeaker]
			if key != "" {
				keys[key] = true
			}
		}
	}
	ordered := make([]string, 0, len(keys))
	for key := range keys {
		ordered = append(ordered, key)
	}
	sort.Strings(ordered)
	speakers := make([]Speaker, 0, len(ordered))
	for index, key := range ordered {
		speakers = append(speakers, Speaker{StableKey: key, DisplayName: "Speaker " + alphabeticLabel(index)})
	}
	return speakers
}

func ownedByCore(window Window, segment Segment) bool {
	midpoint := segment.StartMS + (segment.EndMS-segment.StartMS)/2
	return midpoint >= window.CoreStartMS && midpoint < window.CoreEndMS
}

func clippedDuration(start, end, clipStart, clipEnd int64) int64 {
	return intervalIntersection(max(start, clipStart), min(end, clipEnd), clipStart, clipEnd)
}

func intervalIntersection(startA, endA, startB, endB int64) int64 {
	return max(0, min(endA, endB)-max(startA, startB))
}

func alphabeticLabel(index int) string {
	label := ""
	for index >= 0 {
		label = string(rune('A'+index%26)) + label
		index = index/26 - 1
	}
	return label
}

type Request struct {
	AudioURL        string
	AudioDurationMS int64
	Model           string
	LanguageHint    string
	SpeakerCount    int
}

type ExternalTask struct {
	ID string
}

type TaskState string

const (
	TaskPending   TaskState = "pending"
	TaskRunning   TaskState = "running"
	TaskSucceeded TaskState = "succeeded"
	TaskFailed    TaskState = "failed"
	TaskCanceled  TaskState = "canceled"
)

type ExternalTaskStatus struct {
	State     TaskState
	ResultURL string
	Code      string
	Message   string
}

type ASRProvider interface {
	Submit(context.Context, Request) (ExternalTask, error)
	Poll(context.Context, string) (ExternalTaskStatus, error)
	FetchResult(context.Context, string) (RawResult, error)
}

type ObjectStore interface {
	Put(context.Context, string, io.Reader) error
	Delete(context.Context, string) error
	SignedURL(context.Context, string, time.Duration) (string, error)
}
