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
// Author: Aaron Zielstorff ( Fraunhofer IESE )

// Package model embeds the OpenFGA authorization model used by BaSyx ReBAC.
//
// model.fga is the reviewed source. model.json is its compiled form, which is
// written to OpenFGA. The content hash binds deployed models to this release.
package model

import (
	_ "embed"
	"encoding/json"
	"fmt"

	"github.com/eclipse-basyx/basyx-go-components/internal/common"
)

//go:embed model.fga
var dsl string

//go:embed model.json
var compiled []byte

// DSL returns the reviewed model source.
func DSL() string {
	return dsl
}

// JSON returns a copy of the compiled model.
func JSON() []byte {
	return append([]byte(nil), compiled...)
}

// Hash returns the canonical content hash of the embedded model.
func Hash() (string, error) {
	return CanonicalHash(compiled)
}

// CanonicalHash hashes the semantic content of an authorization model:
// schema version, type definitions and conditions. Store-assigned fields such
// as the model ID and representation defaults that OpenFGA adds when it
// returns a model (null metadata, empty strings, empty relation maps) are
// ignored, so a stored model hashes equal to the embedded one exactly when
// their content is identical.
func CanonicalHash(modelJSON []byte) (string, error) {
	var parsed map[string]any
	if err := json.Unmarshal(modelJSON, &parsed); err != nil {
		return "", fmt.Errorf("REBAC-MODELHASH-PARSE: %w", err)
	}
	content := map[string]any{
		"schema_version":   parsed["schema_version"],
		"type_definitions": parsed["type_definitions"],
		"conditions":       parsed["conditions"],
	}
	hash, err := common.CanonicalJSONHash(pruneDefaults(content))
	if err != nil {
		return "", fmt.Errorf("REBAC-MODELHASH-CANONICALIZE: %w", err)
	}
	return hash, nil
}

// pruneDefaults removes null values, empty strings, empty lists and empty
// objects. An empty "this" object is kept because it denotes direct
// assignability.
func pruneDefaults(value any) any {
	switch typed := value.(type) {
	case map[string]any:
		pruned := make(map[string]any, len(typed))
		for key, item := range typed {
			item = pruneDefaults(item)
			if isDefaultValue(item) && key != "this" {
				continue
			}
			pruned[key] = item
		}
		return pruned
	case []any:
		pruned := make([]any, 0, len(typed))
		for _, item := range typed {
			pruned = append(pruned, pruneDefaults(item))
		}
		return pruned
	default:
		return value
	}
}

func isDefaultValue(value any) bool {
	switch typed := value.(type) {
	case nil:
		return true
	case string:
		return typed == ""
	case map[string]any:
		return len(typed) == 0
	case []any:
		return len(typed) == 0
	default:
		return false
	}
}
