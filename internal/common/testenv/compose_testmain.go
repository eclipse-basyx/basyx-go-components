//nolint:all
package testenv

import (
	"context"
	"fmt"
	"testing"
	"time"
)

type ComposeTestMainOptions struct {
	ComposeFile string

	UpArgs   []string
	DownArgs []string

	UpTimeout   time.Duration
	DownTimeout time.Duration

	PreDownBeforeUp    bool
	SkipDownAfterTests bool

	HealthURL     string
	HealthTimeout time.Duration
	WaitForReady  func() error
}

func RunComposeTestMain(m *testing.M, options ComposeTestMainOptions) int {
	opts := normalizeComposeTestMainOptions(options)

	engine, baseArgs, err := FindCompose()
	if err != nil {
		fmt.Println("compose engine not found:", err)
		return 1
	}

	runWithLimit := func(timeout time.Duration, args ...string) error {
		return runWithTimeout(engine, baseArgs, opts.ComposeFile, timeout, args...)
	}

	if opts.PreDownBeforeUp {
		_ = runWithLimit(opts.DownTimeout, opts.DownArgs...)
	}

	fmt.Println("Starting Docker Compose...")
	if err := runWithLimit(opts.UpTimeout, opts.UpArgs...); err != nil {
		fmt.Printf("Failed to start Docker Compose: %v\n", err)
		return 1
	}

	if opts.WaitForReady != nil {
		if err := opts.WaitForReady(); err != nil {
			fmt.Printf("Service readiness check failed: %v\n", err)
			if !opts.SkipDownAfterTests {
				_ = runWithLimit(opts.DownTimeout, opts.DownArgs...)
			}
			return 1
		}
	}
	if opts.HealthURL != "" {
		if err := WaitHealthyURL(opts.HealthURL, opts.HealthTimeout); err != nil {
			fmt.Printf("Health check failed: %v\n", err)
			if !opts.SkipDownAfterTests {
				_ = runWithLimit(opts.DownTimeout, opts.DownArgs...)
			}
			return 1
		}
	}

	code := m.Run()

	if !opts.SkipDownAfterTests {
		fmt.Println("Stopping Docker Compose...")
		if err := runWithLimit(opts.DownTimeout, opts.DownArgs...); err != nil {
			fmt.Printf("Failed to stop Docker Compose: %v\n", err)
		}
	}

	return code
}

func runWithTimeout(engine string, baseArgs []string, composeFile string, timeout time.Duration, args ...string) error {
	ctx := context.Background()
	cancel := func() {}
	if timeout > 0 {
		ctxWithTimeout, cancelFn := context.WithTimeout(context.Background(), timeout)
		ctx = ctxWithTimeout
		cancel = cancelFn
	}
	defer cancel()

	cmdArgs := append([]string{}, baseArgs...)
	cmdArgs = append(cmdArgs, "-f", composeFile)
	cmdArgs = append(cmdArgs, args...)
	return RunCompose(ctx, engine, cmdArgs...)
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
	if options.UpTimeout <= 0 {
		options.UpTimeout = 10 * time.Minute
	}
	if options.DownTimeout <= 0 {
		options.DownTimeout = 10 * time.Minute
	}
	if options.HealthURL != "" && options.HealthTimeout <= 0 {
		options.HealthTimeout = 2 * time.Minute
	}
	return options
}
