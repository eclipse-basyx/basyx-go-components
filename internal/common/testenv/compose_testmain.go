//nolint:all
package testenv

import (
	"context"
	"fmt"
	"net/http"
	"testing"
	"time"
)

type ComposeTestMainOptions struct {
	ComposeFile string

	UpArgs   []string
	DownArgs []string

	PreDownBeforeUp    bool
	SkipDownAfterTests bool

	FailIfComposeMissing bool

	HealthURL     string
	HealthTimeout time.Duration
	WaitForReady  func() error
}

func RunComposeTestMain(m *testing.M, options ComposeTestMainOptions) int {
	opts := normalizeComposeTestMainOptions(options)

	engine, baseArgs, err := FindCompose()
	if err != nil {
		fmt.Println("compose engine not found:", err)
		if opts.FailIfComposeMissing {
			return 1
		}
		return m.Run()
	}

	run := func(args ...string) error {
		cmdArgs := append([]string{}, baseArgs...)
		cmdArgs = append(cmdArgs, "-f", opts.ComposeFile)
		cmdArgs = append(cmdArgs, args...)
		return RunCompose(context.Background(), engine, cmdArgs...)
	}

	if opts.PreDownBeforeUp {
		_ = run(opts.DownArgs...)
	}

	fmt.Println("Starting Docker Compose...")
	if err := run(opts.UpArgs...); err != nil {
		fmt.Printf("Failed to start Docker Compose: %v\n", err)
		return 1
	}

	if opts.WaitForReady != nil {
		if err := opts.WaitForReady(); err != nil {
			fmt.Printf("Service readiness check failed: %v\n", err)
			if !opts.SkipDownAfterTests {
				_ = run(opts.DownArgs...)
			}
			return 1
		}
	}
	if opts.HealthURL != "" {
		if err := waitForHealthURL(opts.HealthURL, opts.HealthTimeout); err != nil {
			fmt.Printf("Health check failed: %v\n", err)
			if !opts.SkipDownAfterTests {
				_ = run(opts.DownArgs...)
			}
			return 1
		}
	}

	code := m.Run()

	if !opts.SkipDownAfterTests {
		fmt.Println("Stopping Docker Compose...")
		if err := run(opts.DownArgs...); err != nil {
			fmt.Printf("Failed to stop Docker Compose: %v\n", err)
		}
	}

	return code
}

func normalizeComposeTestMainOptions(options ComposeTestMainOptions) ComposeTestMainOptions {
	if options.ComposeFile == "" {
		options.ComposeFile = "docker_compose/docker_compose.yml"
	}
	if len(options.UpArgs) == 0 {
		options.UpArgs = []string{"up", "-d", "--build"}
	}
	if len(options.DownArgs) == 0 {
		options.DownArgs = []string{"down"}
	}
	if options.HealthURL != "" && options.HealthTimeout <= 0 {
		options.HealthTimeout = 2 * time.Minute
	}
	return options
}

func waitForHealthURL(url string, timeout time.Duration) error {
	if timeout <= 0 {
		timeout = 2 * time.Minute
	}
	deadline := time.Now().Add(timeout)
	backoff := time.Second
	client := HTTPClient()

	for {
		resp, err := client.Get(url)
		if err == nil {
			_ = resp.Body.Close()
			if resp.StatusCode == http.StatusOK {
				return nil
			}
		}
		if time.Now().After(deadline) {
			return fmt.Errorf("service not healthy at %s within %s", url, timeout)
		}
		time.Sleep(backoff)
		if backoff < 5*time.Second {
			backoff += 500 * time.Millisecond
		}
	}
}
