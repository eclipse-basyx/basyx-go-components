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

// Package submodelpath contains helpers for parsing and rebuilding submodel idShort paths.
package submodelpath

import (
	"errors"
	"strings"
)

var (
	// ErrEmptyPath indicates that the provided idShort path is empty.
	ErrEmptyPath = errors.New("empty idShort path")
	// ErrInvalidSyntax indicates invalid idShort path structure.
	ErrInvalidSyntax = errors.New("invalid idShort path syntax")
	// ErrEmptyListIndex indicates a list index segment without a value.
	ErrEmptyListIndex = errors.New("empty list index in idShort path")
)

// Segment represents one parsed idShort path segment.
type Segment struct {
	Value   string
	IsIndex bool
}

// ParseIDShortPathSegments parses an idShort path into segments.
func ParseIDShortPathSegments(idShortPath string) ([]Segment, error) {
	if idShortPath == "" {
		return nil, ErrEmptyPath
	}

	segments := make([]Segment, 0, 4)
	current := strings.Builder{}

	flushCurrent := func() {
		if current.Len() == 0 {
			return
		}
		segments = append(segments, Segment{Value: current.String()})
		current.Reset()
	}

	for i := 0; i < len(idShortPath); i++ {
		switch idShortPath[i] {
		case '.':
			flushCurrent()
		case '[':
			flushCurrent()
			endIndex := strings.IndexByte(idShortPath[i+1:], ']')
			if endIndex < 0 {
				return nil, ErrInvalidSyntax
			}

			start := i + 1
			end := start + endIndex
			indexValue := idShortPath[start:end]
			if indexValue == "" {
				return nil, ErrEmptyListIndex
			}

			segments = append(segments, Segment{Value: indexValue, IsIndex: true})
			i = end
		default:
			if err := current.WriteByte(idShortPath[i]); err != nil {
				return nil, ErrInvalidSyntax
			}
		}
	}

	flushCurrent()

	if len(segments) == 0 {
		return nil, ErrInvalidSyntax
	}

	return segments, nil
}

// BuildIDShortPathFromSegments rebuilds an idShort path from parsed segments.
func BuildIDShortPathFromSegments(segments []Segment) string {
	if len(segments) == 0 {
		return ""
	}

	var builder strings.Builder
	for i, segment := range segments {
		if segment.IsIndex {
			builder.WriteString("[")
			builder.WriteString(segment.Value)
			builder.WriteString("]")
			continue
		}

		if i > 0 {
			builder.WriteString(".")
		}
		builder.WriteString(segment.Value)
	}

	return builder.String()
}
