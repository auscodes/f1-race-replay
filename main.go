package main

import (
	"embed"
	"encoding/json"
	"flag"
	"io/fs"
	"log"
	"net/http"
	"time"
)

//go:embed testdata/demo.json
var demo []byte

//go:embed web
var web embed.FS

func replayRoutes(clip recording, startedAt time.Time) *http.ServeMux {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/session", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if err := json.NewEncoder(w).Encode(clip.Session); err != nil {
			log.Printf("send session metadata: %v", err)
		}
	})
	stream := frameStream{clip: clip, startedAt: startedAt}
	mux.HandleFunc("GET /ws", stream.serve)
	return mux
}

func main() {
	addr := flag.String("addr", "127.0.0.1:8080", "address to listen on")
	flag.Parse()

	clip, err := loadRecording(demo)
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
