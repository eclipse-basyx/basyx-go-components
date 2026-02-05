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

package builder

import (
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"strings"
	"sync"

	"github.com/FriedJannik/aas-go-sdk/jsonization"
	"github.com/FriedJannik/aas-go-sdk/types"
	"github.com/eclipse-basyx/basyx-go-components/internal/common"
	"github.com/eclipse-basyx/basyx-go-components/internal/common/model"
	jsoniter "github.com/json-iterator/go"
	"golang.org/x/sync/errgroup"
)

// SubmodelElementBuilder encapsulates the database ID and the constructed SubmodelElement,
// providing a way to manage and build submodel elements from database rows.
type SubmodelElementBuilder struct {
	DatabaseID      int
	SubmodelElement types.ISubmodelElement
}

// Channels for parallel processing
type semanticIDResult struct {
	semanticID types.IReference
}
type descriptionResult struct {
	descriptions []types.ILangStringTextType
}
type displayNameResult struct {
	displayNames []types.ILangStringNameType
}
type embeddedDataSpecResult struct {
	eds []types.IEmbeddedDataSpecification
}
type supplementalSemanticIDsResult struct {
	supplementalSemanticIDs []types.IReference
}
type qualifiersResult struct {
	qualifiers []types.IQualifier
}
type extensionsResult struct {
	extensions []types.IExtension
}

