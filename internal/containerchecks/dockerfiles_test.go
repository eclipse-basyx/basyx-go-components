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

// Package containerchecks contains repository-level checks for container sources.
package containerchecks

import (
	"bufio"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

var dockerImageDigestPattern = regexp.MustCompile(`(?i)@sha256:[a-f0-9]{64}$`)

func TestDockerfilesUseDigestPinnedBaseImages(t *testing.T) {
	dockerfiles := findServiceDockerfiles(t)
	if len(dockerfiles) == 0 {
		t.Fatal("expected at least one service Dockerfile")
	}

	for _, dockerfilePath := range dockerfiles {
		serviceName := filepath.Base(filepath.Dir(dockerfilePath))
		t.Run(serviceName, func(t *testing.T) {
			assertDockerfileBaseImagesArePinned(t, dockerfilePath)
		})
	}
}

func findServiceDockerfiles(t *testing.T) []string {
	t.Helper()

	cmdPath := filepath.Join("..", "..", "cmd")
	entries, err := os.ReadDir(cmdPath)
	if err != nil {
		t.Fatalf("read cmd directory: %v", err)
	}

	var dockerfiles []string
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}

		dockerfilePath := filepath.Join(cmdPath, entry.Name(), "Dockerfile")
		if _, err := os.Stat(dockerfilePath); err == nil {
			dockerfiles = append(dockerfiles, dockerfilePath)
		} else if !os.IsNotExist(err) {
			t.Fatalf("stat %s: %v", dockerfilePath, err)
		}
	}

	return dockerfiles
}

func assertDockerfileBaseImagesArePinned(t *testing.T, dockerfilePath string) {
	t.Helper()

	content, err := os.ReadFile(filepath.Clean(dockerfilePath)) // #nosec G304 -- repository-local Dockerfile path discovered from cmd service directories.
	if err != nil {
		t.Fatalf("read %s: %v", dockerfilePath, err)
	}

	stageNames := make(map[string]struct{})
	scanner := bufio.NewScanner(strings.NewReader(string(content)))
	lineNumber := 0
	for scanner.Scan() {
		lineNumber++
		instruction, ok := parseFromInstruction(scanner.Text())
		if !ok {
			continue
		}

		if _, ok := stageNames[instruction.image]; !ok && !dockerImageDigestPattern.MatchString(instruction.image) {
			t.Errorf("%s:%d base image %q is not pinned by sha256 digest", dockerfilePath, lineNumber, instruction.image)
		}

		if instruction.stageName != "" {
			stageNames[instruction.stageName] = struct{}{}
		}
	}

	if err := scanner.Err(); err != nil {
		t.Fatalf("scan %s: %v", dockerfilePath, err)
	}
}

type fromInstruction struct {
	image     string
	stageName string
}

func parseFromInstruction(line string) (fromInstruction, bool) {
	fields := strings.Fields(line)
	if len(fields) == 0 || !strings.EqualFold(fields[0], "FROM") {
		return fromInstruction{}, false
	}

	imageIndex := firstImageFieldIndex(fields)
	if imageIndex >= len(fields) {
		return fromInstruction{}, false
	}

	return fromInstruction{
		image:     fields[imageIndex],
		stageName: stageNameFromFields(fields, imageIndex),
	}, true
}

func firstImageFieldIndex(fields []string) int {
	imageIndex := 1
	for imageIndex < len(fields) && strings.HasPrefix(fields[imageIndex], "--") {
		imageIndex++
	}

	return imageIndex
}

func stageNameFromFields(fields []string, imageIndex int) string {
	aliasIndex := imageIndex + 2
	if aliasIndex >= len(fields) || !strings.EqualFold(fields[imageIndex+1], "AS") {
		return ""
	}

	return fields[aliasIndex]
}
