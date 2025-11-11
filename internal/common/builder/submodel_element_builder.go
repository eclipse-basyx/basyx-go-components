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
	"sync"

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
	var wg sync.WaitGroup
	refBuilderMap := make(map[int64]*ReferenceBuilder)
	var refMutex sync.RWMutex
	specificSME, err := getSubmodelElementObjectBasedOnModelType(smeRow, refBuilderMap, &refMutex)
	if err != nil {
		return nil, nil, err
	}

	specificSME.SetIdShort(smeRow.IDShort)
	specificSME.SetCategory(smeRow.Category)
	specificSME.SetModelType(smeRow.ModelType)

	// Channels for parallel processing
	type semanticIDResult struct {
		semanticID *model.Reference
		err        error
	}
	type descriptionResult struct {
		descriptions []model.LangStringTextType
		err          error
	}
	type displayNameResult struct {
		displayNames []model.LangStringNameType
		err          error
	}
	type embeddedDataSpecResult struct {
		eds []model.EmbeddedDataSpecification
		err error
	}
	type supplementalSemanticIDsResult struct {
		supplementalSemanticIDs []model.Reference
		err                     error
	}
	type qualifiersResult struct {
		qualifiers []model.Qualifier
		err        error
	}

	semanticIDChan := make(chan semanticIDResult, 1)
	descriptionChan := make(chan descriptionResult, 1)
	displayNameChan := make(chan displayNameResult, 1)
	embeddedDataSpecChan := make(chan embeddedDataSpecResult, 1)
	supplementalSemanticIDsChan := make(chan supplementalSemanticIDsResult, 1)
	qualifiersChan := make(chan qualifiersResult, 1)

	// Parse SemanticID
	wg.Add(1)
	go func() {
		defer wg.Done()
		semanticID, err := getSingleReference(smeRow.SemanticID, smeRow.SemanticIDReferred, refBuilderMap, &refMutex)
		semanticIDChan <- semanticIDResult{semanticID: semanticID, err: err}
	}()

	// Parse Descriptions
	wg.Add(1)
	go func() {
		defer wg.Done()
		if smeRow.Descriptions != nil {
			descriptions, err := ParseLangStringTextType(*smeRow.Descriptions)
			descriptionChan <- descriptionResult{descriptions: descriptions, err: err}
		} else {
			descriptionChan <- descriptionResult{}
		}
	}()

	// Parse DisplayNames
	wg.Add(1)
	go func() {
		defer wg.Done()
		if smeRow.DisplayNames != nil {
			displayNames, err := ParseLangStringNameType(*smeRow.DisplayNames)
			displayNameChan <- displayNameResult{displayNames: displayNames, err: err}
		} else {
			displayNameChan <- displayNameResult{}
		}
	}()

	// Parse EmbeddedDataSpecifications
	wg.Add(1)
	go func() {
		defer wg.Done()
		if smeRow.EmbeddedDataSpecifications != nil {
			var eds []model.EmbeddedDataSpecification
			var json = jsoniter.ConfigCompatibleWithStandardLibrary
			err := json.Unmarshal(*smeRow.EmbeddedDataSpecifications, &eds)
			if err != nil {
				log.Printf("error unmarshaling embedded data specifications: %v", err)
			}
			embeddedDataSpecChan <- embeddedDataSpecResult{eds: eds, err: err}
		} else {
			embeddedDataSpecChan <- embeddedDataSpecResult{}
		}
	}()

	// Parse SupplementalSemanticIDs
	wg.Add(1)
	go func() {
		defer wg.Done()
		if smeRow.SupplementalSemanticIDs != nil {
			var supplementalSemanticIDs []model.Reference
			var json = jsoniter.ConfigCompatibleWithStandardLibrary
			err := json.Unmarshal(*smeRow.SupplementalSemanticIDs, &supplementalSemanticIDs)
			if err != nil {
				log.Printf("error unmarshaling embedded data specifications: %v", err)
			}
			supplementalSemanticIDsChan <- supplementalSemanticIDsResult{supplementalSemanticIDs: supplementalSemanticIDs, err: err}
		} else {
			supplementalSemanticIDsChan <- supplementalSemanticIDsResult{}
		}
	}()

	// Parse Qualifiers
	wg.Add(1)
	go func() {
		defer wg.Done()
		if smeRow.Qualifiers != nil {
			builder := NewQualifiersBuilder()
			qualifierRows, err := ParseQualifiersRow(*smeRow.Qualifiers)
			if err != nil {
				qualifiersChan <- qualifiersResult{err: err}
				return
			}
			for _, qualifierRow := range qualifierRows {
				_, err = builder.AddQualifier(qualifierRow.DbID, qualifierRow.Kind, qualifierRow.Type, qualifierRow.ValueType, qualifierRow.Value, qualifierRow.Position)
				if err != nil {
					qualifiersChan <- qualifiersResult{err: err}
					return
				}

				_, err = builder.AddSemanticID(qualifierRow.DbID, qualifierRow.SemanticID, qualifierRow.SemanticIDReferredReferences)
				if err != nil {
					qualifiersChan <- qualifiersResult{err: err}
					return
				}

				_, err = builder.AddValueID(qualifierRow.DbID, qualifierRow.ValueID, qualifierRow.ValueIDReferredReferences)
				if err != nil {
					qualifiersChan <- qualifiersResult{err: err}
					return
				}

				_, err = builder.AddSupplementalSemanticIDs(qualifierRow.DbID, qualifierRow.SupplementalSemanticIDs, qualifierRow.SupplementalSemanticIDsReferredReferences)
				if err != nil {
					qualifiersChan <- qualifiersResult{err: err}
					return
				}
			}
			qualifiersChan <- qualifiersResult{qualifiers: builder.Build()}
		} else {
			qualifiersChan <- qualifiersResult{}
		}
	}()

	// Wait for all goroutines to complete
	wg.Wait()

	// Collect results from channels
	semIDResult := <-semanticIDChan
	if semIDResult.err != nil {
		return nil, nil, fmt.Errorf("error parsing semantic ID: %w", semIDResult.err)
	}
	if semIDResult.semanticID != nil {
		specificSME.SetSemanticID(semIDResult.semanticID)
	}

	descResult := <-descriptionChan
	if descResult.err != nil {
		return nil, nil, fmt.Errorf("error parsing descriptions: %w", descResult.err)
	}
	if descResult.descriptions != nil {
		specificSME.SetDescription(descResult.descriptions)
	}

	displayResult := <-displayNameChan
	if displayResult.err != nil {
		return nil, nil, fmt.Errorf("error parsing display names: %w", displayResult.err)
	}
	if displayResult.displayNames != nil {
		specificSME.SetDisplayName(displayResult.displayNames)
	}

	edsResult := <-embeddedDataSpecChan
	if edsResult.err != nil {
		return nil, nil, fmt.Errorf("error parsing embedded data specifications: %w", edsResult.err)
	}
	if edsResult.eds != nil {
		specificSME.SetEmbeddedDataSpecifications(edsResult.eds)
	}

	supplResult := <-supplementalSemanticIDsChan
	if supplResult.err != nil {
		return nil, nil, fmt.Errorf("error parsing supplemental semantic IDs: %w", supplResult.err)
	}

	qualResult := <-qualifiersChan
	if qualResult.err != nil {
		return nil, nil, fmt.Errorf("error parsing qualifiers: %w", qualResult.err)
	}
	if qualResult.qualifiers != nil {
		specificSME.SetQualifiers(qualResult.qualifiers)
	}

	// Build nested structure for references
	for _, refBuilder := range refBuilderMap {
		refBuilder.BuildNestedStructure()
	}

	// Set supplemental semantic IDs if present
	if len(supplResult.supplementalSemanticIDs) > 0 {
		suppl := []model.Reference{}
		suppl = append(suppl, supplResult.supplementalSemanticIDs...)
		specificSME.SetSupplementalSemanticIds(suppl)
	}

	return &specificSME, &SubmodelElementBuilder{DatabaseID: int(smeRow.DbID.Int64), SubmodelElement: &specificSME}, nil
}