// BuildSubmodelElement constructs a SubmodelElement from the provided database row.
// It parses the row data, builds the appropriate submodel element type, and sets common attributes
// like IDShort, Category, and ModelType. It also handles parallel parsing of related data such as
// semantic IDs, descriptions, and qualifiers. Returns the constructed SubmodelElement and a
// SubmodelElementBuilder for further management.
// nolint:revive // This method is already refactored and further changes would not improve readability.
func BuildSubmodelElement(smeRow model.SubmodelElementRow, db *sql.DB) (types.ISubmodelElement, *SubmodelElementBuilder, error) {
	var g errgroup.Group
	refBuilderMap := make(map[int64]*ReferenceBuilder)
	var refMutex sync.RWMutex
	specificSME, err := getSubmodelElementObjectBasedOnModelType(smeRow, refBuilderMap, &refMutex, db)
	if err != nil {
		_, _ = fmt.Printf("[DEBUG] BuildSubmodelElement: Error building SME type, idShort=%s, modelType=%d, error: %v\n", smeRow.IDShort, smeRow.ModelType, err)
		return nil, nil, err
	}
	if smeRow.IDShort.Valid {
		specificSME.SetIDShort(&smeRow.IDShort.String)
	}
	if smeRow.Category.Valid {
		specificSME.SetCategory(&smeRow.Category.String)
	}

	semanticIDChan := make(chan semanticIDResult, 1)
	descriptionChan := make(chan descriptionResult, 1)
	displayNameChan := make(chan displayNameResult, 1)
	embeddedDataSpecChan := make(chan embeddedDataSpecResult, 1)
	supplementalSemanticIDsChan := make(chan supplementalSemanticIDsResult, 1)
	qualifiersChan := make(chan qualifiersResult, 1)
	extensionsChan := make(chan extensionsResult, 1)

	// Parse SemanticID
	g.Go(func() error {
		semanticID, err := getSingleReference(smeRow.SemanticID, smeRow.SemanticIDReferred, refBuilderMap, &refMutex)
		if err != nil {
			return err
		}
		semanticIDChan <- semanticIDResult{semanticID: semanticID}
		return nil
	})

	// Parse Descriptions
	g.Go(func() error {
		if smeRow.Descriptions != nil {
			descriptions, err := ParseLangStringTextType(*smeRow.Descriptions)
			if err != nil {
				return err
			}
			descriptionChan <- descriptionResult{descriptions: descriptions}
		} else {
			descriptionChan <- descriptionResult{}
		}
		return nil
	})

	// Parse DisplayNames
	g.Go(func() error {
		if smeRow.DisplayNames != nil {
			displayNames, err := ParseLangStringNameType(*smeRow.DisplayNames)
			if err != nil {
				return err
			}
			displayNameChan <- displayNameResult{displayNames: displayNames}
		} else {
			displayNameChan <- displayNameResult{}
		}
		return nil
	})

	// Parse EmbeddedDataSpecifications
	g.Go(func() error {
		if smeRow.EmbeddedDataSpecifications != nil {
			var edsJsonable []map[string]any
			var json = jsoniter.ConfigCompatibleWithStandardLibrary
			err := json.Unmarshal(*smeRow.EmbeddedDataSpecifications, &edsJsonable)
			var specs []types.IEmbeddedDataSpecification
			for i, jsonable := range edsJsonable {
				eds, err := jsonization.EmbeddedDataSpecificationFromJsonable(jsonable)
				if err != nil {
					// Log the problematic JSON for debugging
					jsonBytes, _ := json.Marshal(jsonable)
					_, _ = fmt.Printf("[DEBUG] SME EmbeddedDataSpec: idShort=%s, index=%d, JSON: %s, Error: %v\n", smeRow.IDShort, i, string(jsonBytes), err)
					return fmt.Errorf("error converting jsonable to EmbeddedDataSpecification (idShort=%s, index=%d, data: %s): %w", smeRow.IDShort, i, string(jsonBytes), err)
				}
				specs = append(specs, eds)
			}
			if err != nil {
				return fmt.Errorf("error unmarshaling embedded data specifications: %w", err)
			}
			embeddedDataSpecChan <- embeddedDataSpecResult{eds: specs}
		} else {
			embeddedDataSpecChan <- embeddedDataSpecResult{}
		}
		return nil
	})

	// Parse SupplementalSemanticIDs
	g.Go(func() error {
		if smeRow.SupplementalSemanticIDs != nil {
			var supplementalSemanticIDsJsonable []map[string]any
			var json = jsoniter.ConfigCompatibleWithStandardLibrary
			err := json.Unmarshal(*smeRow.SupplementalSemanticIDs, &supplementalSemanticIDsJsonable)
			if err != nil {
				return fmt.Errorf("error unmarshaling supplemental semantic IDs: %w", err)
			}
			var supplementalSemanticIDs []types.Reference
			for i, jsonable := range supplementalSemanticIDsJsonable {
				ref, err := jsonization.ReferenceFromJsonable(jsonable)
				if err != nil {
					// Log the problematic JSON for debugging
					jsonBytes, _ := json.Marshal(jsonable)
					_, _ = fmt.Printf("[DEBUG] SME SupplementalSemanticIDs: idShort=%s, index=%d, JSON: %s, Error: %v\n", smeRow.IDShort, i, string(jsonBytes), err)
					return fmt.Errorf("error converting jsonable to Reference (idShort=%s, index=%d, data: %s): %w", smeRow.IDShort, i, string(jsonBytes), err)
				}
				supplementalSemanticIDs = append(supplementalSemanticIDs, *ref.(*types.Reference))
			}
			iSupplementalSemanticIDs := make([]types.IReference, len(supplementalSemanticIDs))
			for i, ref := range supplementalSemanticIDs {
				iSupplementalSemanticIDs[i] = &ref
			}
			supplementalSemanticIDsChan <- supplementalSemanticIDsResult{supplementalSemanticIDs: iSupplementalSemanticIDs}
		} else {
			supplementalSemanticIDsChan <- supplementalSemanticIDsResult{}
		}
		return nil
	})

	// Parse Extensions
	g.Go(func() error {
		if smeRow.Extensions != nil {
			var extensionsJsonable []map[string]any
			var json = jsoniter.ConfigCompatibleWithStandardLibrary
			err := json.Unmarshal(*smeRow.Extensions, &extensionsJsonable)
			var extensions []types.IExtension
			for i, jsonable := range extensionsJsonable {
				ext, err := jsonization.ExtensionFromJsonable(jsonable)
				if err != nil {
					// Log the problematic JSON for debugging
					jsonBytes, _ := json.Marshal(jsonable)
					_, _ = fmt.Printf("[DEBUG] SME Extensions: idShort=%s, index=%d, JSON: %s, Error: %v\n", smeRow.IDShort, i, string(jsonBytes), err)
					return fmt.Errorf("error converting jsonable to Extension (idShort=%s, index=%d, data: %s): %w", smeRow.IDShort, i, string(jsonBytes), err)
				}
				extensions = append(extensions, ext)
			}
			if err != nil {
				return fmt.Errorf("error unmarshaling extensions: %w", err)
			}
			extensionsChan <- extensionsResult{extensions: extensions}
		} else {
			extensionsChan <- extensionsResult{}
		}
		return nil
	})

	// Parse Qualifiers
	g.Go(func() error {
		if smeRow.Qualifiers != nil {
			builder := NewQualifiersBuilder()
			qualifierRows, err := ParseQualifiersRow(*smeRow.Qualifiers)
			if err != nil {
				return err
			}
			for _, qualifierRow := range qualifierRows {
				_, err = builder.AddQualifier(qualifierRow.DbID, qualifierRow.Type, qualifierRow.ValueType, qualifierRow.Value, qualifierRow.Position, qualifierRow.Kind)
				if err != nil {
					return err
				}

				_, err = builder.AddSemanticID(qualifierRow.DbID, qualifierRow.SemanticID, qualifierRow.SemanticIDReferredReferences)
				if err != nil {
					return err
				}

				_, err = builder.AddValueID(qualifierRow.DbID, qualifierRow.ValueID, qualifierRow.ValueIDReferredReferences)
				if err != nil {
					return err
				}

				_, err = builder.AddSupplementalSemanticIDs(qualifierRow.DbID, qualifierRow.SupplementalSemanticIDs, qualifierRow.SupplementalSemanticIDsReferredReferences)
				if err != nil {
					return err
				}
			}
			qualifiersChan <- qualifiersResult{qualifiers: builder.Build()}
		} else {
			qualifiersChan <- qualifiersResult{}
		}
		return nil
	})

	// Wait for all goroutines to complete
	if err := g.Wait(); err != nil {
		return nil, nil, err
	}

	// Collect results from channels
	semIDResult := <-semanticIDChan
	if semIDResult.semanticID != nil {
		specificSME.SetSemanticID(semIDResult.semanticID)
	}

	extResult := <-extensionsChan
	if len(extResult.extensions) > 0 {
		specificSME.SetExtensions(extResult.extensions)
	}

	descResult := <-descriptionChan
	if len(descResult.descriptions) > 0 {
		specificSME.SetDescription(descResult.descriptions)
	}

	displayResult := <-displayNameChan
	if len(displayResult.displayNames) > 0 {
		specificSME.SetDisplayName(displayResult.displayNames)
	}

	edsResult := <-embeddedDataSpecChan
	if len(edsResult.eds) > 0 {
		specificSME.SetEmbeddedDataSpecifications(edsResult.eds)
	}

	supplResult := <-supplementalSemanticIDsChan

	qualResult := <-qualifiersChan
	if len(qualResult.qualifiers) > 0 {
		specificSME.SetQualifiers(qualResult.qualifiers)
	}

	// Build nested structure for references
	for _, refBuilder := range refBuilderMap {
		refBuilder.BuildNestedStructure()
	}

	// Set supplemental semantic IDs if present
	if len(supplResult.supplementalSemanticIDs) > 0 {
		specificSME.SetSupplementalSemanticIDs(supplResult.supplementalSemanticIDs)
	}

	return specificSME, &SubmodelElementBuilder{DatabaseID: int(smeRow.DbID.Int64), SubmodelElement: specificSME}, nil
}

