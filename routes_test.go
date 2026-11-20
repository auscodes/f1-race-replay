package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"reflect"
	"testing"
	"time"
)

func TestSessionRouteReturnsOnlySessionMetadata(t *testing.T) {
	clip, err := loadRecording(demo)
	if err != nil {
		t.Fatal(err)
	}
	response := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/session", nil)
	replayRoutes(clip, time.Now()).ServeHTTP(response, req)

	if response.Code != http.StatusOK {
		t.Fatalf("session status = %d, want 200", response.Code)
	}
	if got := response.Header().Get("Content-Type"); got != "application/json" {
		t.Fatalf("session content type = %q, want application/json", got)
	}
	var got session
	decoder := json.NewDecoder(response.Body)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&got); err != nil {
		t.Fatalf("decode session metadata: %v", err)
	}
	if !reflect.DeepEqual(got, clip.Session) {
		t.Errorf("response metadata differs from the recording's session metadata")
	}
}

func TestWebSocketRouteRequiresUpgrade(t *testing.T) {
	response := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/ws", nil)
	replayRoutes(recording{}, time.Now()).ServeHTTP(response, req)
	if response.Code != http.StatusUpgradeRequired {
		t.Fatalf("plain GET /ws status = %d, want 426", response.Code)
	}
}
