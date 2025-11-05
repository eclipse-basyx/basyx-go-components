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
	"sort"

	"github.com/eclipse-basyx/basyx-go-components/internal/common/model"
)

// SubmodelElementBuilder represents one root level Submodel Element
type SubmodelElementBuilder struct {
	ChildSubmodelElementMap map[int]*ChildSubmodelElementMetadata // Maps database IDs to reference metadata for hierarchy building
	DatabaseID              int
	SubmodelElement         *model.SubmodelElement
}

type ChildSubmodelElementMetadata struct {
	Parent          int
	DatabaseID      int
	SubmodelElement *model.SubmodelElement
}

// BuildSubmodelElement Creates a Root SubmodelElement and the Builder
func BuildSubmodelElement(smeRow SubmodelElementRow) (*model.SubmodelElement, *SubmodelElementBuilder, error) {
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

	supplementalSemanticIDs := []*model.Reference{}
	suppl := []model.Reference{}
	// SupplementalSemanticIDs
	if isArrayNotEmpty(smeRow.SupplementalSemanticIDs) {
		supplementalSemanticIDs, err = ParseReferences(smeRow.SupplementalSemanticIDs, refBuilderMap)
		if err != nil {
			return nil, nil, err
		}
		if moreThanZeroReferences(supplementalSemanticIDs) {
			err = ParseReferredReferences(smeRow.SupplementalSemanticIDsReferred, refBuilderMap)
			if err != nil {
				return nil, nil, err
			}
		}
	}

	// Qualifiers
	if isArrayNotEmpty(smeRow.Qualifiers) {
		builder := NewQualifiersBuilder()
		qualifierRows, err := ParseQualifiersRow(smeRow.Qualifiers)
		if err != nil {
			return nil, nil, err
		}
		for _, qualifierRow := range qualifierRows {
			_, err = builder.AddQualifier(qualifierRow.DbID, qualifierRow.Kind, qualifierRow.Type, qualifierRow.ValueType, qualifierRow.Value, qualifierRow.Position)
			if err != nil {
				return nil, nil, err
			}

			_, err = builder.AddSemanticID(qualifierRow.DbID, qualifierRow.SemanticID, qualifierRow.SemanticIDReferredReferences)
			if err != nil {
				return nil, nil, err
			}

			_, err = builder.AddValueID(qualifierRow.DbID, qualifierRow.ValueID, qualifierRow.ValueIDReferredReferences)
			if err != nil {
				return nil, nil, err
			}

			_, err = builder.AddSupplementalSemanticIDs(qualifierRow.DbID, qualifierRow.SupplementalSemanticIDs, qualifierRow.SupplementalSemanticIDsReferredReferences)
			if err != nil {
				return nil, nil, err
			}
		}
		specificSME.SetQualifiers(builder.Build())
	}

	for _, refBuilder := range refBuilderMap {
		refBuilder.BuildNestedStructure()
	}
	for _, el := range supplementalSemanticIDs {
		suppl = append(suppl, *el)
	}
	if len(suppl) > 0 {
		specificSME.SetSupplementalSemanticIds(suppl)
	}

	return &specificSME, &SubmodelElementBuilder{DatabaseID: int(smeRow.DbID), SubmodelElement: &specificSME}, nil
}

// AddChildSME is Only needed for Lists, Collections, Operations, AnnotatedRelationshipElements and Entites
func (b *SubmodelElementBuilder) AddChildSME(childDbID int, parentID int, childRow SubmodelElementRow) error {
	childSME, _, err := BuildSubmodelElement(childRow)
	if err != nil {
		return err
	}
	if b.ChildSubmodelElementMap == nil {
		b.ChildSubmodelElementMap = make(map[int]*ChildSubmodelElementMetadata)
	}
	b.ChildSubmodelElementMap[childDbID] = &ChildSubmodelElementMetadata{
		Parent:          parentID,
		SubmodelElement: childSME,
	}
	if parentID == b.DatabaseID {
		// Direct child of root, add to root element
		switch (*b.SubmodelElement).GetModelType() {
		case "SubmodelElementCollection":
			collection := (*b.SubmodelElement).(*model.SubmodelElementCollection)
			collection.Value = append(collection.Value, *childSME)
		}
	}
	return nil
}

func (b *SubmodelElementBuilder) BuildHierarchy() {
	// Convert map to slice for sorting
	var sortedMetadata []*ChildSubmodelElementMetadata
	for _, metadata := range b.ChildSubmodelElementMap {
		sortedMetadata = append(sortedMetadata, metadata)
	}

	sort.SliceStable(sortedMetadata, func(i, j int) bool {
		return sortedMetadata[i].DatabaseID < sortedMetadata[j].DatabaseID
	})

	for _, refMetadata := range sortedMetadata {
		if refMetadata.Parent == b.DatabaseID {
			// Already assigned to root, skip
			continue
		}
		parentID := refMetadata.Parent
		submodelElement := refMetadata.SubmodelElement
		parentObj := b.ChildSubmodelElementMap[parentID].SubmodelElement
		switch (*parentObj).GetModelType() {
		case "SubmodelElementCollection":
			collection := (*parentObj).(*model.SubmodelElementCollection)
			collection.Value = append(collection.Value, *submodelElement)
		case "SubmodelElementList":
			fmt.Println("Not implemented")
		}
	}
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
	case "SubmodelElementCollection":
		collection := &model.SubmodelElementCollection{Value: []model.SubmodelElement{}}
		return collection, nil
	// case "Operation":
	// 	operation := &model.Operation{}
	// 	return operation, nil
	// case "Entity":
	// 	entity := &model.Entity{}
	// 	return entity, nil
	// case "AnnotatedRelationshipElement":
	// 	annotatedRelElem := &model.AnnotatedRelationshipElement{}
	// 	return annotatedRelElem, nil
	// case "MultiLanguageProperty":
	// 	mlProp := &model.MultiLanguageProperty{}
	// 	return mlProp, nil
	// case "File":
	// 	file := &model.File{}
	// 	return file, nil
	// case "Blob":
	// 	blob := &model.Blob{}
	// 	return blob, nil
	// case "ReferenceElement":
	// 	refElem := &model.ReferenceElement{}
	// 	return refElem, nil
	// case "RelationshipElement":
	// 	relElem := &model.RelationshipElement{}
	// 	return relElem, nil
	// case "Range":
	// 	rng := &model.Range{}
	// 	return rng, nil
	// case "BasicEventElement":
	// 	eventElem := &model.BasicEventElement{}
	// 	return eventElem, nil
	// case "SubmodelElementList":
	// 	smeList := &model.SubmodelElementList{}
	// 	return smeList, nil
	// case "Capability":
	// 	capability := &model.Capability{}
	// 	return capability, nil
	default:
		return nil, fmt.Errorf("modelType %s is unknown", smeRow.ModelType)
	}
}

func isArrayNotEmpty(data json.RawMessage) bool {
	return len(data) > 0 && string(data) != "null"
}

func moreThanZeroReferences(referenceArray []*model.Reference) bool {
	return len(referenceArray) > 0
}