// getSubmodelElementObjectBasedOnModelType determines the specific SubmodelElement type
// based on the ModelType field in the row and delegates to the appropriate build function.
// It handles reference building for types that require it.
func getSubmodelElementObjectBasedOnModelType(smeRow model.SubmodelElementRow, refBuilderMap map[int64]*ReferenceBuilder, refMutex *sync.RWMutex, db *sql.DB) (types.ISubmodelElement, error) {
	switch smeRow.ModelType {
	case int64(types.ModelTypeProperty):
		prop, err := buildProperty(smeRow, refBuilderMap, refMutex)
		if err != nil {
			return nil, err
		}
		return prop, nil
	case int64(types.ModelTypeSubmodelElementCollection):
		return buildSubmodelElementCollection()
	case int64(types.ModelTypeOperation):
		return buildOperation(smeRow)
	case int64(types.ModelTypeEntity):
		return buildEntity(smeRow)
	case int64(types.ModelTypeAnnotatedRelationshipElement):
		return buildAnnotatedRelationshipElement(smeRow)
	case int64(types.ModelTypeMultiLanguageProperty):
		mlProp, err := buildMultiLanguageProperty(smeRow)
		if err != nil {
			return nil, err
		}
		return mlProp, nil
	case int64(types.ModelTypeFile):
		file, err := buildFile(smeRow)
		if err != nil {
			return nil, err
		}
		return file, nil
	case int64(types.ModelTypeBlob):
		blob, err := buildBlob(smeRow)
		if err != nil {
			return nil, err
		}
		return blob, nil
	case int64(types.ModelTypeReferenceElement):
		return buildReferenceElement(smeRow)
	case int64(types.ModelTypeRelationshipElement):
		return buildRelationshipElement(smeRow)
	case int64(types.ModelTypeRange):
		rng, err := buildRange(smeRow)
		if err != nil {
			return nil, err
		}
		return rng, nil
	case int64(types.ModelTypeBasicEventElement):
		eventElem, err := buildBasicEventElement(smeRow)
		if err != nil {
			return nil, err
		}
		return eventElem, nil
	case int64(types.ModelTypeSubmodelElementList):
		return buildSubmodelElementList(smeRow)
	case int64(types.ModelTypeCapability):
		capability, err := buildCapability()
		if err != nil {
			return nil, err
		}
		return capability, nil
	default:
		return nil, common.NewInternalServerError(fmt.Sprintf("Received invalid ModelType: %d while constructing SubmodelElement", smeRow.ModelType))
	}
}

