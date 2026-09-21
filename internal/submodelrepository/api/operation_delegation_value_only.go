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

package api

import (
	"encoding/json"
	"errors"
	"fmt"
	"sort"

	"github.com/FriedJannik/aas-go-sdk/jsonization"
	"github.com/FriedJannik/aas-go-sdk/types"
	"github.com/eclipse-basyx/basyx-go-components/internal/common"
	gen "github.com/eclipse-basyx/basyx-go-components/internal/common/model"
)

func operationRequestFromValueOnly(operationElement types.ISubmodelElement, request gen.OperationRequestValueOnly) (gen.OperationRequest, error) {
	operation, ok := operationElement.(types.IOperation)
	if !ok {
		return gen.OperationRequest{}, errors.New("SMREPO-OPVALREQ-NOTOPERATION declared element is not an Operation")
	}

	inputArguments, err := operationVariablesFromValueOnly(operation.InputVariables(), request.InputArguments, "inputArguments")
	if err != nil {
		return gen.OperationRequest{}, err
	}
	inoutputArguments, err := operationVariablesFromValueOnly(operation.InoutputVariables(), request.InoutputArguments, "inoutputArguments")
	if err != nil {
		return gen.OperationRequest{}, err
	}

	return gen.OperationRequest{
		InputArguments:        inputArguments,
		InoutputArguments:     inoutputArguments,
		ClientTimeoutDuration: request.ClientTimeoutDuration,
	}, nil
}

func operationVariablesFromValueOnly(declarations []types.IOperationVariable, supplied map[string]any, group string) ([]types.IOperationVariable, error) {
	if len(supplied) == 0 {
		return nil, nil
	}

	declarationByName := make(map[string]types.IOperationVariable, len(declarations))
	for index, declaration := range declarations {
		name, err := operationVariableName(declaration, group, index)
		if err != nil {
			return nil, err
		}
		if _, duplicate := declarationByName[name]; duplicate {
			return nil, fmt.Errorf("SMREPO-OPVALREQ-AMBIGUOUS-%s operation declares idShort %q more than once", group, name)
		}
		declarationByName[name] = declaration
	}

	for _, name := range sortedKeys(supplied) {
		if _, known := declarationByName[name]; !known {
			return nil, fmt.Errorf("SMREPO-OPVALREQ-UNKNOWN-%s operation does not declare idShort %q", group, name)
		}
	}

	result := make([]types.IOperationVariable, 0, len(supplied))
	for _, declaration := range declarations {
		name := *declaration.Value().IDShort()
		value, present := supplied[name]
		if !present {
			continue
		}
		operationVariable, err := operationVariableWithValueOnly(declaration, value)
		if err != nil {
			return nil, fmt.Errorf("SMREPO-OPVALREQ-APPLY-%s-%s %w", group, name, err)
		}
		result = append(result, operationVariable)
	}
	return result, nil
}

func operationVariableName(variable types.IOperationVariable, group string, index int) (string, error) {
	if variable == nil || variable.Value() == nil {
		return "", fmt.Errorf("SMREPO-OPVALREQ-NILDECL-%s-%d operation variable declaration is incomplete", group, index)
	}
	idShort := variable.Value().IDShort()
	if idShort == nil || *idShort == "" {
		return "", fmt.Errorf("SMREPO-OPVALREQ-NAMELESS-%s-%d operation variable declaration has no idShort", group, index)
	}
	return *idShort, nil
}

func sortedKeys(values map[string]any) []string {
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}

func operationVariableWithValueOnly(declaration types.IOperationVariable, supplied any) (types.IOperationVariable, error) {
	jsonable, err := jsonization.ToJsonable(declaration.Value())
	if err != nil {
		return nil, fmt.Errorf("SMREPO-OPVALREQ-SERIALIZEDECL %w", err)
	}
	elementJSON := jsonable

	normalizedValue, err := normalizeValueOnlyInput(supplied)
	if err != nil {
		return nil, err
	}
	if err := applyValueOnlyToElementJSON(elementJSON, normalizedValue); err != nil {
		return nil, err
	}

	element, err := jsonization.SubmodelElementFromJsonable(elementJSON)
	if err != nil {
		return nil, fmt.Errorf("SMREPO-OPVALREQ-PARSEELEMENT %w", err)
	}
	return types.NewOperationVariable(element), nil
}

func normalizeValueOnlyInput(value any) (any, error) {
	serialized, err := json.Marshal(value)
	if err != nil {
		return nil, fmt.Errorf("SMREPO-OPVALREQ-MARSHALVALUE %w", err)
	}
	var normalized any
	if err := common.DecodeJSONPreservingNumbers(serialized, &normalized); err != nil {
		return nil, fmt.Errorf("SMREPO-OPVALREQ-UNMARSHALVALUE %w", err)
	}
	return normalized, nil
}

