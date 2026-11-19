package main

import (
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
		{name: "before first boundary", atMS: 199, sequence: 0, elapsedMS: 0, x: 15},
		{name: "at first boundary", atMS: 200, sequence: 1, elapsedMS: 200, x: 25},
		{name: "at loop boundary", atMS: 400, sequence: 2, elapsedMS: 0, x: 15},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got := clip.frameAt(time.Duration(tc.atMS) * time.Millisecond)
			if got.Sequence != tc.sequence || got.ElapsedMS != tc.elapsedMS || got.Positions[0].X != tc.x {
				t.Fatalf("frame at %d ms = %+v, want sequence %d, elapsed %d, x %v", tc.atMS, got, tc.sequence, tc.elapsedMS, tc.x)
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