// buildSubmodelElementCollection creates a new SubmodelElementCollection with an empty value slice.
func buildSubmodelElementCollection() (types.ISubmodelElement, error) {
	collection := types.NewSubmodelElementCollection()
	return collection, nil
}

// buildProperty constructs a Property SubmodelElement from the database row,
// including parsing the value and building the associated value reference.
func buildProperty(smeRow model.SubmodelElementRow, refBuilderMap map[int64]*ReferenceBuilder, refMutex *sync.RWMutex) (types.ISubmodelElement, error) {
	// If no value data, return a Property with default valueType
	if smeRow.Value == nil {
		return types.NewProperty(types.DataTypeDefXSDString), nil
	}

	var valueRow model.PropertyValueRow
	err := json.Unmarshal(*smeRow.Value, &valueRow)
	if err != nil {
		return nil, err
	}
	valueID, err := getSingleReference(&valueRow.ValueID, &valueRow.ValueIDReferred, refBuilderMap, refMutex)
	if err != nil {
		return nil, err
	}

	// Convert model enum string to SDK enum, default to string if empty
	valueType := types.DataTypeDefXSD(valueRow.ValueType)

	prop := types.NewProperty(valueType)
	if valueRow.Value != "" {
		prop.SetValue(&valueRow.Value)
	}
	if valueID != nil {
		prop.SetValueID(valueID)
	}

	return prop, nil
}

// buildBasicEventElement constructs a BasicEventElement SubmodelElement from the database row,
// parsing the event details and building references for observed and message broker.
func buildBasicEventElement(smeRow model.SubmodelElementRow) (types.ISubmodelElement, error) {
	var valueRow model.BasicEventElementValueRow
	if smeRow.Value == nil {
		return nil, fmt.Errorf("smeRow.Value is nil")
	}
	err := json.Unmarshal(*smeRow.Value, &valueRow)
	if err != nil {
		return nil, err
	}
	var observedRefsJson, messageBrokerRefsJson map[string]any
	err = json.Unmarshal([]byte(valueRow.Observed), &observedRefsJson)
	if err != nil {
		return nil, err
	}
	if valueRow.MessageBroker.Valid {
		err = json.Unmarshal([]byte(valueRow.MessageBroker.String), &messageBrokerRefsJson)
		if err != nil {
			return nil, err
		}
	}

	var observedRefs, messageBrokerRefs types.IReference
	observedRefs, err = jsonization.ReferenceFromJsonable(observedRefsJson)
	if err != nil {
		_, _ = fmt.Printf("[DEBUG] buildBasicEventElement Observed: JSON: %v, Error: %v\n", observedRefsJson, err)
		return nil, err
	}

	if valueRow.MessageBroker.Valid {
		messageBrokerRefs, err = jsonization.ReferenceFromJsonable(messageBrokerRefsJson)
		if err != nil {
			_, _ = fmt.Printf("[DEBUG] buildBasicEventElement MessageBroker: JSON: %v, Error: %v\n", messageBrokerRefsJson, err)
			return nil, err
		}
	}

	// Convert state and direction strings to SDK enums
	state := types.StateOfEvent(valueRow.State)
	direction := types.Direction(valueRow.Direction)
	bee := types.NewBasicEventElement(observedRefs, direction, state)
	if valueRow.MessageTopic != "" {
		bee.SetMessageTopic(&valueRow.MessageTopic)
	}
	if valueRow.LastUpdate != "" {
		bee.SetLastUpdate(&valueRow.LastUpdate)
	}
	if valueRow.MinInterval != "" {
		bee.SetMinInterval(&valueRow.MinInterval)
	}
	if valueRow.MaxInterval != "" {
		bee.SetMaxInterval(&valueRow.MaxInterval)
	}
	if messageBrokerRefs != nil {
		bee.SetMessageBroker(messageBrokerRefs)
	}
	return bee, nil
}

