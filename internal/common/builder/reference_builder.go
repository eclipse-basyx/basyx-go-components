/*******************************************************************************
 * Copyright (C) 2025 the Eclipse BaSyx Authors and Fraunhofer IESE
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

package builder

import (
	"database/sql"
	"slices"

	gen "github.com/eclipse-basyx/basyx-go-components/internal/common/model"
	_ "github.com/lib/pq" // PostgreSQL Treiber
)

type ReferenceBuilder struct {
	reference                 *gen.Reference
	keyIds                    []int64
	childKeyIds               []int64
	referredSemanticIdBuilder *ReferenceBuilder // Builder for nested referredSemanticId
}

func NewReferenceBuilder(referenceType string) (*gen.Reference, *ReferenceBuilder) {
	ref := &gen.Reference{
		Type: gen.ReferenceTypes(referenceType),
		Keys: []gen.Key{},
	}
	return ref, &ReferenceBuilder{keyIds: []int64{}, reference: ref, childKeyIds: []int64{}}
}

func NewMultiReferenceBuilder() ([]*gen.Reference, *MultiReferenceBuilder) {
	referenceMap := make(map[int64]*ReferenceBuilder)
	references := []*gen.Reference{}
	return references, &MultiReferenceBuilder{referenceBuilderMap: referenceMap, references: references}
}

func (rb *ReferenceBuilder) CreateKey(key_id int64, key_type string, key_value string) {
	skip := slices.Contains(rb.keyIds, key_id)
	if !skip {
		rb.keyIds = append(rb.keyIds, key_id)
		rb.reference.Keys = append(rb.reference.Keys, gen.Key{
			Type:  gen.KeyTypes(key_type),
			Value: key_value,
		})
	}
}

// CreateReferredSemanticId creates the referredSemanticId for this reference
func (rb *ReferenceBuilder) CreateReferredSemanticId(rootBuilder *ReferenceBuilder, referredSemanticIdDbId sql.NullInt64, referenceType string) *ReferenceBuilder {
	skip := slices.Contains(rootBuilder.childKeyIds, referredSemanticIdDbId.Int64)
	if !skip {
		if rb.referredSemanticIdBuilder == nil {
			referredSemanticId, builder := NewReferenceBuilder(referenceType)
			rb.reference.ReferredSemanticId = referredSemanticId
			rb.referredSemanticIdBuilder = builder
			rootBuilder.childKeyIds = append(rootBuilder.childKeyIds, referredSemanticIdDbId.Int64)
		}
		return rb.referredSemanticIdBuilder
	}
	return nil
}

// GetReferredSemanticIdBuilder returns the builder for the referredSemanticId (if it exists)
func (rb *ReferenceBuilder) GetReferredSemanticIdBuilder() *ReferenceBuilder {
	return rb.referredSemanticIdBuilder
}
