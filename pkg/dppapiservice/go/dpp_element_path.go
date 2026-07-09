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
// Author: Jannik Fried ( Fraunhofer IESE ), Aaron Zielstorff ( Fraunhofer IESE )

package dppapi

import (
	"fmt"
	"strconv"
	"strings"
	"unicode/utf8"
)

type dppElementPath struct {
	sectionName string
	idShortPath string
}

type dppJSONPathSegment struct {
	name    string
	index   int
	isIndex bool
}

func parseDPPJSONElementPath(elementIDPath string) (dppElementPath, error) {
	segments, err := parseDPPJSONPathSegments(elementIDPath)
	if err != nil {
		return dppElementPath{}, err
	}
	if len(segments) < 2 || segments[0].isIndex {
		return dppElementPath{}, invalidDPPElementIDPathError()
	}
	idShortPath, err := dppIDShortPathFromJSONPathSegments(segments[1:])
	if err != nil {
		return dppElementPath{}, err
	}
	return dppElementPath{sectionName: segments[0].name, idShortPath: idShortPath}, nil
}

func parseDPPJSONPathSegments(value string) ([]dppJSONPathSegment, error) {
	if value == "" || value[0] != '$' {
		return nil, invalidDPPElementIDPathError()
	}
	segments := make([]dppJSONPathSegment, 0)
	for index := 1; index < len(value); {
		if value[index] != '[' {
			return nil, invalidDPPElementIDPathError()
		}
		segment, next, err := parseDPPJSONPathBracketSegment(value, index+1)
		if err != nil {
			return nil, err
		}
		segments = append(segments, segment)
		index = next
	}
	return segments, nil
}

func parseDPPJSONPathBracketSegment(value string, index int) (dppJSONPathSegment, int, error) {
	if index >= len(value) || value[index] == '"' {
		return dppJSONPathSegment{}, 0, invalidDPPElementIDPathError()
	}
	if value[index] == '\'' {
		return parseDPPJSONPathQuotedSegment(value, index)
	}
	return parseDPPJSONPathIndexSegment(value, index)
}

func parseDPPJSONPathQuotedSegment(value string, index int) (dppJSONPathSegment, int, error) {
	quote := value[index]
	index++
	name := make([]rune, 0)
	for index < len(value) {
		if value[index] == '\\' {
			escaped, next, err := parseDPPJSONPathEscape(value, index)
			if err != nil {
				return dppJSONPathSegment{}, 0, err
			}
			name = append(name, escaped)
			index = next
			continue
		}
		if value[index] == quote {
			if index+1 >= len(value) || value[index+1] != ']' {
				return dppJSONPathSegment{}, 0, invalidDPPElementIDPathError()
			}
			if len(name) == 0 {
				return dppJSONPathSegment{}, 0, invalidDPPElementIDPathError()
			}
			return dppJSONPathSegment{name: string(name)}, index + 2, nil
		}
		r, width := utf8.DecodeRuneInString(value[index:])
		if r == utf8.RuneError && width == 1 || r < 0x20 {
			return dppJSONPathSegment{}, 0, invalidDPPElementIDPathError()
		}
		name = append(name, r)
		index += width
	}
	return dppJSONPathSegment{}, 0, invalidDPPElementIDPathError()
}

func parseDPPJSONPathEscape(value string, index int) (rune, int, error) {
	if index+1 >= len(value) {
		return 0, 0, invalidDPPElementIDPathError()
	}
	switch value[index+1] {
	case 'b':
		return '\b', index + 2, nil
	case 'f':
		return '\f', index + 2, nil
	case 'n':
		return '\n', index + 2, nil
	case 'r':
		return '\r', index + 2, nil
	case 't':
		return '\t', index + 2, nil
	case '\'', '\\':
		return rune(value[index+1]), index + 2, nil
	case 'u':
		return parseDPPJSONPathUnicodeEscape(value, index)
	default:
		return 0, 0, invalidDPPElementIDPathError()
	}
}

func parseDPPJSONPathUnicodeEscape(value string, index int) (rune, int, error) {
	if index+6 > len(value) {
		return 0, 0, invalidDPPElementIDPathError()
	}
	hex := value[index+2 : index+6]
	if !isDPPJSONPathNormalHex(hex) {
		return 0, 0, invalidDPPElementIDPathError()
	}
	parsed, err := strconv.ParseInt(hex, 16, 32)
	if err != nil {
		return 0, 0, invalidDPPElementIDPathError()
	}
	return rune(parsed), index + 6, nil
}

func isDPPJSONPathNormalHex(value string) bool {
	if len(value) != 4 || value[0] != '0' || value[1] != '0' {
		return false
	}
	if value[2] == '0' {
		return value[3] >= '0' && value[3] <= '7' || value[3] == 'b' || value[3] == 'e' || value[3] == 'f'
	}
	if value[2] != '1' {
		return false
	}
	return value[3] >= '0' && value[3] <= '9' || value[3] >= 'a' && value[3] <= 'f'
}

func parseDPPJSONPathIndexSegment(value string, index int) (dppJSONPathSegment, int, error) {
	start := index
	if value[index] == '0' {
		index++
	} else {
		if value[index] < '1' || value[index] > '9' {
			return dppJSONPathSegment{}, 0, invalidDPPElementIDPathError()
		}
		for index < len(value) && value[index] >= '0' && value[index] <= '9' {
			index++
		}
	}
	if start == index || index >= len(value) || value[index] != ']' {
		return dppJSONPathSegment{}, 0, invalidDPPElementIDPathError()
	}
	parsed, err := strconv.Atoi(value[start:index])
	if err != nil {
		return dppJSONPathSegment{}, 0, invalidDPPElementIDPathError()
	}
	return dppJSONPathSegment{index: parsed, isIndex: true}, index + 1, nil
}

func dppIDShortPathFromJSONPathSegments(segments []dppJSONPathSegment) (string, error) {
	parts := make([]string, 0, len(segments))
	for _, segment := range segments {
		if segment.isIndex {
			if len(parts) == 0 {
				return "", invalidDPPElementIDPathError()
			}
			last := len(parts) - 1
			parts[last] += "[" + strconv.Itoa(segment.index) + "]"
			continue
		}
		if segment.name == "" {
			return "", invalidDPPElementIDPathError()
		}
		parts = append(parts, segment.name)
	}
	if len(parts) == 0 {
		return "", invalidDPPElementIDPathError()
	}
	return strings.Join(parts, "."), nil
}

func invalidDPPElementIDPathError() error {
	return fmt.Errorf("DPP-ELEMPATH-INVALID elementIdPath must be an RFC 9535 Normalized Path selecting a DPP data element")
}
