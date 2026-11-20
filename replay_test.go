package main

import (
	"encoding/json"
	"testing"
	"time"
)

func TestFrameAt(t *testing.T) {
	clip := recording{
		IntervalMS: 200,
		Frames: [][]position{
			{{Driver: "01", point: point{X: 15, Y: 20}}},
			{{Driver: "01", point: point{X: 25, Y: 20}}},
		},
	}
	for _, tc := range []struct {
		name      string
		atMS      int64
		sequence  int64
		elapsedMS int64
		x         float64
	}{
		{name: "at startup", atMS: 0, sequence: 0, elapsedMS: 0, x: 15},
		{name: "before first boundary", atMS: 199, sequence: 0, elapsedMS: 0, x: 15},
		{name: "at first boundary", atMS: 200, sequence: 1, elapsedMS: 200, x: 25},
		{name: "before loop boundary", atMS: 399, sequence: 1, elapsedMS: 200, x: 25},
		{name: "at loop boundary", atMS: 400, sequence: 2, elapsedMS: 0, x: 15},
		{name: "after several loops", atMS: 1400, sequence: 7, elapsedMS: 200, x: 25},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got := clip.frameAt(time.Duration(tc.atMS) * time.Millisecond)
			if got.Sequence != tc.sequence || got.ElapsedMS != tc.elapsedMS || got.Positions[0].X != tc.x {
				t.Fatalf("frame at %d ms = %+v, want sequence %d, elapsed %d, x %v", tc.atMS, got, tc.sequence, tc.elapsedMS, tc.x)
			}
		})
	}
}

func TestSingleFrameRecording(t *testing.T) {
	data := []byte(`{
		"session": {"drivers": ["01"], "track": [{"x": 0, "y": 0}, {"x": 40, "y": 0}, {"x": 40, "y": 40}]},
		"intervalMs": 200,
		"frames": [[{"driver": "01", "x": 10, "y": 20}]]
	}`)
	clip, err := loadRecording(data)
	if err != nil {
		t.Fatal(err)
	}
	if clip.Session.DurationMS != 200 {
		t.Fatalf("duration = %d ms, want 200 ms", clip.Session.DurationMS)
	}
	got := clip.frameAt(800 * time.Millisecond)
	if got.Sequence != 4 || got.ElapsedMS != 0 || len(got.Positions) != 1 || got.Positions[0].X != 10 {
		t.Fatalf("frame after four loops = %+v, want sequence 4 and the original position", got)
	}
}

func TestLoadRecordingRejectsInvalidData(t *testing.T) {
	t.Run("malformed JSON", func(t *testing.T) {
		if _, err := loadRecording([]byte(`{"session":`)); err == nil {
			t.Fatal("expected malformed JSON to be rejected")
		}
	})

	valid, err := loadRecording(demo)
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		name   string
		change func(*recording)
	}{
		{name: "zero frame interval", change: func(clip *recording) { clip.IntervalMS = 0 }},
		{name: "negative frame interval", change: func(clip *recording) { clip.IntervalMS = -1 }},
		{name: "no frames", change: func(clip *recording) { clip.Frames = nil }},
		{name: "incomplete track", change: func(clip *recording) { clip.Session.Track = clip.Session.Track[:2] }},
		{name: "no drivers", change: func(clip *recording) { clip.Session.Drivers = nil }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			clip := valid
			tc.change(&clip)
			data, err := json.Marshal(clip)
			if err != nil {
				t.Fatal(err)
			}
			if _, err := loadRecording(data); err == nil {
				t.Fatal("expected invalid recording to be rejected")
			}
		})
	}
}

func TestDemoRecording(t *testing.T) {
	clip, err := loadRecording(demo)
	if err != nil {
		t.Fatal(err)
	}
	for i, positions := range clip.Frames {
		if len(positions) != len(clip.Session.Drivers) {
			t.Fatalf("frame %d has %d positions for %d drivers", i, len(positions), len(clip.Session.Drivers))
		}
		for j, position := range positions {
			if position.Driver != clip.Session.Drivers[j] {
				t.Fatalf("frame %d position %d has unexpected driver %q", i, j, position.Driver)
			}
		}
	}
}
