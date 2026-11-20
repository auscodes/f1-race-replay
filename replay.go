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

type session struct {
	Name        string            `json:"name"`
	Caption     string            `json:"caption,omitempty"`
	Drivers     []string          `json:"drivers"`
	DriverNames map[string]string `json:"driverNames,omitempty"`
	Track       []point           `json:"track"`
	DurationMS  int64             `json:"durationMs"`
	Source      *recordingSource  `json:"source,omitempty"`
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