// buildOperation constructs an Operation SubmodelElement from the database row,
// parsing input, output, and inoutput variables.
func buildOperation(smeRow model.SubmodelElementRow) (types.ISubmodelElement, error) {
	var jsonMarshaller = jsoniter.ConfigCompatibleWithStandardLibrary
	var valueRow model.OperationValueRow
	if smeRow.Value == nil {
		return nil, fmt.Errorf("smeRow.Value is nil")
	}
	err := jsonMarshaller.Unmarshal(*smeRow.Value, &valueRow)
	if err != nil {
		return nil, err
	}

	var inputVars, outputVars, inoutputVars []types.IOperationVariable
	if valueRow.InputVariables != nil {
		var inputVarsJsonable []map[string]any
		err = jsonMarshaller.Unmarshal(valueRow.InputVariables, &inputVarsJsonable)
		if err != nil {
			return nil, err
		}
		for i, jsonable := range inputVarsJsonable {
			varOp, err := jsonization.OperationVariableFromJsonable(jsonable)
			if err != nil {
				_, _ = fmt.Printf("[DEBUG] buildOperation InputVariable[%d]: JSON: %v, Error: %v\n", i, jsonable, err)
				return nil, err
			}
			inputVars = append(inputVars, varOp)
		}
	}
	if valueRow.OutputVariables != nil {
		var outputVarsJsonable []map[string]any
		err = jsonMarshaller.Unmarshal(valueRow.OutputVariables, &outputVarsJsonable)
		if err != nil {
			return nil, err
		}
		for i, jsonable := range outputVarsJsonable {
			varOp, err := jsonization.OperationVariableFromJsonable(jsonable)
			if err != nil {
				_, _ = fmt.Printf("[DEBUG] buildOperation OutputVariable[%d]: JSON: %v, Error: %v\n", i, jsonable, err)
				return nil, err
			}
			outputVars = append(outputVars, varOp)
		}
	}
	if valueRow.InoutputVariables != nil {
		var inoutputVarsJsonable []map[string]any
		err = jsonMarshaller.Unmarshal(valueRow.InoutputVariables, &inoutputVarsJsonable)
		if err != nil {
			return nil, err
		}
		for i, jsonable := range inoutputVarsJsonable {
			varOp, err := jsonization.OperationVariableFromJsonable(jsonable)
			if err != nil {
				_, _ = fmt.Printf("[DEBUG] buildOperation InoutputVariable[%d]: JSON: %v, Error: %v\n", i, jsonable, err)
				return nil, err
			}
			inoutputVars = append(inoutputVars, varOp)
		}
	}

	operation := types.NewOperation()

	if len(inputVars) > 0 {
		operation.SetInputVariables(inputVars)
	}
	if len(outputVars) > 0 {
		operation.SetOutputVariables(outputVars)
	}
	if len(inoutputVars) > 0 {
		operation.SetInoutputVariables(inoutputVars)
	}

	_ = inputVars
	_ = outputVars
	_ = inoutputVars
	return operation, nil
}

