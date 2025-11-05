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
	"encoding/json"
	"fmt"
	"log"

	"github.com/eclipse-basyx/basyx-go-components/internal/common/model"
)

// SubmodelElementBuilder represents one root level Submodel Element
type SubmodelElementBuilder struct {
}

// NewSMEBuilder Creates a Root SubmodelElement and the Builder
func NewSMEBuilder(smeRow SubmodelElementRow) (*model.SubmodelElement, *SubmodelElementBuilder, error) {
	refBuilderMap := make(map[int64]*ReferenceBuilder)
	specificSME, err := getSubmodelElementObjectBasedOnModelType(smeRow, refBuilderMap)
	if err != nil {
		return nil, nil, err
	}
	specificSME.SetIdShort(smeRow.IDShort)
	specificSME.SetCategory(smeRow.Category)
	specificSME.SetModelType(smeRow.ModelType)

	refs, err := ParseReferences(smeRow.SemanticID, refBuilderMap)
	if err != nil {
		return nil, nil, err
	}

	if err = ParseReferredReferences(smeRow.SemanticIDReferred, refBuilderMap); err != nil {
		return nil, nil, err
	}

	if len(refs) > 0 {
		specificSME.SetSemanticID(refs[0])
	}

	if isArrayNotEmpty(smeRow.Descriptions) {
		descriptions, err := ParseLangStringTextType(smeRow.Descriptions)
		if err != nil {
			return nil, nil, fmt.Errorf("error parsing descriptions: %w", err)
		}
		specificSME.SetDescription(descriptions)
	}

	if isArrayNotEmpty(smeRow.DisplayNames) {
		displayNames, err := ParseLangStringNameType(smeRow.DisplayNames)
		if err != nil {
			return nil, nil, fmt.Errorf("error parsing display names: %w", err)
		}
		specificSME.SetDisplayName(displayNames)
	}

	builder := NewEmbeddedDataSpecificationsBuilder()

	err = builder.BuildContentsIec61360(smeRow.DataSpecIEC61360)
	if err != nil {
		log.Printf("Failed to build contents: %v", err)
	}

	err = builder.BuildReferences(smeRow.DataSpecReference, smeRow.DataSpecReferenceReferred)
	if err != nil {
		log.Printf("Failed to build references: %v", err)
	}

	eds := builder.Build()

	if len(eds) > 0 {
		specificSME.SetEmbeddedDataSpecifications(eds)
	}

	for _, refBuilder := range refBuilderMap {
		refBuilder.BuildNestedStructure()
	}

	return &specificSME, &SubmodelElementBuilder{}, nil
}

// AddChildSME is Only needed for Lists, Collections, Operations, AnnotatedRelationshipElements and Entites
func (b *SubmodelElementBuilder) AddChildSME() error {
	return nil
}

func getSubmodelElementObjectBasedOnModelType(smeRow SubmodelElementRow, refBuilderMap map[int64]*ReferenceBuilder) (model.SubmodelElement, error) {
	switch smeRow.ModelType {
	case "Property":
		var valueRow PropertyValueRow
		err := json.Unmarshal(smeRow.Value, &valueRow)
		if err != nil {
			return nil, err
		}
		var refs []*model.Reference
		if isArrayNotEmpty(valueRow.ValueID) {
			refs, err = ParseReferences(valueRow.ValueID, refBuilderMap)
			if err != nil {
				return nil, err
			}
			if isArrayNotEmpty(valueRow.ValueIDReferred) {
				if err = ParseReferredReferences(valueRow.ValueIDReferred, refBuilderMap); err != nil {
					return nil, err
				}
			}
		}

		prop := &model.Property{
			Value:     valueRow.Value,
			ValueType: valueRow.ValueType,
		}

		if len(refs) > 0 {
			prop.ValueID = refs[0]
		}

		return prop, nil
	default:
		return nil, fmt.Errorf("modelType %s is unknown", smeRow.ModelType)
	}
}

func isArrayNotEmpty(data json.RawMessage) bool {
	return len(data) > 0 && string(data) != "null"
}
