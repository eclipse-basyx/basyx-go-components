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

package persistence

import (
	"slices"
	"strconv"
	"strings"

	"github.com/FriedJannik/aas-go-sdk/types"
	"github.com/eclipse-basyx/basyx-go-components/internal/common"
	submodelelements "github.com/eclipse-basyx/basyx-go-components/internal/submodelrepository/persistence/submodelElements"
)

const missingIDShortReason = "in the request body has no idShort (AASd-117)"

// requireSubmodelIDShorts and the other require functions enforce AASd-117 in
// every verification mode: the element tree is addressed and stored by idShort
// paths, so an element without idShort outside a SubmodelElementList could not
// be addressed after it is stored.
func requireSubmodelIDShorts(submodel types.ISubmodel, errorCode string) error {
	return missingIDShortError(errorCode, missingIDShortLocationInElements("submodelElements", submodel.SubmodelElements(), false))
}

func requireSubmodelElementIDShorts(element types.ISubmodelElement, isListChild bool, errorCode string) error {
	return missingIDShortError(errorCode, missingIDShortLocation(element, isListChild))
}

func requireChildSubmodelElementIDShorts(element types.ISubmodelElement, errorCode string) error {
	return missingIDShortError(errorCode, missingChildIDShortLocation(element))
}

func requireOwnIDShort(element types.ISubmodelElement, isListChild bool, errorCode string) error {
	if isListChild || hasIDShort(element) {
		return nil
	}
	return missingIDShortError(errorCode, []string{})
}

func insertParentChildPlacement(parentModelType types.ModelType) (isFromList bool, acceptsChildren bool) {
	_, acceptsChildren = submodelelements.ChildElementField(parentModelType)
	return parentModelType == types.ModelTypeSubmodelElementList, acceptsChildren
}

func missingIDShortError(errorCode string, reversedLocation []string) error {
	if reversedLocation == nil {
		return nil
	}
	if len(reversedLocation) == 0 {
		return common.NewErrBadRequest(errorCode + " Submodel element " + missingIDShortReason)
	}
	slices.Reverse(reversedLocation)
	return common.NewErrBadRequest(errorCode + " Submodel element at '" + strings.Join(reversedLocation, ".") + "' " + missingIDShortReason)
}

// missingIDShortLocation returns nil when the element and its descendants have
// the required idShorts. Otherwise it returns the location of the first element
// without one, relative to element and with the innermost segment first, so
// that the location is only built when an idShort is missing.
func missingIDShortLocation(element types.ISubmodelElement, isListChild bool) []string {
	if !isListChild && !hasIDShort(element) {
		return []string{}
	}
	return missingChildIDShortLocation(element)
}

func missingChildIDShortLocation(element types.ISubmodelElement) []string {
	if element == nil {
		return nil
	}
	if operation, isOperation := element.(types.IOperation); isOperation {
		return missingOperationVariableIDShortLocation(operation)
	}
	field, isContainer := submodelelements.ChildElementField(element.ModelType())
	if !isContainer {
		return nil
	}
	return missingIDShortLocationInElements(field, submodelelements.ChildElements(element), element.ModelType() == types.ModelTypeSubmodelElementList)
}

func missingIDShortLocationInElements(field string, elements []types.ISubmodelElement, areListChildren bool) []string {
	for index, element := range elements {
		if location := missingIDShortLocation(element, areListChildren); location != nil {
			return append(location, indexedLocationSegment(field, index))
		}
	}
	return nil
}

func missingOperationVariableIDShortLocation(operation types.IOperation) []string {
	if location := missingIDShortLocationInVariables("inputVariables", operation.InputVariables()); location != nil {
		return location
	}
	if location := missingIDShortLocationInVariables("outputVariables", operation.OutputVariables()); location != nil {
		return location
	}
	return missingIDShortLocationInVariables("inoutputVariables", operation.InoutputVariables())
}

func missingIDShortLocationInVariables(field string, variables []types.IOperationVariable) []string {
	for index, variable := range variables {
		if variable == nil {
			continue
		}
		if location := missingIDShortLocation(variable.Value(), false); location != nil {
			return append(location, indexedLocationSegment(field, index)+".value")
		}
	}
	return nil
}

func indexedLocationSegment(field string, index int) string {
	return field + "[" + strconv.Itoa(index) + "]"
}

func hasIDShort(element types.ISubmodelElement) bool {
	if element == nil {
		return false
	}
	idShort := element.IDShort()
	return idShort != nil && *idShort != ""
}
