// Package main provides a tiny static health probe used in distroless container images.
package main

import (
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

const (
	defaultPort    = "5000"
	defaultTimeout = 5 * time.Second
)

type probeOptions struct {
	url     string
	quiet   bool
	spider  bool
	output  string
	debug   bool
	timeout time.Duration
}

func main() {
	options, err := parseOptions(os.Args)
	if err != nil {
		_, _ = fmt.Fprintln(os.Stderr, err.Error())
		os.Exit(1)
	}

	if options.url == "" {
		options.url = buildDefaultHealthURL()
	}

	if options.debug {
		_, _ = fmt.Fprintf(os.Stderr, "healthprobe url=%s timeout=%s\n", options.url, options.timeout)
	}

	if err := runProbe(options); err != nil {
		if !options.quiet {
			_, _ = fmt.Fprintln(os.Stderr, err.Error())
		}
		os.Exit(1)
	}
}

func parseOptions(args []string) (probeOptions, error) {
	options := probeOptions{
		output:  "-",
		timeout: defaultTimeout,
	}

	commandName := filepath.Base(args[0])
	if commandName == "healthprobe" {
		options.quiet = true
	}

	remainingArgs := args[1:]
	for len(remainingArgs) > 0 {
		argument := remainingArgs[0]
		remainingArgs = remainingArgs[1:]

		switch {
		case argument == "--quiet" || argument == "-q":
			options.quiet = true
		case argument == "--spider":
			options.spider = true
		case argument == "--debug":
			options.debug = true
		case argument == "--tries":
			if len(remainingArgs) == 0 {
				return options, errors.New("HEALTHPROBE-PARSE-MISSINGTRIES")
			}
			remainingArgs = remainingArgs[1:]
		case strings.HasPrefix(argument, "--tries="):
			continue
		case argument == "--output-document" || argument == "-O":
			if len(remainingArgs) == 0 {
				return options, errors.New("HEALTHPROBE-PARSE-MISSINGOUTPUT")
			}
			options.output = remainingArgs[0]
			remainingArgs = remainingArgs[1:]
		case strings.HasPrefix(argument, "--output-document="):
			options.output = strings.TrimPrefix(argument, "--output-document=")
		case argument == "--timeout":
			if len(remainingArgs) == 0 {
				return options, errors.New("HEALTHPROBE-PARSE-MISSINGTIMEOUT")
			}
			timeoutSeconds, err := strconv.Atoi(remainingArgs[0])
			if err != nil || timeoutSeconds <= 0 {
				return options, errors.New("HEALTHPROBE-PARSE-INVALIDTIMEOUT")
			}
			options.timeout = time.Duration(timeoutSeconds) * time.Second
			remainingArgs = remainingArgs[1:]
		case strings.HasPrefix(argument, "-"):
			continue
		default:
			options.url = argument
		}
	}

	if !options.spider && options.output == "" {
		options.output = "-"
	}

	return options, nil
}

func buildDefaultHealthURL() string {
	port := os.Getenv("SERVER_PORT")
	if port == "" {
		port = defaultPort
	}

	contextPath := os.Getenv("SERVER_CONTEXTPATH")
	if contextPath == "" {
		return fmt.Sprintf("http://127.0.0.1:%s/health", port)
	}

	return fmt.Sprintf("http://127.0.0.1:%s%s/health", port, contextPath)
}

func runProbe(options probeOptions) error {
	client := &http.Client{Timeout: options.timeout}

	response, err := client.Get(options.url)
	if err != nil {
		return fmt.Errorf("HEALTHPROBE-RUN-REQUESTFAILED: %w", err)
	}
	defer func() {
		_ = response.Body.Close()
	}()

	if response.StatusCode >= http.StatusBadRequest {
		return fmt.Errorf("HEALTHPROBE-RUN-UNHEALTHYSTATUS: %d", response.StatusCode)
	}

	if options.spider {
		_, _ = io.Copy(io.Discard, response.Body)
		return nil
	}

	if options.output == "-" {
		_, err = io.Copy(os.Stdout, response.Body)
		if err != nil {
			return fmt.Errorf("HEALTHPROBE-RUN-WRITESTDOUTFAILED: %w", err)
		}
		return nil
	}

	file, err := os.Create(options.output)
	if err != nil {
		return fmt.Errorf("HEALTHPROBE-RUN-CREATEOUTPUTFAILED: %w", err)
	}
	defer func() {
		_ = file.Close()
	}()

	_, err = io.Copy(file, response.Body)
	if err != nil {
		return fmt.Errorf("HEALTHPROBE-RUN-WRITEOUTPUTFAILED: %w", err)
	}

	return nil
}
