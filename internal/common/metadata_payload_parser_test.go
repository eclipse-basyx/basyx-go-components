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

package common

import "testing"

func TestParseSubmodelMetadataPayload_AllowsNullOptionalFields(t *testing.T) {
	_, err := ParseSubmodelMetadataPayload(map[string]any{
		"id":          "urn:test:submodel",
		"modelType":   "Submodel",
		"description": nil,
		"displayName": nil,
		"semanticId":  nil,
	})
	if err != nil {
		t.Fatalf("expected null optional fields to be accepted, got error: %v", err)
	}
}

func TestParseSubmodelMetadataPayload_RejectsNullRequiredField(t *testing.T) {
	_, err := ParseSubmodelMetadataPayload(map[string]any{
		"id":        "urn:test:submodel",
		"modelType": nil,
	})
	if err == nil {
		t.Fatal("expected error for null modelType, got nil")
	}
}

func TestParseSubmodelElementMetadataPayload_AllowsNullOptionalFields(t *testing.T) {
	_, err := ParseSubmodelElementMetadataPayload(map[string]any{
		"modelType":   "Property",
		"description": nil,
		"semanticId":  nil,
	})
	if err != nil {
		t.Fatalf("expected null optional fields to be accepted, got error: %v", err)
	}
}

func TestParseSubmodelElementMetadataPayload_RejectsNullModelType(t *testing.T) {
	_, err := ParseSubmodelElementMetadataPayload(map[string]any{
		"modelType": nil,
	})
	if err == nil {
		t.Fatal("expected error for null modelType, got nil")
	}
}