// getSingleReference parses a single reference from JSON data and builds it using the reference builders.
// Returns the first reference if available, or nil.
func getSingleReference(reference *json.RawMessage, referredReference *json.RawMessage, refBuilderMap map[int64]*ReferenceBuilder, refMutex *sync.RWMutex) (types.IReference, error) {
	var refs []types.IReference
	if reference != nil {
		parsedRefs, err := ParseReferences(*reference, refBuilderMap, refMutex)
		if err != nil {
			return nil, err
		}
		refs = append(refs, parsedRefs...)
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

// buildEntity constructs an Entity SubmodelElement from the database row,
// parsing the entity type, global asset ID, statements, and specific asset IDs.
func buildEntity(smeRow model.SubmodelElementRow) (types.ISubmodelElement, error) {
	var valueRow model.EntityValueRow
	if smeRow.Value == nil {
		return nil, fmt.Errorf("smeRow.Value is nil")
	}
	err := json.Unmarshal(*smeRow.Value, &valueRow)
	if err != nil {
		return nil, err
	}

	entity := types.NewEntity()

	entityType := types.EntityType(valueRow.EntityType)

	var specificAssetIDs []types.ISpecificAssetID
	if valueRow.SpecificAssetIDs != nil {
		var jsonable []map[string]any
		err = json.Unmarshal(valueRow.SpecificAssetIDs, &jsonable)
		if err != nil {
			return nil, err
		}
		for i, j := range jsonable {
			said, err := jsonization.SpecificAssetIDFromJsonable(j)
			if err != nil {
				_, _ = fmt.Printf("[DEBUG] buildEntity SpecificAssetID[%d]: JSON: %v, Error: %v\n", i, j, err)
				return nil, err
			}
			specificAssetIDs = append(specificAssetIDs, said)
		}
		entity.SetSpecificAssetIDs(specificAssetIDs)
	}

	entity.SetEntityType(&entityType)
	if valueRow.GlobalAssetID != "" {
		entity.SetGlobalAssetID(&valueRow.GlobalAssetID)
	}
	// SpecificAssetIDs remain as model types for now - full conversion pending
	return entity, nil
}

// buildAnnotatedRelationshipElement constructs an AnnotatedRelationshipElement SubmodelElement from the database row,
// parsing the first and second references, and the annotations.
func buildAnnotatedRelationshipElement(smeRow model.SubmodelElementRow) (types.ISubmodelElement, error) {
	var valueRow model.AnnotatedRelationshipElementValueRow
	if smeRow.Value == nil {
		return nil, fmt.Errorf("smeRow.Value is nil")
	}
	err := json.Unmarshal(*smeRow.Value, &valueRow)
	if err != nil {
		return nil, err
	}

	var firstJsonable, secondJsonable map[string]any
	if valueRow.First == nil {
		return nil, fmt.Errorf("first reference in RelationshipElement is nil")
	}
	err = json.Unmarshal(valueRow.First, &firstJsonable)
	if err != nil {
		return nil, err
	}
	if valueRow.Second == nil {
		return nil, fmt.Errorf("second reference in RelationshipElement is nil")
	}
	err = json.Unmarshal(valueRow.Second, &secondJsonable)
	if err != nil {
		return nil, err
	}

	firstSDK, err := jsonization.ReferenceFromJsonable(firstJsonable)
	if err != nil {
		_, _ = fmt.Printf("[DEBUG] buildAnnotatedRelationshipElement First: JSON: %v, Error: %v\n", firstJsonable, err)
		return nil, fmt.Errorf("error converting first jsonable to Reference: %w", err)
	}
	secondSDK, err := jsonization.ReferenceFromJsonable(secondJsonable)
	if err != nil {
		_, _ = fmt.Printf("[DEBUG] buildAnnotatedRelationshipElement Second: JSON: %v, Error: %v\n", secondJsonable, err)
		return nil, fmt.Errorf("error converting second jsonable to Reference: %w", err)
	}

	relElem := types.NewAnnotatedRelationshipElement()
	relElem.SetFirst(firstSDK)
	relElem.SetSecond(secondSDK)
	return relElem, nil
}

// buildMultiLanguageProperty creates a new MultiLanguageProperty SubmodelElement.
func buildMultiLanguageProperty(smeRow model.SubmodelElementRow) (types.ISubmodelElement, error) {
	mlp := types.NewMultiLanguageProperty()

	if smeRow.Value == nil {
		return mlp, nil
	}

	var valueRow model.MultiLanguagePropertyElementValueRow

	err := json.Unmarshal(*smeRow.Value, &valueRow)
	if err != nil {
		return nil, err
	}

	if valueRow.Value != nil {
		var valueJsonable []map[string]any
		err = json.Unmarshal(*valueRow.Value, &valueJsonable)
		if err != nil {
			return nil, err
		}
		var textTypes []types.ILangStringTextType
		for _, val := range valueJsonable {
			// Remove internal database 'id' field before SDK parsing
			delete(val, "id")
			valueSDK, err := jsonization.LangStringTextTypeFromJsonable(val)
			if err != nil {
				_, _ = fmt.Printf("[DEBUG] buildMultiLanguageProperty Value: JSON: %v, Error: %v\n", val, err)
				return nil, err
			}
			textTypes = append(textTypes, valueSDK)
		}
		mlp.SetValue(textTypes)
	}

	// Handle ValueID reference if present
	if valueRow.ValueID != nil {
		var valueIDJsonable map[string]any
		err = json.Unmarshal(*valueRow.ValueID, &valueIDJsonable)
		if err != nil {
			return nil, err
		}
		valueIDSDK, err := jsonization.ReferenceFromJsonable(valueIDJsonable)
		if err != nil {
			_, _ = fmt.Printf("[DEBUG] buildMultiLanguageProperty ValueID: JSON: %v, Error: %v\n", valueIDJsonable, err)
			return nil, fmt.Errorf("error converting valueID jsonable to Reference: %w", err)
		}
		mlp.SetValueID(valueIDSDK)
	}

	return mlp, nil
}

// buildFile creates a new File SubmodelElement.
func buildFile(smeRow model.SubmodelElementRow) (types.ISubmodelElement, error) {
	var valueRow model.FileElementValueRow
	if smeRow.Value == nil {
		return nil, fmt.Errorf("smeRow.Value is nil")
	}
	err := json.Unmarshal(*smeRow.Value, &valueRow)
	if err != nil {
		return nil, err
	}
	file := types.NewFile()
	if len(valueRow.ContentType) > 0 {
		file.SetContentType(&valueRow.ContentType)
	}
	if valueRow.Value != "" {
		file.SetValue(&valueRow.Value)
	}
	return file, nil
}

// buildBlob creates a new Blob SubmodelElement.
func buildBlob(smeRow model.SubmodelElementRow) (types.ISubmodelElement, error) {
	var valueRow model.BlobElementValueRow
	if smeRow.Value == nil {
		return nil, fmt.Errorf("smeRow.Value is nil")
	}
	if err := json.Unmarshal(*smeRow.Value, &valueRow); err != nil {
		return nil, err
	}

	// Postgres bytea is commonly returned as: \x<hex>
	raw := strings.TrimSpace(valueRow.Value)
	if raw == "" {
		return nil, fmt.Errorf("blob value is empty")
	}

	var decoded []byte
	var decodedHex []byte
	var err error

	if strings.HasPrefix(raw, `\x`) || strings.HasPrefix(raw, `\\x`) {
		// handle "\x..." and "\\x..." (sometimes JSON escaping causes double slash)
		raw = strings.TrimPrefix(raw, `\\x`)
		raw = strings.TrimPrefix(raw, `\x`)

		decoded, err = hex.DecodeString(raw)
		if err != nil {
			return nil, common.NewInternalServerError("Failed to hex-decode blob value: " + err.Error())
		}
		// as fallback copy
		decodedHex, _ = hex.DecodeString(raw)
	}
	decoded, err = common.Decode(string(decoded))
	if err != nil {
		decoded = decodedHex // Fallback to hex decoded value
		_, _ = fmt.Println("WARNING: Error while decoding Base64 - falling back to HEX Decoded Value as a fallback.")
	}

	blob := types.NewBlob()
	blob.SetContentType(&valueRow.ContentType)
	if string(decoded) != "" {
		blob.SetValue(decoded)
	}
	return blob, nil
}

// buildRange creates a new Range SubmodelElement.
func buildRange(smeRow model.SubmodelElementRow) (types.ISubmodelElement, error) {
	var valueRow model.RangeValueRow
	if smeRow.Value == nil {
		return nil, fmt.Errorf("smeRow.Value is nil")
	}
	err := json.Unmarshal(*smeRow.Value, &valueRow)
	if err != nil {
		return nil, err
	}
	// Convert value type string to SDK enum
	valueType := types.DataTypeDefXSD(valueRow.ValueType)
	rng := types.NewRange(valueType)
	if valueRow.Min != "" {
		rng.SetMin(&valueRow.Min)
	}
	if valueRow.Max != "" {
		rng.SetMax(&valueRow.Max)
	}
	return rng, nil
}

// buildCapability creates a new Capability SubmodelElement.
func buildCapability() (types.ISubmodelElement, error) {
	return types.NewCapability(), nil
}

// buildReferenceElement constructs a ReferenceElement SubmodelElement from the database row,
// parsing the reference value.
func buildReferenceElement(smeRow model.SubmodelElementRow) (types.ISubmodelElement, error) {
	var valueRow model.ReferenceElementValueRow
	if smeRow.Value == nil {
		return nil, fmt.Errorf("smeRow.Value is nil")
	}
	err := json.Unmarshal(*smeRow.Value, &valueRow)
	if err != nil {
		return nil, err
	}

	var refSDK types.IReference
	if valueRow.Value != nil {
		var refJsonable map[string]any
		err = json.Unmarshal(valueRow.Value, &refJsonable)
		if err != nil {
			return nil, err
		}
		// Skip if empty object {} - not a valid Reference
		if len(refJsonable) > 0 {
			refSDK, err = jsonization.ReferenceFromJsonable(refJsonable)
			if err != nil {
				_, _ = fmt.Printf("[DEBUG] buildReferenceElement: JSON: %v, Error: %v\n", refJsonable, err)
				return nil, fmt.Errorf("error converting reference jsonable to Reference: %w", err)
			}
		}
	}

	refElem := types.NewReferenceElement()
	if refSDK != nil {
		refElem.SetValue(refSDK)
	}
	return refElem, nil
}

// buildRelationshipElement constructs a RelationshipElement SubmodelElement from the database row,
// parsing the first and second references.
func buildRelationshipElement(smeRow model.SubmodelElementRow) (types.ISubmodelElement, error) {
	var valueRow model.RelationshipElementValueRow
	if smeRow.Value == nil {
		return nil, fmt.Errorf("smeRow.Value is nil")
	}
	err := json.Unmarshal(*smeRow.Value, &valueRow)
	if err != nil {
		return nil, err
	}

	var firstJsonable, secondJsonable map[string]any
	if valueRow.First == nil {
		return nil, fmt.Errorf("first reference in RelationshipElement is nil")
	}
	err = json.Unmarshal(valueRow.First, &firstJsonable)
	if err != nil {
		return nil, err
	}
	if valueRow.Second == nil {
		return nil, fmt.Errorf("second reference in RelationshipElement is nil")
	}
	err = json.Unmarshal(valueRow.Second, &secondJsonable)
	if err != nil {
		return nil, err
	}

	firstSDK, err := jsonization.ReferenceFromJsonable(firstJsonable)
	if err != nil {
		_, _ = fmt.Printf("[DEBUG] buildRelationshipElement First: JSON: %v, Error: %v\n", firstJsonable, err)
		return nil, fmt.Errorf("error converting first jsonable to Reference: %w", err)
	}
	secondSDK, err := jsonization.ReferenceFromJsonable(secondJsonable)
	if err != nil {
		_, _ = fmt.Printf("[DEBUG] buildRelationshipElement Second: JSON: %v, Error: %v\n", secondJsonable, err)
		return nil, fmt.Errorf("error converting second jsonable to Reference: %w", err)
	}

	relElem := types.NewRelationshipElement()
	relElem.SetFirst(firstSDK)
	relElem.SetSecond(secondSDK)
	return relElem, nil
}

// buildSubmodelElementList constructs a SubmodelElementList SubmodelElement from the database row,
// parsing the value type and type value list elements.
func buildSubmodelElementList(smeRow model.SubmodelElementRow) (types.ISubmodelElement, error) {
	var valueRow model.SubmodelElementListRow
	if smeRow.Value == nil {
		return nil, fmt.Errorf("smeRow.Value is nil")
	}
	err := json.Unmarshal(*smeRow.Value, &valueRow)
	if err != nil {
		return nil, err
	}

	typeValueListElement := types.AASSubmodelElements(valueRow.TypeValueListElement)
	smeList := types.NewSubmodelElementList(typeValueListElement)

	// SemanticIDListElement
	if len(valueRow.SemanticIDListElement) != 0 {
		var jsonable map[string]any
		err = json.Unmarshal(valueRow.SemanticIDListElement, &jsonable)
		if err != nil {
			return nil, err
		}
		// Skip if empty object {} - not a valid Reference
		if len(jsonable) > 0 {
			semIDLe, err := jsonization.ReferenceFromJsonable(jsonable)
			if err != nil {
				_, _ = fmt.Printf("[DEBUG] buildSubmodelElementList SemanticIDListElement: JSON: %v, Error: %v\n", jsonable, err)
				return nil, fmt.Errorf("error converting SemanticIDListElement jsonable to Reference: %w", err)
			}
			smeList.SetSemanticIDListElement(semIDLe)
		}
	}

	// Convert type value list element string to SDK enum
	if valueRow.ValueTypeListElement != nil {
		valueTypeListElement := types.DataTypeDefXSD(*valueRow.ValueTypeListElement)
		smeList.SetValueTypeListElement(&valueTypeListElement)
	}

	// OrderRelevant is already fetched in the main query
	if valueRow.OrderRelevant != nil {
		smeList.SetOrderRelevant(valueRow.OrderRelevant)
	}

	return smeList, nil
}
