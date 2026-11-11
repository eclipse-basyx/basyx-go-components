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
	"github.com/eclipse-basyx/basyx-go-components/internal/common/model"
	submodelelements "github.com/eclipse-basyx/basyx-go-components/internal/submodelrepository/persistence/submodelElements"
)

const missingIDShortReason = "has no idShort (AASd-117)"

// idShortViolation locates the first element in a request body whose idShort
// is missing (AASd-117) or violates the idShort type of the metamodel. The
// location is relative to the checked element, innermost segment first, so
// that it is only built when a violation is found.
type idShortViolation struct {
	reversedLocation []string
	idShort          string
	isMissing        bool
}

// requireSubmodelIDShorts and the other require functions enforce AASd-117 and
// the idShort type of the metamodel in every verification mode: the element
// tree is addressed and stored by idShort paths, so an element without a valid
// idShort outside a SubmodelElementList could not be addressed after it is
// stored. Error codes are errorScope-MISSINGIDSHORT or errorScope-INVALIDIDSHORT.
func requireSubmodelIDShorts(submodel types.ISubmodel, errorScope string) error {
	if err := common.ValidateOptionalIDShort(errorScope, "Submodel in the request body", submodel.IDShort()); err != nil {
		return err
	}
	return idShortViolationError(errorScope, idShortViolationInElements("submodelElements", submodel.SubmodelElements(), false))
}

func requireSubmodelElementIDShorts(element types.ISubmodelElement, isListChild bool, errorScope string) error {
	return idShortViolationError(errorScope, idShortViolationInTree(element, isListChild))
}

func requireChildSubmodelElementIDShorts(element types.ISubmodelElement, errorScope string) error {
	return idShortViolationError(errorScope, childIDShortViolation(element))
}

func requireOwnIDShort(element types.ISubmodelElement, isListChild bool, errorScope string) error {
	return idShortViolationError(errorScope, ownIDShortViolation(element, isListChild))
}

func insertParentChildPlacement(parentModelType types.ModelType) (isFromList bool, acceptsChildren bool) {
	_, acceptsChildren = submodelelements.ChildElementField(parentModelType)
	return parentModelType == types.ModelTypeSubmodelElementList, acceptsChildren
}

func idShortViolationError(errorScope string, violation *idShortViolation) error {
	if violation == nil {
		return nil
	}
	subject := "Submodel element in the request body"
	if len(violation.reversedLocation) > 0 {
		slices.Reverse(violation.reversedLocation)
		subject = "Submodel element at '" + strings.Join(violation.reversedLocation, ".") + "' in the request body"
	}
	if violation.isMissing {
		return common.NewErrBadRequest(errorScope + "-MISSINGIDSHORT " + subject + " " + missingIDShortReason)
	}
	return common.NewErrInvalidIDShort(errorScope, subject, violation.idShort)
}

func idShortViolationInTree(element types.ISubmodelElement, isListChild bool) *idShortViolation {
	if violation := ownIDShortViolation(element, isListChild); violation != nil {
		return violation
	}
	return childIDShortViolation(element)
}

func ownIDShortViolation(element types.ISubmodelElement, isListChild bool) *idShortViolation {
	if element == nil || element.IDShort() == nil {
		if isListChild {
			return nil
		}
		return &idShortViolation{isMissing: true}
	}
	if idShort := *element.IDShort(); !model.IsValidIDShort(idShort) {
		return &idShortViolation{idShort: idShort}
	}
	return nil
}

func childIDShortViolation(element types.ISubmodelElement) *idShortViolation {
	if element == nil {
		return nil
	}
	if operation, isOperation := element.(types.IOperation); isOperation {
		return operationVariableIDShortViolation(operation)
	}
	field, isContainer := submodelelements.ChildElementField(element.ModelType())
	if !isContainer {
		return nil
	}
	return idShortViolationInElements(field, submodelelements.ChildElements(element), element.ModelType() == types.ModelTypeSubmodelElementList)
}

func idShortViolationInElements(field string, elements []types.ISubmodelElement, areListChildren bool) *idShortViolation {
	for index, element := range elements {
		if violation := idShortViolationInTree(element, areListChildren); violation != nil {
			violation.reversedLocation = append(violation.reversedLocation, indexedLocationSegment(field, index))
			return violation
		}
	}
	return nil
}

func operationVariableIDShortViolation(operation types.IOperation) *idShortViolation {
	if violation := idShortViolationInVariables("inputVariables", operation.InputVariables()); violation != nil {
		return violation
	}
	if violation := idShortViolationInVariables("outputVariables", operation.OutputVariables()); violation != nil {
		return violation
	}
	return idShortViolationInVariables("inoutputVariables", operation.InoutputVariables())
}

func idShortViolationInVariables(field string, variables []types.IOperationVariable) *idShortViolation {
	for index, variable := range variables {
		if variable == nil {
			continue
		}
		if violation := idShortViolationInTree(variable.Value(), false); violation != nil {
			violation.reversedLocation = append(violation.reversedLocation, indexedLocationSegment(field, index)+".value")
			return violation
		}
	}
	return nil
}

func indexedLocationSegment(field string, index int) string {
	return field + "[" + strconv.Itoa(index) + "]"
}
