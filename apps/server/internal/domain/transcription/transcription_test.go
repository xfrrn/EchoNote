package transcription

import "testing"

func TestPlanUsesCoreAndOverlap(t *testing.T) {
	windows, err := Plan(3 * CoreWindowMS)
	if err != nil {
		t.Fatal(err)
	}
	if len(windows) != 3 {
		t.Fatalf("chunks=%d, want 3", len(windows))
	}
	middle := windows[1]
	if middle.CoreStartMS != CoreWindowMS || middle.CoreEndMS != 2*CoreWindowMS ||
		middle.RenderStartMS != CoreWindowMS-OverlapMS || middle.RenderEndMS != 2*CoreWindowMS+OverlapMS {
		t.Fatalf("middle=%+v", middle)
	}

	short, err := Plan(CoreWindowMS)
	if err != nil || len(short) != 1 || short[0].RenderStartMS != 0 || short[0].RenderEndMS != CoreWindowMS {
		t.Fatalf("short=%+v err=%v", short, err)
	}
}

func TestAlignMapsSwappedLocalSpeakersAndMergeOwnsBoundaryOnce(t *testing.T) {
	boundary := CoreWindowMS
	chunks := []Chunk{
		{
			ID: "chunk-0", Window: Window{Sequence: 0, CoreStartMS: 0, CoreEndMS: boundary, RenderStartMS: 0, RenderEndMS: boundary + OverlapMS},
			Segments: []Segment{
				{LocalSequence: 0, LocalSpeaker: "0", StartMS: 1000, EndMS: 4000, Text: "first"},
				{LocalSequence: 1, LocalSpeaker: "0", StartMS: boundary - 4000, EndMS: boundary - 1000, Text: "host overlap"},
				{LocalSequence: 2, LocalSpeaker: "1", StartMS: boundary + 1000, EndMS: boundary + 5000, Text: "guest overlap"},
				{LocalSequence: 3, LocalSpeaker: "0", StartMS: boundary - 1000, EndMS: boundary + 1000, Text: "boundary"},
			},
		},
		{
			ID: "chunk-1", Window: Window{Sequence: 1, CoreStartMS: boundary, CoreEndMS: 2 * boundary, RenderStartMS: boundary - OverlapMS, RenderEndMS: 2 * boundary},
			Segments: []Segment{
				{LocalSequence: 0, LocalSpeaker: "9", StartMS: boundary - 4000, EndMS: boundary - 1000, Text: "host overlap"},
				{LocalSequence: 1, LocalSpeaker: "7", StartMS: boundary + 1000, EndMS: boundary + 5000, Text: "guest overlap"},
				{LocalSequence: 2, LocalSpeaker: "9", StartMS: boundary - 1000, EndMS: boundary + 1000, Text: "boundary"},
				{LocalSequence: 3, LocalSpeaker: "7", StartMS: boundary + 6000, EndMS: boundary + 9000, Text: "second"},
			},
		},
	}

	alignment, err := Align(chunks)
	if err != nil {
		t.Fatal(err)
	}
	if alignment.SpeakerMaps[1]["9"] != alignment.SpeakerMaps[0]["0"] ||
		alignment.SpeakerMaps[1]["7"] != alignment.SpeakerMaps[0]["1"] {
		t.Fatalf("speaker maps=%+v", alignment.SpeakerMaps)
	}
	for index := range chunks {
		chunks[index].SpeakerMap = alignment.SpeakerMaps[index]
	}
	segments, err := Merge(chunks)
	if err != nil {
		t.Fatal(err)
	}
	boundaryCount := 0
	for _, segment := range segments {
		if segment.Text == "boundary" {
			boundaryCount++
		}
	}
	if boundaryCount != 1 {
		t.Fatalf("boundary count=%d, segments=%+v", boundaryCount, segments)
	}
}

func TestAlignCreatesSpeakerWithoutOverlapEvidence(t *testing.T) {
	chunks := []Chunk{
		{Window: Window{Sequence: 0, CoreEndMS: CoreWindowMS, RenderEndMS: CoreWindowMS + OverlapMS}, Segments: []Segment{{LocalSpeaker: "0", StartMS: 0, EndMS: 3000, Text: "one"}}},
		{Window: Window{Sequence: 1, CoreStartMS: CoreWindowMS, CoreEndMS: 2 * CoreWindowMS, RenderStartMS: CoreWindowMS - OverlapMS, RenderEndMS: 2 * CoreWindowMS}, Segments: []Segment{{LocalSpeaker: "8", StartMS: CoreWindowMS + 1000, EndMS: CoreWindowMS + 4000, Text: "two"}}},
	}
	alignment, err := Align(chunks)
	if err != nil {
		t.Fatal(err)
	}
	if len(alignment.Speakers) != 2 || len(alignment.LowConfidenceChunk) != 1 || alignment.LowConfidenceChunk[0] != 1 {
		t.Fatalf("alignment=%+v", alignment)
	}
}

func TestSpeakersFromChunksIgnoresOverlapOnlySpeaker(t *testing.T) {
	chunk := Chunk{
		Window: Window{CoreStartMS: 0, CoreEndMS: 100},
		Segments: []Segment{
			{LocalSpeaker: "owned", StartMS: 10, EndMS: 20},
			{LocalSpeaker: "overlap", StartMS: 110, EndMS: 120},
		},
		SpeakerMap: map[string]string{"owned": "speaker-001", "overlap": "speaker-002"},
	}
	speakers := SpeakersFromChunks([]Chunk{chunk})
	if len(speakers) != 1 || speakers[0].StableKey != "speaker-001" {
		t.Fatalf("speakers=%+v", speakers)
	}
}
