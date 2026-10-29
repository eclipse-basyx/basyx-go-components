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

package aol

import (
	"io/fs"
	"sort"
	"strings"
	"unicode"
)

// Document is a version-pinned, source-labelled documentation resource.
type Document struct {
	URI  string `json:"uri"`
	Text string `json:"text"`
}

// Documents loads only Markdown and explicitly supplied OpenAPI documents.
func Documents(files fs.FS) ([]Document, error) {
	docs := []Document{}
	err := fs.WalkDir(files, ".", func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() || !strings.HasSuffix(path, ".md") {
			return nil
		}
		raw, err := fs.ReadFile(files, path)
		if err != nil {
			return err
		}
		docs = append(docs, Document{URI: "basyx-docs://" + path, Text: string(raw)})
		return nil
	})
	return docs, err
}

// SearchDocs performs deterministic local term search with bounded excerpts.
func SearchDocs(docs []Document, query string) []Document {
	type match struct {
		doc   Document
		score int
	}
	matches := []match{}
	terms := strings.Fields(strings.ToLower(query))
	if len(terms) == 0 {
		return []Document{}
	}
	for _, doc := range docs {
		score, position := scoreDocument(doc.Text, terms)
		if score == 0 {
			continue
		}
		start := max(0, position-100)
		end := min(len(doc.Text), start+1800)
		matches = append(matches, match{Document{URI: doc.URI, Text: strings.ToValidUTF8(doc.Text[start:end], "")}, score})
	}
	sort.Slice(matches, func(i, j int) bool {
		if matches[i].score == matches[j].score {
			return matches[i].doc.URI < matches[j].doc.URI
		}
		return matches[i].score > matches[j].score
	})
	result := []Document{}
	for _, item := range matches[:min(len(matches), 5)] {
		result = append(result, item.doc)
	}
	return result
}

// CleanText removes unsafe control characters only from presentation text.
func CleanText(s string) string {
	return strings.Map(func(r rune) rune {
		if unicode.IsControl(r) && r != '\n' && r != '\t' {
			return -1
		}
		return r
	}, s)
}

func scoreDocument(text string, terms []string) (int, int) {
	lower := strings.ToLower(text)
	score, position := 0, -1
	for _, term := range terms {
		index := strings.Index(lower, term)
		if index < 0 {
			continue
		}
		score++
		if position < 0 {
			position = index
		}
	}
	return score, position
}
