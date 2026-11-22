package main

import (
	"bytes"
	"compress/gzip"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestReadCompressedRecordingPreservesID(t *testing.T) {
	plain, err := readRecording(t.Context(), "")
	if err != nil {
		t.Fatal(err)
	}
	var data bytes.Buffer
	writer := gzip.NewWriter(&data)
	if _, err := writer.Write(demo); err != nil {
		t.Fatal(err)
	}
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(t.TempDir(), "race.json.gz")
	if err := os.WriteFile(path, data.Bytes(), 0600); err != nil {
		t.Fatal(err)
	}
	clip, err := readRecording(t.Context(), path)
	if err != nil || clip.id != plain.id || len(clip.Frames) != len(plain.Frames) {
		t.Fatalf("compressed recording changed its identity or frame count: %v", err)
	}
	if err := os.WriteFile(path, data.Bytes()[:data.Len()-1], 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := readRecording(t.Context(), path); err == nil || !strings.Contains(err.Error(), "decompress recording") {
		t.Fatalf("expected a decompression error, got %v", err)
	}
}

func TestReadRecordingFromFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "race.json")
	data := []byte(`{
		"session": {
			"name": "Historical clip", "caption": "Pit stop replay",
			"drivers": ["44"], "driverNames": {"44": "Lewis Hamilton"},
			"source": {"provider": "OpenF1", "sessionKey": 11307, "clipStartUtc": "2026-06-14T14:01:08.000+00:00"},
			"track": [{"x": 0, "y": 0}, {"x": 50, "y": 0}, {"x": 25, "y": 25}]
		},
		"intervalMs": 200,
		"frames": [null, [{"driver": "44", "x": 15, "y": 20}]]
	}`)
	if err := os.WriteFile(path, data, 0600); err != nil {
		t.Fatal(err)
	}
	clip, err := readRecording(t.Context(), path)
	if err != nil {
		t.Fatal(err)
	}
	if clip.Session.Caption != "Pit stop replay" || clip.Session.DriverNames["44"] != "Lewis Hamilton" ||
		clip.Session.Source == nil || clip.Session.Source.SessionKey != 11307 || clip.Session.DurationMS != 400 {
		t.Fatalf("unexpected session metadata: %+v", clip.Session)
	}
	if frame := clip.frameAt(0); frame.Positions == nil || len(frame.Positions) != 0 {
		t.Fatalf("missing positions should be an empty array, got %+v", frame.Positions)
	}
}

func TestReadRecordingFileErrors(t *testing.T) {
	path := filepath.Join(t.TempDir(), "race.json")
	if _, err := readRecording(t.Context(), path); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("missing recording error = %v, want os.ErrNotExist", err)
	}
	if err := os.WriteFile(path, []byte(`{"invalid":`), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := readRecording(t.Context(), path); err == nil || !strings.Contains(err.Error(), path) {
		t.Fatalf("invalid recording error = %v, want the filename in the error", err)
	}
}
