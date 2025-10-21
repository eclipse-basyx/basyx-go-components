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

// Author: Aaron Zielstorff ( Fraunhofer IESE ), Jannik Fried ( Fraunhofer IESE )
package builder

import (
	"encoding/json"
	"fmt"

	gen "github.com/eclipse-basyx/basyx-go-components/internal/common/model"
)

type QualifiersBuilder struct {
	qualifiers    map[int64]*gen.Qualifier
	refBuilderMap map[int64]*ReferenceBuilder
}

func NewQualifiersBuilder() *QualifiersBuilder {
	return &QualifiersBuilder{qualifiers: make(map[int64]*gen.Qualifier), refBuilderMap: make(map[int64]*ReferenceBuilder)}
}

func (b *QualifiersBuilder) AddQualifier(qualifierDbId int64, kind string, qType string, valueType string, value string) (*QualifiersBuilder, error) {
	_, exists := b.qualifiers[qualifierDbId]
	if !exists {
		Kind, err := gen.NewQualifierKindFromValue(kind)
		if err != nil {
			fmt.Println(err)
			return nil, fmt.Errorf("error parsing Qualifier Kind to Go Struct for Qualifier '%d'. See console for details", qualifierDbId)
		}
		ValueType, err := gen.NewDataTypeDefXsdFromValue(valueType)
		if err != nil {
			fmt.Println(err)
			return nil, fmt.Errorf("error parsing ValueType for Qualifier '%d' to Go Struct. See console for details", qualifierDbId)
		}
		b.qualifiers[qualifierDbId] = &gen.Qualifier{
			Kind:      Kind,
			Type:      qType,
			ValueType: ValueType,
			Value:     value,
		}
	} else {
		fmt.Printf("[Warning] qualifier with id '%d' already exists - skipping.", qualifierDbId)
	}
	return b, nil
}

func (b *QualifiersBuilder) AddSemanticId(qualifierDbId int64, semanticIdRows json.RawMessage, semanticIdReferredSemanticIdRows json.RawMessage) (*QualifiersBuilder, error) {
	qualifier := b.qualifiers[qualifierDbId]

	semanticId, err := b.createExactlyOneReference(qualifierDbId, semanticIdRows, semanticIdReferredSemanticIdRows, "SemanticID")

	if err != nil {
		return nil, err
	}

	qualifier.SemanticId = semanticId

	return b, nil
}

func (b *QualifiersBuilder) AddValueId(qualifierDbId int64, valueIdRows json.RawMessage, valueIdReferredSemanticIdRows json.RawMessage) (*QualifiersBuilder, error) {
	qualifier := b.qualifiers[qualifierDbId]

	valueId, err := b.createExactlyOneReference(qualifierDbId, valueIdRows, valueIdReferredSemanticIdRows, "ValueId")

	if err != nil {
		return nil, err
	}

	qualifier.ValueId = valueId
	return b, nil
}

func (b *QualifiersBuilder) AddSupplementalSemanticIds(qualifierDbId int64, supplementalSemanticIdsRows json.RawMessage, supplementalSemanticIdsReferredSemanticIdRows json.RawMessage) (*QualifiersBuilder, error) {
	qualifier, exists := b.qualifiers[qualifierDbId]

	if !exists {
		return nil, fmt.Errorf("tried to add SupplementalSemanticIds to Qualifier '%d' before creating the Qualifier itself", qualifierDbId)
	}

	refs, err := ParseReferences(supplementalSemanticIdsRows, b.refBuilderMap)

	if err != nil {
		return nil, err
	}

	if len(supplementalSemanticIdsReferredSemanticIdRows) > 0 {
		ParseReferredReferences(supplementalSemanticIdsReferredSemanticIdRows, b.refBuilderMap)
	}

	suppl := []gen.Reference{}

	for _, el := range refs {
		suppl = append(suppl, *el)
	}

	qualifier.SupplementalSemanticIds = suppl

	return b, nil
}

func (b *QualifiersBuilder) createExactlyOneReference(qualifierDbId int64, refRows json.RawMessage, referredRefRows json.RawMessage, Type string) (*gen.Reference, error) {
	_, exists := b.qualifiers[qualifierDbId]

	if !exists {
		return nil, fmt.Errorf("tried to add %s to Qualifier '%d' before creating the Qualifier itself", Type, qualifierDbId)
	}

	refs, err := ParseReferences(refRows, b.refBuilderMap)

	if err != nil {
		return nil, err
	}

	if len(referredRefRows) > 0 {
		ParseReferredReferences(referredRefRows, b.refBuilderMap)
	}

	if len(refs) != 1 {
		return nil, fmt.Errorf("expected exactly one or no %s for Qualifier '%d' but got %d", Type, qualifierDbId, len(refs))
	}

	return refs[0], nil
}

func (b *QualifiersBuilder) Build() []gen.Qualifier {

	for _, builder := range b.refBuilderMap {
		builder.BuildNestedStructure()
	}

	qualifiers := make([]gen.Qualifier, 0, len(b.qualifiers))
	for _, qualifier := range b.qualifiers {
		qualifiers = append(qualifiers, *qualifier)
	}

	return qualifiers
}
