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

import gen "github.com/eclipse-basyx/basyx-go-components/internal/common/model"

type MultiReferenceBuilder struct {
	referenceBuilderMap map[int64]*ReferenceBuilder
	references          []*gen.Reference
}

func (rb *MultiReferenceBuilder) CreateReference(dbId int64, referenceType string) {
	// TODO: add referredSemanticId
	// Use ReferenceBuilder to create a new reference for MultiReferenceBuilder
	_, exists := rb.referenceBuilderMap[dbId]
	if exists {
		// Reference already exists, do not create a new one
		return
	}
	reference, builder := NewReferenceBuilder(referenceType)
	rb.referenceBuilderMap[dbId] = builder
	rb.references = append(rb.references, reference)
}

func (rb *MultiReferenceBuilder) CreateKey(dbId int64, key_id int64, key_type string, key_value string) {
	referenceBuilder, exists := rb.referenceBuilderMap[dbId]
	if exists {
		referenceBuilder.CreateKey(key_id, key_type, key_value)
	}
}

func (rb *MultiReferenceBuilder) GetReferences() []*gen.Reference {
	return rb.references
}

// GetReferenceBuilder returns the ReferenceBuilder for a specific dbId
func (rb *MultiReferenceBuilder) GetReferenceBuilder(dbId int64) *ReferenceBuilder {
	return rb.referenceBuilderMap[dbId]
}
