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
	jsoniter "github.com/json-iterator/go"
)

// SubmodelElementBuilder represents one root level Submodel Element
type SubmodelElementBuilder struct {
	DatabaseID      int
	SubmodelElement *model.SubmodelElement
}

// BuildSubmodelElement Creates a Root SubmodelElement and the Builder
func BuildSubmodelElement(smeRow model.SubmodelElementRow) (*model.SubmodelElement, *SubmodelElementBuilder, error) {
	refBuilderMap := make(map[int64]*ReferenceBuilder)
	specificSME, err := getSubmodelElementObjectBasedOnModelType(smeRow, refBuilderMap)
	if err != nil {
		return nil, nil, err
	}
	semanticID, err := getSingleReference(smeRow.SemanticID, smeRow.SemanticIDReferred, refBuilderMap)
	if err != nil {
		return nil, nil, err
	}
	if semanticID != nil {
		specificSME.SetSemanticID(semanticID)
	}
	specificSME.SetIdShort(smeRow.IDShort)
	specificSME.SetCategory(smeRow.Category)
	specificSME.SetModelType(smeRow.ModelType)

	if smeRow.Descriptions != nil {
		descriptions, err := ParseLangStringTextType(*smeRow.Descriptions)
		if err != nil {
			return nil, nil, fmt.Errorf("error parsing descriptions: %w", err)
		}
		specificSME.SetDescription(descriptions)
	}

	if smeRow.DisplayNames != nil {
		displayNames, err := ParseLangStringNameType(*smeRow.DisplayNames)
		if err != nil {
			return nil, nil, fmt.Errorf("error parsing display names: %w", err)
		}
		specificSME.SetDisplayName(displayNames)
	}

	if smeRow.EmbeddedDataSpecifications != nil {
		var eds []model.EmbeddedDataSpecification
		var json = jsoniter.ConfigCompatibleWithStandardLibrary
		err := json.Unmarshal(*smeRow.EmbeddedDataSpecifications, &eds)
		if err != nil {
			log.Printf("error unmarshaling embedded data specifications: %v", err)
			return nil, nil, err
		}
		specificSME.SetEmbeddedDataSpecifications(eds)
	}

	supplementalSemanticIDs := []*model.Reference{}
	suppl := []model.Reference{}
	// SupplementalSemanticIDs
	if smeRow.SupplementalSemanticIDs != nil {
		supplementalSemanticIDs, err = ParseReferences(*smeRow.SupplementalSemanticIDs, refBuilderMap)
		if err != nil {
			return nil, nil, err
		}
		// if moreThanZeroReferences(supplementalSemanticIDs) {
		// 	err = ParseReferredReferences(smeRow.SupplementalSemanticIDsReferred, refBuilderMap)
		// 	if err != nil {
		// 		return nil, nil, err
		// 	}
		// }
	}

	// Qualifiers
	if smeRow.Qualifiers != nil {
		builder := NewQualifiersBuilder()
		qualifierRows, err := ParseQualifiersRow(*smeRow.Qualifiers)
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

	return &specificSME, &SubmodelElementBuilder{DatabaseID: int(smeRow.DbID.Int64), SubmodelElement: &specificSME}, nil
}

func getSubmodelElementObjectBasedOnModelType(smeRow model.SubmodelElementRow, refBuilderMap map[int64]*ReferenceBuilder) (model.SubmodelElement, error) {
	switch smeRow.ModelType {
	case "Property":
		prop, err := buildProperty(smeRow, refBuilderMap)
		if err != nil {
			return nil, err
		}
		return prop, nil
	case "SubmodelElementCollection":
		return buildSubmodelElementCollection()
	case "Operation":
		operation := &model.Operation{}
		return operation, nil
	case "Entity":
		entity := &model.Entity{}
		return entity, nil
	case "AnnotatedRelationshipElement":
		annotatedRelElem := &model.AnnotatedRelationshipElement{}
		return annotatedRelElem, nil
	case "MultiLanguageProperty":
		mlProp := &model.MultiLanguageProperty{}
		return mlProp, nil
	case "File":
		file := &model.File{}
		return file, nil
	case "Blob":
		blob := &model.Blob{}
		return blob, nil
	case "ReferenceElement":
		refElem := &model.ReferenceElement{}
		return refElem, nil
	case "RelationshipElement":
		relElem := &model.RelationshipElement{}
		return relElem, nil
	case "Range":
		rng := &model.Range{}
		return rng, nil
	case "BasicEventElement":
		eventElem, err := buildBasicEventElement(smeRow, refBuilderMap)
		if err != nil {
			return nil, err
		}
		return eventElem, nil
	case "SubmodelElementList":
		smeList := &model.SubmodelElementList{Value: []model.SubmodelElement{}}
		return smeList, nil
	case "Capability":
		capability := &model.Capability{}
		return capability, nil
	default:
		return nil, fmt.Errorf("modelType %s is unknown", smeRow.ModelType)
	}
}

func buildSubmodelElementCollection() (model.SubmodelElement, error) {
	collection := &model.SubmodelElementCollection{Value: []model.SubmodelElement{}}
	return collection, nil
}

func buildProperty(smeRow model.SubmodelElementRow, refBuilderMap map[int64]*ReferenceBuilder) (*model.Property, error) {
	var valueRow model.PropertyValueRow
	if smeRow.Value == nil {
		return nil, fmt.Errorf("smeRow.Value is nil")
	}
	err := json.Unmarshal(*smeRow.Value, &valueRow)
	if err != nil {
		return nil, err
	}
	valueID, err := getSingleReference(&valueRow.ValueID, &valueRow.ValueIDReferred, refBuilderMap)
	if err != nil {
		return nil, err
	}

	prop := &model.Property{
		Value:     valueRow.Value,
		ValueType: valueRow.ValueType,
		ValueID:   valueID,
	}

	return prop, nil
}

func buildBasicEventElement(smeRow model.SubmodelElementRow, refBuilderMap map[int64]*ReferenceBuilder) (*model.BasicEventElement, error) {
	var valueRow model.BasicEventElementValueRow
	if smeRow.Value == nil {
		return nil, fmt.Errorf("smeRow.Value is nil")
	}
	err := json.Unmarshal(*smeRow.Value, &valueRow)
	if err != nil {
		return nil, err
	}
	observedRefs, err := getSingleReference(&valueRow.ObservedRef, &valueRow.ObservedRefReferred, refBuilderMap)
	if err != nil {
		return nil, err
	}
	messageBrokerRefs, err := getSingleReference(&valueRow.MessageBrokerRef, &valueRow.MessageBrokerRefReferred, refBuilderMap)
	if err != nil {
		return nil, err
	}

	state, err := model.NewStateOfEventFromValue(valueRow.State)
	if err != nil {
		return nil, err
	}

	direction, err := model.NewDirectionFromValue(valueRow.Direction)
	if err != nil {
		return nil, err
	}

	bee := &model.BasicEventElement{
		Direction:     direction,
		State:         state,
		MessageTopic:  valueRow.MessageTopic,
		LastUpdate:    valueRow.LastUpdate,
		MinInterval:   valueRow.MinInterval,
		MaxInterval:   valueRow.MaxInterval,
		Observed:      observedRefs,
		MessageBroker: messageBrokerRefs,
	}
	return bee, nil
}

func moreThanZeroReferences(referenceArray []*model.Reference) bool {
	return len(referenceArray) > 0
}

func getSingleReference(reference *json.RawMessage, referredReference *json.RawMessage, refBuilderMap map[int64]*ReferenceBuilder) (*model.Reference, error) {
	var refs []*model.Reference
	var err error
	if reference != nil {
		refs, err = ParseReferences(*reference, refBuilderMap)
		if err != nil {
			return nil, err
		}
		if referredReference != nil {
			if err = ParseReferredReferences(*referredReference, refBuilderMap); err != nil {
				return nil, err
			}
		}
	}
	if len(refs) > 0 {
		return refs[0], nil
	}
	return nil, nil
}