func applyValueOnlyToElementJSON(element map[string]any, value any) error {
	modelType, ok := element["modelType"].(string)
	if !ok || modelType == "" {
		return errors.New("SMREPO-OPVALREQ-MISSINGMODELTYPE operation variable declaration has no modelType")
	}

	switch modelType {
	case "Property":
		propertyValue, err := operationScalarInput(element, value)
		if err != nil {
			return err
		}
		element["value"] = propertyValue
		return nil
	case "MultiLanguageProperty":
		return applyMultiLanguagePropertyValue(element, value)
	case "Range":
		return applyOperationRangeValue(element, value)
	case "File", "Blob":
		return applyObjectFields(element, value, []string{"contentType", "value"}, nil, modelType)
	case "ReferenceElement":
		valueObject, err := valueOnlyObject(value, modelType)
		if err != nil {
			return err
		}
		element["value"] = valueObject
		return nil
	case "RelationshipElement":
		return applyObjectFields(element, value, []string{"first", "second"}, []string{"first", "second"}, modelType)
	case "AnnotatedRelationshipElement":
		return applyAnnotatedRelationshipValue(element, value)
	case "Entity":
		return applyEntityValue(element, value)
	case "BasicEventElement":
		return applyObjectFields(element, value, []string{"observed"}, []string{"observed"}, modelType)
	case "SubmodelElementCollection":
		valueObject, err := valueOnlyObject(value, modelType)
		if err != nil {
			return err
		}
		return applyNamedElementValues(element, "value", valueObject, modelType)
	case "SubmodelElementList":
		return applyListElementValues(element, value)
	default:
		return fmt.Errorf("SMREPO-OPVALREQ-UNSUPPORTEDTYPE value-only invocation does not support %s", modelType)
	}
}

func applyMultiLanguagePropertyValue(element map[string]any, value any) error {
	items, ok := value.([]any)
	if !ok {
		return fmt.Errorf("SMREPO-OPVALREQ-MLPSHAPE expected array, got %T", value)
	}
	fullValues := make([]any, 0, len(items))
	for index, item := range items {
		languageValue, err := valueOnlyObject(item, "MultiLanguageProperty")
		if err != nil {
			return err
		}
		if len(languageValue) != 1 {
			return fmt.Errorf("SMREPO-OPVALREQ-MLPITEM-%d expected one language entry", index)
		}
		for language, rawText := range languageValue {
			text, ok := rawText.(string)
			if !ok {
				return fmt.Errorf("SMREPO-OPVALREQ-MLPTEXT-%d expected string, got %T", index, rawText)
			}
			fullValues = append(fullValues, map[string]any{"language": language, "text": text})
		}
	}
	element["value"] = fullValues
	return nil
}

func applyObjectFields(element map[string]any, value any, allowed []string, required []string, modelType string) error {
	valueObject, err := valueOnlyObject(value, modelType)
	if err != nil {
		return err
	}
	if err := validateObjectFields(valueObject, allowed, required, modelType); err != nil {
		return err
	}
	for field, fieldValue := range valueObject {
		element[field] = fieldValue
	}
	return nil
}

func validateObjectFields(valueObject map[string]any, allowed []string, required []string, modelType string) error {
	allowedFields := make(map[string]struct{}, len(allowed))
	for _, field := range allowed {
		allowedFields[field] = struct{}{}
	}
	for field := range valueObject {
		if _, ok := allowedFields[field]; !ok {
			return fmt.Errorf("SMREPO-OPVALREQ-UNKNOWNFIELD-%s unexpected field %q", modelType, field)
		}
	}
	for _, field := range required {
		if _, present := valueObject[field]; !present {
			return fmt.Errorf("SMREPO-OPVALREQ-MISSINGFIELD-%s required field %q is missing", modelType, field)
		}
	}
	return nil
}

func valueOnlyObject(value any, modelType string) (map[string]any, error) {
	valueObject, ok := value.(map[string]any)
	if !ok {
		return nil, fmt.Errorf("SMREPO-OPVALREQ-OBJECTSHAPE-%s expected object, got %T", modelType, value)
	}
	return valueObject, nil
}

func applyAnnotatedRelationshipValue(element map[string]any, value any) error {
	valueObject, err := valueOnlyObject(value, "AnnotatedRelationshipElement")
	if err != nil {
		return err
	}
	if err := validateObjectFields(valueObject, []string{"first", "second", "annotations"}, []string{"first", "second"}, "AnnotatedRelationshipElement"); err != nil {
		return err
	}
	element["first"] = valueObject["first"]
	element["second"] = valueObject["second"]
	annotations, present := valueObject["annotations"]
	if !present {
		return nil
	}
	annotationValues, err := valueOnlyObject(annotations, "AnnotatedRelationshipElement.annotations")
	if err != nil {
		return err
	}
	return applyNamedElementValues(element, "annotations", annotationValues, "AnnotatedRelationshipElement.annotations")
}

func applyEntityValue(element map[string]any, value any) error {
	valueObject, err := valueOnlyObject(value, "Entity")
	if err != nil {
		return err
	}
	if err := validateObjectFields(valueObject, []string{"entityType", "globalAssetId", "specificAssetIds", "statements"}, nil, "Entity"); err != nil {
		return err
	}
	for _, field := range []string{"entityType", "globalAssetId", "specificAssetIds"} {
		if fieldValue, present := valueObject[field]; present {
			element[field] = fieldValue
		}
	}
	statements, present := valueObject["statements"]
	if !present {
		return nil
	}
	statementValues, err := valueOnlyObject(statements, "Entity.statements")
	if err != nil {
		return err
	}
	return applyNamedElementValues(element, "statements", statementValues, "Entity.statements")
}

