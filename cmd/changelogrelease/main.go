/*******************************************************************************
* Copyright (C) 2026 the Eclipse BaSyx Authors and Fraunhofer IESE
*
* Permission is hereby granted, free of charge, to any person obtaining
* a copy of this software and associated documentation files (the
* "Software"), to deal in the Software without restriction, including
* without limitation the rights to use, copy, modify, merge, publish,
* distribute, sublicense, and/or sell copies of the Software, and to
* permit persons to whom the Software is furnished to do so, subject to
* the following conditions:
*
* The above copyright notice and this permission notice shall be
* included in all copies or substantial portions of the Software.
*
* THE SOFTWARE IS PROVIDED "AS IS", WITHOUT WARRANTY OF ANY KIND,
* EXPRESS OR IMPLIED, INCLUDING BUT NOT LIMITED TO THE WARRANTIES OF
* MERCHANTABILITY, FITNESS FOR A PARTICULAR PURPOSE AND
* NONINFRINGEMENT. IN NO EVENT SHALL THE AUTHORS OR COPYRIGHT HOLDERS BE
* LIABLE FOR ANY CLAIM, DAMAGES OR OTHER LIABILITY, WHETHER IN AN ACTION
* OF CONTRACT, TORT OR OTHERWISE, ARISING FROM, OUT OF OR IN CONNECTION
* WITH THE SOFTWARE OR THE USE OR OTHER DEALINGS IN THE SOFTWARE.
*
* SPDX-License-Identifier: MIT
******************************************************************************/

// Package main provides the release changelog maintenance command.
package main

import (
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/eclipse-basyx/basyx-go-components/internal/changelogrelease"
)

type commandResult struct {
	content    []byte
	outputPath string
}

func main() {
	os.Exit(run(os.Args[1:], os.Stdout, os.Stderr))
}

func run(args []string, stdout io.Writer, stderr io.Writer) int {
	result, err := execute(args)
	if err == nil {
		err = writeResult(result, stdout)
	}
	if err != nil {
		_, _ = fmt.Fprintln(stderr, err)
		return 1
	}
	return 0
}

func execute(args []string) (commandResult, error) {
	if len(args) == 0 {
		return commandResult{}, fmt.Errorf("CHLOG-MAIN-PARSECOMMAND: expected validate, prepare, extract, or compose")
	}
	switch args[0] {
	case "validate":
		return executeValidate(args[1:])
	case "prepare":
		return executePrepare(args[1:])
	case "extract":
		return executeExtract(args[1:])
	case "compose":
		return executeCompose(args[1:])
	default:
		return commandResult{}, fmt.Errorf("CHLOG-MAIN-PARSECOMMAND: unsupported command %q", args[0])
	}
}

func executeValidate(args []string) (commandResult, error) {
	flags := newFlagSet("validate")
	changelogPath := flags.String("changelog", "CHANGELOG.md", "source changelog path")
	if err := flags.Parse(args); err != nil {
		return commandResult{}, fmt.Errorf("CHLOG-MAIN-PARSEFLAGS: validate arguments are invalid: %w", err)
	}
	content, err := readFile(*changelogPath)
	if err != nil {
		return commandResult{}, err
	}
	if err = changelogrelease.Validate(content); err != nil {
		return commandResult{}, err
	}
	return commandResult{content: []byte("Changelog is valid.\n"), outputPath: "-"}, nil
}

func executePrepare(args []string) (commandResult, error) {
	flags := newFlagSet("prepare")
	version := flags.String("version", "", "release version in vX.Y.Z format")
	changelogPath := flags.String("changelog", "CHANGELOG.md", "source changelog path")
	outputPath := flags.String("output", "-", "output path or - for standard output")
	if err := flags.Parse(args); err != nil {
		return commandResult{}, fmt.Errorf("CHLOG-MAIN-PARSEFLAGS: prepare arguments are invalid: %w", err)
	}
	if *version == "" {
		return commandResult{}, fmt.Errorf("CHLOG-MAIN-VALIDATEARGS: prepare requires --version")
	}
	content, err := readFile(*changelogPath)
	if err != nil {
		return commandResult{}, err
	}
	prepared, err := changelogrelease.Prepare(content, *version)
	if err != nil {
		return commandResult{}, err
	}
	return commandResult{content: prepared, outputPath: *outputPath}, nil
}

func executeExtract(args []string) (commandResult, error) {
	flags := newFlagSet("extract")
	version := flags.String("version", "", "release version in vX.Y.Z format")
	changelogPath := flags.String("changelog", "CHANGELOG.md", "source changelog path")
	outputPath := flags.String("output", "-", "output path or - for standard output")
	if err := flags.Parse(args); err != nil {
		return commandResult{}, fmt.Errorf("CHLOG-MAIN-PARSEFLAGS: extract arguments are invalid: %w", err)
	}
	if *version == "" {
		return commandResult{}, fmt.Errorf("CHLOG-MAIN-VALIDATEARGS: extract requires --version")
	}
	content, err := readFile(*changelogPath)
	if err != nil {
		return commandResult{}, err
	}
	notes, err := changelogrelease.Extract(content, *version)
	if err != nil {
		return commandResult{}, err
	}
	return commandResult{content: notes, outputPath: *outputPath}, nil
}

func executeCompose(args []string) (commandResult, error) {
	flags := newFlagSet("compose")
	bodyPath := flags.String("body", "", "existing release body path")
	changelogPath := flags.String("changelog", "", "rendered changelog path")
	outputPath := flags.String("output", "-", "output path or - for standard output")
	if err := flags.Parse(args); err != nil {
		return commandResult{}, fmt.Errorf("CHLOG-MAIN-PARSEFLAGS: compose arguments are invalid: %w", err)
	}
	if *bodyPath == "" || *changelogPath == "" {
		return commandResult{}, fmt.Errorf("CHLOG-MAIN-VALIDATEARGS: compose requires --body and --changelog")
	}
	body, err := readFile(*bodyPath)
	if err != nil {
		return commandResult{}, err
	}
	changelog, err := readFile(*changelogPath)
	if err != nil {
		return commandResult{}, err
	}
	composed, err := changelogrelease.Compose(body, changelog)
	if err != nil {
		return commandResult{}, err
	}
	return commandResult{content: composed, outputPath: *outputPath}, nil
}

func newFlagSet(name string) *flag.FlagSet {
	flags := flag.NewFlagSet(name, flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	return flags
}

func readFile(path string) ([]byte, error) {
	cleanPath := filepath.Clean(path)
	content, err := os.ReadFile(cleanPath) // #nosec G304 -- release operators explicitly provide trusted workspace paths.
	if err != nil {
		return nil, fmt.Errorf("CHLOG-MAIN-READFILE: failed to read %s: %w", cleanPath, err)
	}
	return content, nil
}

func writeResult(result commandResult, stdout io.Writer) error {
	if result.outputPath == "-" {
		if _, err := stdout.Write(result.content); err != nil {
			return fmt.Errorf("CHLOG-MAIN-WRITESTDOUT: failed to write command output: %w", err)
		}
		return nil
	}
	cleanPath := filepath.Clean(result.outputPath)
	if err := os.MkdirAll(filepath.Dir(cleanPath), 0o750); err != nil {
		return fmt.Errorf("CHLOG-MAIN-CREATEDIR: failed to create output directory for %s: %w", cleanPath, err)
	}
	if err := os.WriteFile(cleanPath, result.content, 0o644); err != nil {
		return fmt.Errorf("CHLOG-MAIN-WRITEFILE: failed to write %s: %w", cleanPath, err)
	}
	return nil
}
