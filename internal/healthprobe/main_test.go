package main

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestParseOptionsHealthprobeDefaultsQuiet(t *testing.T) {
	options, err := parseOptions([]string{"healthprobe"})
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}

	if !options.quiet {
		t.Fatal("expected quiet default for healthprobe command")
	}

	if options.timeout != defaultTimeout {
		t.Fatalf("expected timeout %v, got %v", defaultTimeout, options.timeout)
	}
}

func TestParseOptionsWgetStyleArgs(t *testing.T) {
	options, err := parseOptions([]string{
		"wget",
		"--quiet",
		"--tries=1",
		"--output-document=-",
		"--timeout",
		"7",
		"http://localhost:8080/health",
	})
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}

	if !options.quiet {
		t.Fatal("expected quiet to be true")
	}
	if options.output != "-" {
		t.Fatalf("expected output '-', got %q", options.output)
	}
	if options.timeout != 7*time.Second {
		t.Fatalf("expected timeout 7s, got %v", options.timeout)
	}
	if options.url != "http://localhost:8080/health" {
		t.Fatalf("unexpected url %q", options.url)
	}
}

func TestParseOptionsInvalidTimeout(t *testing.T) {
	_, err := parseOptions([]string{"wget", "--timeout", "abc", "http://localhost:8080/health"})
	if err == nil {
		t.Fatal("expected error for invalid timeout")
	}
}

func TestBuildDefaultHealthURL(t *testing.T) {
	t.Setenv("SERVER_PORT", "8088")
	t.Setenv("SERVER_CONTEXTPATH", "")

	url := buildDefaultHealthURL()
	if url != "http://127.0.0.1:8088/health" {
		t.Fatalf("unexpected url %q", url)
	}
}

func TestBuildDefaultHealthURLWithContextPath(t *testing.T) {
	t.Setenv("SERVER_PORT", "8089")
	t.Setenv("SERVER_CONTEXTPATH", "/api")

	url := buildDefaultHealthURL()
	if url != "http://127.0.0.1:8089/api/health" {
		t.Fatalf("unexpected url %q", url)
	}
}

func TestRunProbeWritesStdout(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
		writer.WriteHeader(http.StatusOK)
		_, _ = writer.Write([]byte(`{"status":"UP"}`))
	}))
	defer server.Close()

	originalStdout := os.Stdout
	readPipe, writePipe, err := os.Pipe()
	if err != nil {
		t.Fatalf("failed to create pipe: %v", err)
	}
	os.Stdout = writePipe
	defer func() {
		os.Stdout = originalStdout
		_ = readPipe.Close()
		_ = writePipe.Close()
	}()

	err = runProbe(probeOptions{url: server.URL, output: "-", timeout: time.Second})
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}

	if closeErr := writePipe.Close(); closeErr != nil {
		t.Fatalf("failed to close write pipe: %v", closeErr)
	}

	buffer := make([]byte, 64)
	count, readErr := readPipe.Read(buffer)
	if readErr != nil {
		t.Fatalf("failed to read probe output: %v", readErr)
	}
	if count == 0 {
		t.Fatal("expected probe output on stdout")
	}
}

func TestRunProbeWritesFile(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
		writer.WriteHeader(http.StatusOK)
		_, _ = writer.Write([]byte(`{"status":"UP"}`))
	}))
	defer server.Close()

	outputPath := filepath.Join(t.TempDir(), "health.json")

	err := runProbe(probeOptions{url: server.URL, output: outputPath, timeout: time.Second})
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}

	content, readErr := os.ReadFile(outputPath) // #nosec G304 -- outputPath is created via t.TempDir()
	if readErr != nil {
		t.Fatalf("failed reading output file: %v", readErr)
	}
	if string(content) != `{"status":"UP"}` {
		t.Fatalf("unexpected file content %q", string(content))
	}
}

func TestRunProbeReturnsErrorOnUnhealthyStatus(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
		writer.WriteHeader(http.StatusInternalServerError)
	}))
	defer server.Close()

	err := runProbe(probeOptions{url: server.URL, output: "-", timeout: time.Second})
	if err == nil {
		t.Fatal("expected error for unhealthy status")
	}
}
