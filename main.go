package main

import (
	"bytes"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"embed"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"io/fs"
	"log"
	"mime"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

//go:embed testdata/demo.json
var demo []byte

//go:embed web
var web embed.FS

func readRecording(ctx context.Context, location string) (recording, error) {
	readStart := time.Now()
	var data []byte
	var err error
	source := "file"
	if location == "" {
		source = "embedded"
		data = demo
	} else if strings.HasPrefix(location, "s3://") {
		source = "s3"
		data, err = readS3Object(ctx, location)
	} else {
		data, err = os.ReadFile(location)
	}
	if err != nil {
		return recording{}, fmt.Errorf("read recording %q: %w", location, err)
	}
	readTime := time.Since(readStart)
	storedBytes := len(data)
	unpackStart := time.Now()
	if len(data) >= 2 && data[0] == 0x1f && data[1] == 0x8b {
		reader, err := gzip.NewReader(bytes.NewReader(data))
		if err != nil {
			return recording{}, fmt.Errorf("open gzip recording %q: %w", location, err)
		}
		defer reader.Close()
		data, err = io.ReadAll(reader)
		if err != nil {
			return recording{}, fmt.Errorf("decompress recording %q: %w", location, err)
		}
	}
	unpackTime := time.Since(unpackStart)
	parseStart := time.Now()
	clip, err := loadRecording(data)
	if err != nil {
		return recording{}, fmt.Errorf("recording %q: %w", location, err)
	}
	clip.id = fmt.Sprintf("%x", sha256.Sum256(data))
	log.Printf("recording loaded source=%s stored_bytes=%d json_bytes=%d read=%s unpack=%s parse_and_hash=%s",
		source, storedBytes, len(data), readTime, unpackTime, time.Since(parseStart))
	return clip, nil
}

func acceptsGzip(header http.Header) bool {
	for coding := range strings.SplitSeq(strings.Join(header.Values("Accept-Encoding"), ","), ",") {
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
		return true
	}
	return false
}

func replayRoutes(clip recording, clock *playbackClock) *http.ServeMux {
	mux := http.NewServeMux()
	metadata := clip.Session
	if clip.id != "" {
		metadata.RecordingVersion = "v1-" + clip.id
	}
	if len(metadata.Grid) > 0 && metadata.Timeline != nil {
		// Keep the lap and event selectors usable before the full timing feed arrives.
		reference := metadata.Drivers[0]
		metadata.Timeline = &raceTimeline{
			Events:     clip.Session.Timeline.Events,
			RaceStates: clip.Session.Timeline.RaceStates,
			RaceLaps:   clip.Session.Timeline.RaceLaps,
			TotalLaps:  clip.Session.Timeline.TotalLaps,
			Drivers: map[string]driverTiming{
				reference: {Laps: clip.Session.Timeline.Drivers[reference].Laps},
			},
		}
		mux.HandleFunc("GET /api/timing", raceTimingHandler(clip.Session.Timeline))
	}
	mux.HandleFunc("GET /api/session", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("Cache-Control", "no-cache")
		if err := json.NewEncoder(w).Encode(metadata); err != nil {
			log.Printf("send session metadata: %v", err)
		}
	})
	segmentHandler := func(w http.ResponseWriter, r *http.Request) {
		version := r.PathValue("version")
		if version != "" && version != metadata.RecordingVersion {
			w.Header().Set("Cache-Control", "no-store")
			http.NotFound(w, r)
			return
		}
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
		w.Header().Set("Vary", "Accept-Encoding")
		if version != "" {
			w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
		} else {
			w.Header().Set("Cache-Control", "no-cache")
		}
		var output io.Writer = w
		var compressor *gzip.Writer
		if acceptsGzip(r.Header) {
			// A fixed level with lower CPU cost is sufficient for repetitive position data.
			compressor, _ = gzip.NewWriterLevel(w, gzip.BestSpeed)
			output = compressor
			w.Header().Set("Content-Encoding", "gzip")
		}
		err = json.NewEncoder(output).Encode(struct {
			StartFrame int64        `json:"startFrame"`
			Frames     [][]position `json:"frames"`
		}{StartFrame: start, Frames: clip.Frames[start:min(start+size, count)]})
		if compressor != nil {
			closeErr := compressor.Close()
			if err == nil {
				err = closeErr
			}
		}
		if err != nil {
			log.Printf("send replay segment: %v", err)
		}
	}
	mux.HandleFunc("GET /api/segments/{index}", segmentHandler)
	mux.HandleFunc("GET /api/recordings/{version}/segments/{index}", segmentHandler)
	registry := prometheus.NewRegistry()
	connections := prometheus.NewGauge(prometheus.GaugeOpts{
		Name: "f1_replay_websocket_connections",
		Help: "Number of currently open replay WebSocket connections on this server.",
	})
	registry.MustRegister(connections)
	mux.Handle("GET /metrics", promhttp.HandlerFor(registry, promhttp.HandlerOpts{}))
	mux.HandleFunc("GET /readyz", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	})
	stream := frameStream{clip: clip, clock: clock, connections: connections}
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
		if acceptsGzip(r.Header) {
			w.Header().Set("Content-Encoding", "gzip")
			_, _ = w.Write(gzipped)
			return
		}
		_, _ = w.Write(payload)
	}
}

func main() {
	addr := flag.String("addr", "127.0.0.1:8080", "address to listen on")
	resetClock := flag.Bool("reset-shared-clock", false, "reset the Redis clock for this recording and exit")
	recordingLocation := flag.String("recording", "", "local path or s3://bucket/key for a prepared JSON recording")
	flag.Parse()

	startup := time.Now()
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
	clock := newPlaybackClock(time.Now())
	if redisURL := os.Getenv("REPLAY_REDIS_URL"); redisURL != "" {
		shared, err := newRedisReplayClock(redisURL, clip.id)
		if err != nil {
			log.Fatal(err)
		}
		operation := "read"
		if *resetClock {
			operation = "reset"
		}
		if err := shared.refresh(context.Background(), clock, operation); err != nil {
			log.Fatal(err)
		}
		if *resetClock {
			_ = shared.client.Close()
			log.Print("shared replay clock reset")
			return
		}
		go shared.watch(context.Background(), clock, clockRefreshInterval)
	} else if *resetClock {
		log.Fatal("-reset-shared-clock requires REPLAY_REDIS_URL")
	}
	mux := replayRoutes(clip, clock)
	mux.Handle("GET /", http.FileServerFS(viewer))
	server := http.Server{
		Addr:              *addr,
		Handler:           mux,
		ReadHeaderTimeout: 5 * time.Second,
	}
	log.Printf("F1 Race Replay at http://%s startup=%s", *addr, time.Since(startup))
	log.Fatal(server.ListenAndServe())
}