func getSubmodelElementObjectBasedOnModelType(smeRow model.SubmodelElementRow, refBuilderMap map[int64]*ReferenceBuilder, refMutex *sync.RWMutex) (model.SubmodelElement, error) {
	switch smeRow.ModelType {
	case "Property":
		prop, err := buildProperty(smeRow, refBuilderMap, refMutex)
		if err != nil {
			return nil, err
		}
		return prop, nil
	case "SubmodelElementCollection":
		return buildSubmodelElementCollection()
	case "Operation":
		var json = jsoniter.ConfigCompatibleWithStandardLibrary
		var valueRow model.OperationValueRow
		if smeRow.Value == nil {
			return nil, fmt.Errorf("smeRow.Value is nil")
		}
		err := json.Unmarshal(*smeRow.Value, &valueRow)
		if err != nil {
			return nil, err
		}

		var inputVars, outputVars, inoutputVars []model.OperationVariable
		if valueRow.InputVariables != nil {
			err = json.Unmarshal(valueRow.InputVariables, &inputVars)
			if err != nil {
				return nil, err
			}
		}
		if valueRow.OutputVariables != nil {
			err = json.Unmarshal(valueRow.OutputVariables, &outputVars)
			if err != nil {
				return nil, err
			}
		}
		if valueRow.InoutputVariables != nil {
			err = json.Unmarshal(valueRow.InoutputVariables, &inoutputVars)
			if err != nil {
				return nil, err
			}
		}

		operation := &model.Operation{
			InputVariables:    inputVars,
			OutputVariables:   outputVars,
			InoutputVariables: inoutputVars,
		}
		return operation, nil
	case "Entity":
		var valueRow model.EntityValueRow
		if smeRow.Value == nil {
			return nil, fmt.Errorf("smeRow.Value is nil")
		}
		err := json.Unmarshal(*smeRow.Value, &valueRow)
		if err != nil {
			return nil, err
		}

		entityType, err := model.NewEntityTypeFromValue(valueRow.EntityType)
		if err != nil {
			return nil, err
		}

		var statements []model.SubmodelElement
		if valueRow.Statements != nil {
			var stmtJSONs []json.RawMessage
			err = json.Unmarshal(valueRow.Statements, &stmtJSONs)
			if err != nil {
				return nil, err
			}
			for _, stmtJSON := range stmtJSONs {
				stmt, err := model.UnmarshalSubmodelElement(stmtJSON)
				if err != nil {
					return nil, err
				}
				statements = append(statements, stmt)
			}
		}

		var specificAssetIDs []model.SpecificAssetID
		if valueRow.SpecificAssetIDs != nil {
			err = json.Unmarshal(valueRow.SpecificAssetIDs, &specificAssetIDs)
			if err != nil {
				return nil, err
			}
		}

		entity := &model.Entity{
			EntityType:       entityType,
			GlobalAssetID:    valueRow.GlobalAssetID,
			Statements:       statements,
			SpecificAssetIds: specificAssetIDs,
		}
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
		eventElem, err := buildBasicEventElement(smeRow, refBuilderMap, refMutex)
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

func buildProperty(smeRow model.SubmodelElementRow, refBuilderMap map[int64]*ReferenceBuilder, refMutex *sync.RWMutex) (*model.Property, error) {
	var valueRow model.PropertyValueRow
	if smeRow.Value == nil {
		return nil, fmt.Errorf("smeRow.Value is nil")
	}
	err := json.Unmarshal(*smeRow.Value, &valueRow)
	if err != nil {
		return nil, err
	}
	valueID, err := getSingleReference(&valueRow.ValueID, &valueRow.ValueIDReferred, refBuilderMap, refMutex)
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

func buildBasicEventElement(smeRow model.SubmodelElementRow, refBuilderMap map[int64]*ReferenceBuilder, refMutex *sync.RWMutex) (*model.BasicEventElement, error) {
	var valueRow model.BasicEventElementValueRow
	if smeRow.Value == nil {
		return nil, fmt.Errorf("smeRow.Value is nil")
	}
	err := json.Unmarshal(*smeRow.Value, &valueRow)
	if err != nil {
		return nil, err
	}
	observedRefs, err := getSingleReference(&valueRow.ObservedRef, &valueRow.ObservedRefReferred, refBuilderMap, refMutex)
	if err != nil {
		return nil, err
	}
	messageBrokerRefs, err := getSingleReference(&valueRow.MessageBrokerRef, &valueRow.MessageBrokerRefReferred, refBuilderMap, refMutex)
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

func getSingleReference(reference *json.RawMessage, referredReference *json.RawMessage, refBuilderMap map[int64]*ReferenceBuilder, refMutex *sync.RWMutex) (*model.Reference, error) {
	var refs []*model.Reference
	var err error
	if reference != nil {
		refs, err = ParseReferences(*reference, refBuilderMap, refMutex)
		if err != nil {
			return nil, err
		}
		if referredReference != nil {
			if err = ParseReferredReferences(*referredReference, refBuilderMap, refMutex); err != nil {
				return nil, err
			}
		}
	}
	if len(refs) > 0 {
		return refs[0], nil
	}
	return nil, nil
}
