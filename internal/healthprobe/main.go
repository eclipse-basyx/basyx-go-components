// Package main provides a tiny static health probe used in distroless container images.
package main

import (
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
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
		_, _ = fmt.Fprintln(os.Stderr, "HEALTHPROBE-MAIN-PARSEFAILED")
		os.Exit(1)
	}

	if options.url == "" {
		options.url = buildDefaultHealthURL()
	}

	if options.debug {
		_, _ = fmt.Fprintln(os.Stderr, "HEALTHPROBE-MAIN-DEBUGENABLED")
	}

	if err := runProbe(options); err != nil {
		if !options.quiet {
			_, _ = fmt.Fprintln(os.Stderr, "HEALTHPROBE-MAIN-PROBEFAILED")
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

	probeURL, err := parseAndValidateProbeURL(options.url)
	if err != nil {
		return err
	}

	// #nosec G704 -- URL is constrained to localhost/loopback via parseAndValidateProbeURL
	response, err := client.Get(probeURL.String())
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

	outputPath, err := sanitizeOutputPath(options.output)
	if err != nil {
		return err
	}

	// #nosec G304 G703 -- output path is sanitized in sanitizeOutputPath to block traversal and absolute paths
	file, err := os.OpenFile(outputPath, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0o600)
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

func parseAndValidateProbeURL(rawURL string) (*url.URL, error) {
	parsedURL, err := url.Parse(rawURL)
	if err != nil {
		return nil, errors.New("HEALTHPROBE-PARSE-INVALIDURL")
	}

	if parsedURL.Scheme != "http" && parsedURL.Scheme != "https" {
		return nil, errors.New("HEALTHPROBE-PARSE-INVALIDSCHEME")
	}

	host := parsedURL.Hostname()
	if host == "localhost" {
		return parsedURL, nil
	}

	ip := net.ParseIP(host)
	if ip == nil || !ip.IsLoopback() {
		return nil, errors.New("HEALTHPROBE-PARSE-NONLOCALHOST")
	}

	return parsedURL, nil
}

func sanitizeOutputPath(path string) (string, error) {
	cleanPath := filepath.Clean(path)
	if cleanPath == "" || cleanPath == "." {
		return "", errors.New("HEALTHPROBE-PARSE-INVALIDOUTPUTPATH")
	}

	if filepath.IsAbs(cleanPath) {
		return "", errors.New("HEALTHPROBE-PARSE-ABSOLUTEOUTPUTPATH")
	}

	if cleanPath == ".." || strings.HasPrefix(cleanPath, "../") {
		return "", errors.New("HEALTHPROBE-PARSE-TRAVERSALOUTPUTPATH")
	}

	return cleanPath, nil
}
