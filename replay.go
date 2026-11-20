package main

import (
	"encoding/json"
	"fmt"
	"time"
)

type point struct {
	X float64 `json:"x"`
	Y float64 `json:"y"`
}

type position struct {
	Driver string `json:"driver"`
	point
}

type recordingSource struct {
	Provider     string `json:"provider"`
	SessionKey   int    `json:"sessionKey"`
	ClipStartUTC string `json:"clipStartUtc"`
}

type timedValue struct {
	ElapsedMS int64  `json:"elapsedMs"`
	Value     string `json:"value"`
}

type driverTiming struct {
	Laps          []timedValue `json:"laps"`
	Positions     []timedValue `json:"positions"`
	Gaps          []timedValue `json:"gaps"`
	CompletedLaps []lapResult  `json:"completedLaps,omitempty"`
	Intervals     []timedValue `json:"intervals,omitempty"`
	Tyres         []tyreStint  `json:"tyres,omitempty"`
	Outcome       *raceOutcome `json:"outcome,omitempty"`
}

type tyreStint struct {
	LapStart    int    `json:"lapStart"`
	LapEnd      int    `json:"lapEnd"`
	Compound    string `json:"compound"`
	StartingAge *int   `json:"startingAge,omitempty"`
}

type raceOutcome struct {
	ElapsedMS int64  `json:"elapsedMs"`
	Status    string `json:"status"`
	Position  int    `json:"position,omitempty"`
	Gap       string `json:"gap,omitempty"`
}

type gridDriver struct {
	Code  string `json:"code"`
	Team  string `json:"team"`
	Color string `json:"color"`
}

type lapResult struct {
	ElapsedMS  int64 `json:"elapsedMs"`
	Lap        int   `json:"lap"`
	DurationMS int64 `json:"durationMs"`
}

type raceEvent struct {
	ElapsedMS int64  `json:"elapsedMs"`
	Label     string `json:"label"`
	Kind      string `json:"kind"`
	StopMS    int64  `json:"stopMs,omitempty"`
	LaneMS    int64  `json:"laneMs,omitempty"`
}

type raceTimeline struct {
	Events     []raceEvent             `json:"events"`
	Drivers    map[string]driverTiming `json:"drivers"`
	RaceLaps   []timedValue            `json:"raceLaps,omitempty"`
	RaceStates []timedValue            `json:"raceStates,omitempty"`
	TotalLaps  int                     `json:"totalLaps,omitempty"`
}

type session struct {
	Name         string                `json:"name"`
	Caption      string                `json:"caption,omitempty"`
	Drivers      []string              `json:"drivers"`
	DriverNames  map[string]string     `json:"driverNames,omitempty"`
	SectorBreaks []int                 `json:"sectorBreaks,omitempty"`
	Track        []point               `json:"track"`
	DurationMS   int64                 `json:"durationMs"`
	Source       *recordingSource      `json:"source,omitempty"`
	IntervalMS   int64                 `json:"intervalMs"`
	SegmentSize  int64                 `json:"segmentSize"`
	Timeline     *raceTimeline         `json:"timeline,omitempty"`
	Grid         map[string]gridDriver `json:"grid,omitempty"`
}

type recording struct {
	Session    session      `json:"session"`
	IntervalMS int64        `json:"intervalMs"`
	Frames     [][]position `json:"frames"`
}

type frame struct {
	Sequence  int64      `json:"sequence"`
	ElapsedMS int64      `json:"elapsedMs"`
	Positions []position `json:"positions"`
}

func loadRecording(data []byte) (recording, error) {
	var clip recording
	if err := json.Unmarshal(data, &clip); err != nil {
		return recording{}, fmt.Errorf("decode recording: %w", err)
	}
	if clip.IntervalMS <= 0 || len(clip.Frames) == 0 {
		return recording{}, fmt.Errorf("recording needs a positive frame interval and at least one frame")
	}
	if len(clip.Session.Track) < 3 || len(clip.Session.Drivers) == 0 {
		return recording{}, fmt.Errorf("recording needs a track and at least one driver")
	}
	if clip.IntervalMS > (1<<63-1)/int64(time.Millisecond)/int64(len(clip.Frames)) {
		return recording{}, fmt.Errorf("recording is too long for the playback clock")
	}
	for i, positions := range clip.Frames {
		if positions == nil {
			clip.Frames[i] = []position{}
		}
	}
	clip.Session.DurationMS = int64(len(clip.Frames)) * clip.IntervalMS
	clip.Session.IntervalMS = clip.IntervalMS
	segmentMS := int64(60_000)
	if len(clip.Session.Grid) > 0 {
		segmentMS = 20_000
	}
	clip.Session.SegmentSize = max(1, segmentMS/clip.IntervalMS)
	return clip, nil
}

func (clip recording) frameAt(elapsed time.Duration) frame {
	sequence := elapsed.Milliseconds() / clip.IntervalMS
	// Wrap the index to loop the recording while the sequence keeps increasing.
	index := sequence % int64(len(clip.Frames))
	return frame{
		Sequence:  sequence,
		ElapsedMS: index * clip.IntervalMS,
		Positions: clip.Frames[index],
	}
}
