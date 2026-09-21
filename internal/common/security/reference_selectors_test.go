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
// Author: Martin Stemmer ( Fraunhofer IESE )

package auth

import (
	"context"
	"strings"
	"testing"

	"github.com/doug-martin/goqu/v9"
	"github.com/eclipse-basyx/basyx-go-components/internal/common"
)

func TestReferenceSelectorSnapshotsInput(t *testing.T) {
	reference, err := common.DecodeAPIReference(common.EncodeString(`{"type":"ExternalReference","keys":[{"type":"GlobalReference","value":"urn:original"}]}`), 3072)
	if err != nil {
		t.Fatal(err)
	}
	ctx := common.ContextWithConfig(context.Background(), &common.Config{})
	ctx, err = WithAuthorizedReferenceSelectors(ctx, SemanticResourceSM, ReferenceSelector{Field: "$sm#semanticId", Reference: reference})
	if err != nil {
		t.Fatal(err)
	}
	reference.Keys()[0].SetValue("urn:mutated")
	query, err := AddReferenceSelectorQuery(ctx, goqu.Dialect("postgres").From("submodel").Select("id"), SemanticResourceSM)
	if err != nil {
		t.Fatal(err)
	}
	sql, args, err := query.Prepared(true).ToSQL()
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(sql, "submodel_supplemental_semantic_id_reference") {
		t.Fatal("missing supplemental predicate")
	}
	original := false
	for _, arg := range args {
		if arg == "urn:mutated" {
			t.Fatal("caller mutated the published predicate")
		}
		if arg == "urn:original" {
			original = true
		}
	}
	if !original {
		t.Fatal("lost reference key")
	}
}
