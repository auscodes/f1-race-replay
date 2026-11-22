package main

import (
	"bytes"
	"compress/gzip"
	"errors"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"github.com/aws/aws-sdk-go-v2/service/s3/types"
)

func TestReadRecordingFromS3(t *testing.T) {
	var compressed bytes.Buffer
	writer := gzip.NewWriter(&compressed)
	if _, err := writer.Write(demo); err != nil {
		t.Fatal(err)
	}
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || r.Header.Get("Authorization") == "" {
			t.Errorf("expected a signed GetObject request, got %s", r.Method)
		}
		switch r.URL.Path {
		case "/replay-bucket/clips/club circuit.json":
			w.Write(demo)
		case "/replay-bucket/clips/club circuit.json.gz":
			w.Write(compressed.Bytes())
		case "/replay-bucket/clips/missing.json":
			w.Header().Set("Content-Type", "application/xml")
			w.WriteHeader(http.StatusNotFound)
			w.Write([]byte("<Error><Code>NoSuchKey</Code><Message>Missing recording</Message></Error>"))
		default:
			t.Errorf("unexpected object path %q", r.URL.Path)
			w.WriteHeader(http.StatusBadRequest)
		}
	}))
	defer server.Close()

	// Keep the SDK on the local test server with disposable credentials.
	t.Setenv("AWS_ACCESS_KEY_ID", "test-key")
	t.Setenv("AWS_SECRET_ACCESS_KEY", "test-secret")
	t.Setenv("AWS_SESSION_TOKEN", "")
	t.Setenv("AWS_REGION", "us-east-1")
	t.Setenv("AWS_PROFILE", "")
	t.Setenv("AWS_CONFIG_FILE", filepath.Join(t.TempDir(), "config"))
	t.Setenv("AWS_SHARED_CREDENTIALS_FILE", filepath.Join(t.TempDir(), "credentials"))
	t.Setenv("AWS_ENDPOINT_URL_S3", server.URL)
	t.Setenv("AWS_IGNORE_CONFIGURED_ENDPOINT_URLS", "false")

	clip, err := readRecording(t.Context(), "s3://replay-bucket/clips/club%20circuit.json")
	if err != nil {
		t.Fatal(err)
	}
	if clip.Session.Name != "Club Circuit" || clip.Session.DurationMS != 12000 || len(clip.Frames) != 60 {
		t.Fatalf("unexpected recording from S3: %q, %d frames", clip.Session.Name, len(clip.Frames))
	}
	zipped, err := readRecording(t.Context(), "s3://replay-bucket/clips/club%20circuit.json.gz")
	if err != nil || zipped.id != clip.id || len(zipped.Frames) != len(clip.Frames) {
		t.Fatalf("compressed S3 object changed recording identity or frame count: %v", err)
	}
	var noSuchKey *types.NoSuchKey
	if _, err := readRecording(t.Context(), "s3://replay-bucket/clips/missing.json"); !errors.As(err, &noSuchKey) {
		t.Fatalf("missing S3 recording error = %v, want NoSuchKey", err)
	}
}

func TestInvalidS3RecordingLocation(t *testing.T) {
	for _, location := range []string{"s3:///clip.json", "s3://bucket/", "s3://bucket/clip.json?key=value"} {
		t.Run(location, func(t *testing.T) {
			if _, err := readRecording(t.Context(), location); err == nil || !strings.Contains(err.Error(), "s3://bucket/key") {
				t.Fatalf("invalid S3 recording location error = %v", err)
			}
		})
	}
}