func applyNamedElementValues(element map[string]any, field string, supplied map[string]any, context string) error {
	declaredItems, err := operationDeclaredChildren(element, field)
	if err != nil {
		return err
	}
	declaredByName := make(map[string]map[string]any, len(declaredItems))
	for index, item := range declaredItems {
		child, ok := item.(map[string]any)
		if !ok {
			return fmt.Errorf("SMREPO-OPVALREQ-INVALIDCHILD-%s-%d declaration child is not an object", context, index)
		}
		name, ok := child["idShort"].(string)
		if !ok || name == "" {
			return fmt.Errorf("SMREPO-OPVALREQ-NAMELESSCHILD-%s-%d declaration child has no idShort", context, index)
		}
		if _, duplicate := declaredByName[name]; duplicate {
			return fmt.Errorf("SMREPO-OPVALREQ-AMBIGUOUSCHILD-%s child idShort %q is declared more than once", context, name)
		}
		declaredByName[name] = child
	}
	for _, name := range sortedKeys(supplied) {
		child, known := declaredByName[name]
		if !known {
			return fmt.Errorf("SMREPO-OPVALREQ-UNKNOWNCHILD-%s child idShort %q is not declared", context, name)
		}
		if err := applyValueOnlyToElementJSON(child, supplied[name]); err != nil {
			return fmt.Errorf("SMREPO-OPVALREQ-APPLYCHILD-%s-%s %w", context, name, err)
		}
	}
	return nil
}

func applyListElementValues(element map[string]any, value any) error {
	values, ok := value.([]any)
	if !ok {
		return fmt.Errorf("SMREPO-OPVALREQ-LISTSHAPE expected array, got %T", value)
	}
	declaredItems, err := operationDeclaredChildren(element, "value")
	if err != nil {
		return err
	}
	if len(values) != len(declaredItems) {
		return fmt.Errorf("SMREPO-OPVALREQ-LISTLENGTH expected %d values, got %d", len(declaredItems), len(values))
	}
	for index, valueItem := range values {
		declaredItem, ok := declaredItems[index].(map[string]any)
		if !ok {
			return fmt.Errorf("SMREPO-OPVALREQ-INVALIDLISTITEM-%d declaration item is not an object", index)
		}
		if err := applyValueOnlyToElementJSON(declaredItem, valueItem); err != nil {
			return fmt.Errorf("SMREPO-OPVALREQ-APPLYLISTITEM-%d %w", index, err)
		}
	}
	return nil
}

func operationResultToValueOnly(payload any) (map[string]any, error) {
	canonical, err := toDelegatedOperationResultPayloadFromBody(payload)
	if err != nil {
		return nil, err
	}
	result := make(map[string]any, len(canonical))
	for key, value := range canonical {
		result[key] = value
	}
	for _, group := range []string{"outputArguments", "inoutputArguments"} {
		rawArguments, present := canonical[group]
		if !present {
			continue
		}
		arguments, err := operationResultArgumentsToValueOnly(rawArguments, group)
		if err != nil {
			return nil, err
		}
		result[group] = arguments
	}
	return result, nil
}

func operationResultArgumentsToValueOnly(payload any, group string) (map[string]any, error) {
	jsonableArguments, err := jsonableOperationVariablesFromArray(payload)
	if err != nil {
		return nil, fmt.Errorf("SMREPO-OPVALRES-PARSEGROUP-%s %w", group, err)
	}
	result := make(map[string]any, len(jsonableArguments))
	for index, jsonable := range jsonableArguments {
		operationVariable, err := jsonization.OperationVariableFromJsonable(jsonable)
		if err != nil {
			return nil, fmt.Errorf("SMREPO-OPVALRES-PARSEARG-%s-%d %w", group, index, err)
		}
		name, err := operationVariableName(operationVariable, group, index)
		if err != nil {
			return nil, err
		}
		if _, duplicate := result[name]; duplicate {
			return nil, fmt.Errorf("SMREPO-OPVALRES-DUPLICATE-%s argument idShort %q occurs more than once", group, name)
		}
		valueOnly, err := operationElementToValueOnly(operationVariable.Value())
		if err != nil {
			return nil, fmt.Errorf("SMREPO-OPVALRES-CONVERT-%s-%s %w", group, name, err)
		}
		if valueOnly == nil {
			return nil, fmt.Errorf("SMREPO-OPVALRES-UNREPRESENTABLE-%s-%s argument cannot be represented as value-only", group, name)
		}
		result[name] = valueOnly
	}
	return result, nil
}

func operationDeclaredChildren(element map[string]any, field string) ([]any, error) {
	raw, present := element[field]
	if !present {
		return nil, nil
	}
	children, ok := raw.([]any)
	if !ok {
		return nil, fmt.Errorf("SMREPO-OPVALREQ-INVALIDCHILDREN declaration field %q is not an array", field)
	}
	return children, nil
}
