package main

import (
	"bytes"
	"compress/gzip"
	"context"
	"embed"
	"encoding/json"
	"flag"
	"fmt"
	"io/fs"
	"log"
	"mime"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"
)

//go:embed testdata/demo.json
var demo []byte

//go:embed web
var web embed.FS

func readRecording(ctx context.Context, location string) (recording, error) {
	if location == "" {
		return loadRecording(demo)
	}
	var data []byte
	var err error
	if strings.HasPrefix(location, "s3://") {
		data, err = readS3Object(ctx, location)
	} else {
		data, err = os.ReadFile(location)
	}
	if err != nil {
		return recording{}, fmt.Errorf("read recording %q: %w", location, err)
	}
	clip, err := loadRecording(data)
	if err != nil {
		return recording{}, fmt.Errorf("recording %q: %w", location, err)
	}
	return clip, nil
}

func replayRoutes(clip recording, startedAt time.Time) *http.ServeMux {
	mux := http.NewServeMux()
	metadata := clip.Session
	if len(metadata.Grid) > 0 && metadata.Timeline != nil {
		// Keep the lap and event selectors usable before the full timing feed arrives.
		reference := metadata.Drivers[0]
		metadata.Timeline = &raceTimeline{
			Events: clip.Session.Timeline.Events,
			Drivers: map[string]driverTiming{
				reference: {Laps: clip.Session.Timeline.Drivers[reference].Laps},
			},
		}
		mux.HandleFunc("GET /api/timing", raceTimingHandler(clip.Session.Timeline))
	}
	mux.HandleFunc("GET /api/session", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if err := json.NewEncoder(w).Encode(metadata); err != nil {
			log.Printf("send session metadata: %v", err)
		}
	})
	mux.HandleFunc("GET /api/segments/{index}", func(w http.ResponseWriter, r *http.Request) {
		index, err := strconv.ParseInt(r.PathValue("index"), 10, 64)
		if err != nil || index < 0 {
			http.Error(w, "invalid segment index", http.StatusBadRequest)
			return
		}
		size, count := clip.Session.SegmentSize, int64(len(clip.Frames))
		if size < 1 || count == 0 || index > (count-1)/size {
			http.NotFound(w, r)
			return
		}
		start := index * size
		w.Header().Set("Content-Type", "application/json")
		if err := json.NewEncoder(w).Encode(struct {
			StartFrame int64        `json:"startFrame"`
			Frames     [][]position `json:"frames"`
		}{StartFrame: start, Frames: clip.Frames[start:min(start+size, count)]}); err != nil {
			log.Printf("send replay segment: %v", err)
		}
	})
	stream := frameStream{clip: clip, startedAt: startedAt}
	mux.HandleFunc("GET /ws", stream.serve)
	return mux
}

func raceTimingHandler(timeline *raceTimeline) http.HandlerFunc {
	// The archive is immutable. Reuse the same encoded payloads for all viewers.
	payload, err := json.Marshal(timeline)
	if err != nil {
		log.Printf("encode race timing: %v", err)
		return func(w http.ResponseWriter, _ *http.Request) {
			http.Error(w, "race timing unavailable", http.StatusInternalServerError)
		}
	}
	var compressed bytes.Buffer
	zip := gzip.NewWriter(&compressed)
	_, _ = zip.Write(payload) // bytes.Buffer writes do not fail.
	_ = zip.Close()
	gzipped := compressed.Bytes()

	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("Vary", "Accept-Encoding")
		for coding := range strings.SplitSeq(strings.Join(r.Header.Values("Accept-Encoding"), ","), ",") {
			name, params, err := mime.ParseMediaType(coding)
			if err != nil || name != "gzip" {
				continue
			}
			if q, present := params["q"]; present {
				quality, err := strconv.ParseFloat(q, 64)
				if err != nil || !(quality > 0 && quality <= 1) {
					continue
				}
			}
			w.Header().Set("Content-Encoding", "gzip")
			_, _ = w.Write(gzipped)
			return
		}
		_, _ = w.Write(payload)
	}
}

func main() {
	addr := flag.String("addr", "127.0.0.1:8080", "address to listen on")
	recordingLocation := flag.String("recording", "", "local path or s3://bucket/key for a prepared JSON recording")
	flag.Parse()

	loadCtx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	clip, err := readRecording(loadCtx, *recordingLocation)
	cancel()
	if err != nil {
		log.Fatal(err)
	}
	viewer, err := fs.Sub(web, "web")
	if err != nil {
		log.Fatal(err)
	}
	mux := replayRoutes(clip, time.Now())
	mux.Handle("GET /", http.FileServerFS(viewer))
	server := http.Server{
		Addr:              *addr,
		Handler:           mux,
		ReadHeaderTimeout: 5 * time.Second,
	}
	log.Printf("F1 Race Replay at http://%s", *addr)
	log.Fatal(server.ListenAndServe())
}
