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
* MERCHANTABILITY, FITNESS FOR A PARTICULAR PURPOSE AND NONINFRINGEMENT.
* IN NO EVENT SHALL THE AUTHORS OR COPYRIGHT HOLDERS BE LIABLE FOR ANY
* CLAIM, DAMAGES OR OTHER LIABILITY, WHETHER IN AN ACTION OF CONTRACT,
* TORT OR OTHERWISE, ARISING FROM, OUT OF OR IN CONNECTION WITH THE
* SOFTWARE OR THE USE OR OTHER DEALINGS IN THE SOFTWARE.
*
* SPDX-License-Identifier: MIT
******************************************************************************/

package persistence

import (
	"testing"

	"github.com/FriedJannik/aas-go-sdk/types"
	"github.com/eclipse-basyx/basyx-go-components/internal/common"
	"github.com/stretchr/testify/require"
)

func TestRequireSubmodelElementIDShorts(t *testing.T) {
	t.Parallel()
	testCases := []struct {
		name            string
		element         types.ISubmodelElement
		isListChild     bool
		missingLocation string
	}{
		{name: "named property", element: namedTestProperty("Named")},
		{name: "omitted root", element: namelessTestProperty(), missingLocation: "Submodel element in the request body has"},
		{name: "empty root", element: namedTestProperty(""), missingLocation: "Submodel element in the request body has"},
		{name: "list member root", element: namelessTestProperty(), isListChild: true},
		{name: "collection child", element: testCollection("Root", namedTestProperty("Child"), namelessTestProperty()), missingLocation: "'value[1]'"},
		{name: "list members", element: testList("Root", testCollection("", namedTestProperty("Child")))},
		{name: "child of list member", element: testList("Root", testCollection("", namelessTestProperty())), missingLocation: "'value[0].value[0]'"},
		{name: "entity statement", element: testEntity("Root", namelessTestProperty()), missingLocation: "'statements[0]'"},
		{name: "annotation", element: testAnnotatedRelationship("Root", namelessTestProperty()), missingLocation: "'annotations[0]'"},
		{name: "operation variable", element: testOperation("Root", namelessTestProperty()), missingLocation: "'outputVariables[0].value'"},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()
			err := requireSubmodelElementIDShorts(testCase.element, testCase.isListChild, "SMREPO-TEST-MISSINGIDSHORT")
			if testCase.missingLocation == "" {
				require.NoError(t, err)
				return
			}
			require.True(t, common.IsErrBadRequest(err), "%v", err)
			require.ErrorContains(t, err, "SMREPO-TEST-MISSINGIDSHORT")
			require.ErrorContains(t, err, testCase.missingLocation)
			require.ErrorContains(t, err, "AASd-117")
		})
	}
}

func TestRequireSubmodelIDShorts(t *testing.T) {
	t.Parallel()
	submodel := types.NewSubmodel("urn:test:submodel")
	require.NoError(t, requireSubmodelIDShorts(submodel, "SMREPO-TEST-MISSINGIDSHORT"))

	submodel.SetSubmodelElements([]types.ISubmodelElement{namedTestProperty("Named"), testCollection("", namedTestProperty("Child"))})
	err := requireSubmodelIDShorts(submodel, "SMREPO-TEST-MISSINGIDSHORT")
	require.True(t, common.IsErrBadRequest(err), "%v", err)
	require.ErrorContains(t, err, "'submodelElements[1]'")

	submodel.SetSubmodelElements([]types.ISubmodelElement{namedTestProperty("Named"), testCollection("Coll", namelessTestProperty())})
	require.ErrorContains(t, requireSubmodelIDShorts(submodel, "SMREPO-TEST-MISSINGIDSHORT"), "'submodelElements[1].value[0]'")
}

func TestRequireChildSubmodelElementIDShortsIgnoresRoot(t *testing.T) {
	t.Parallel()
	require.NoError(t, requireChildSubmodelElementIDShorts(testCollection("", namedTestProperty("Child")), "SMREPO-TEST-MISSINGIDSHORT"))
	require.Error(t, requireChildSubmodelElementIDShorts(testCollection("", namelessTestProperty()), "SMREPO-TEST-MISSINGIDSHORT"))
}

func TestRequireOwnIDShortIgnoresChildren(t *testing.T) {
	t.Parallel()
	require.NoError(t, requireOwnIDShort(testCollection("Root", namelessTestProperty()), false, "SMREPO-TEST-MISSINGIDSHORT"))
	require.NoError(t, requireOwnIDShort(namelessTestProperty(), true, "SMREPO-TEST-MISSINGIDSHORT"))
	require.ErrorContains(t, requireOwnIDShort(namelessTestProperty(), false, "SMREPO-TEST-MISSINGIDSHORT"), "Submodel element in the request body has no idShort (AASd-117)")
}

func TestInsertParentChildPlacement(t *testing.T) {
	t.Parallel()
	testCases := []struct {
		parentModelType types.ModelType
		isFromList      bool
		acceptsChildren bool
	}{
		{parentModelType: types.ModelTypeSubmodelElementCollection, acceptsChildren: true},
		{parentModelType: types.ModelTypeEntity, acceptsChildren: true},
		{parentModelType: types.ModelTypeAnnotatedRelationshipElement, acceptsChildren: true},
		{parentModelType: types.ModelTypeSubmodelElementList, isFromList: true, acceptsChildren: true},
		{parentModelType: types.ModelTypeProperty},
		{parentModelType: types.ModelTypeOperation},
	}

	for _, testCase := range testCases {
		isFromList, acceptsChildren := insertParentChildPlacement(testCase.parentModelType)
		require.Equal(t, testCase.isFromList, isFromList, "%v", testCase.parentModelType)
		require.Equal(t, testCase.acceptsChildren, acceptsChildren, "%v", testCase.parentModelType)
	}
}

func namelessTestProperty() *types.Property {
	return types.NewProperty(types.DataTypeDefXSDString)
}

func namedTestProperty(idShort string) *types.Property {
	property := namelessTestProperty()
	property.SetIDShort(&idShort)
	return property
}

// setTestIDShort leaves the idShort omitted when idShort is empty.
func setTestIDShort(element types.ISubmodelElement, idShort string) types.ISubmodelElement {
	if idShort != "" {
		element.SetIDShort(&idShort)
	}
	return element
}

func testCollection(idShort string, children ...types.ISubmodelElement) types.ISubmodelElement {
	collection := types.NewSubmodelElementCollection()
	collection.SetValue(children)
	return setTestIDShort(collection, idShort)
}

func testList(idShort string, members ...types.ISubmodelElement) types.ISubmodelElement {
	list := types.NewSubmodelElementList(types.AASSubmodelElementsSubmodelElementCollection)
	list.SetValue(members)
	return setTestIDShort(list, idShort)
}

func testEntity(idShort string, statements ...types.ISubmodelElement) types.ISubmodelElement {
	entity := types.NewEntity()
	entity.SetStatements(statements)
	return setTestIDShort(entity, idShort)
}

func testAnnotatedRelationship(idShort string, annotations ...types.IDataElement) types.ISubmodelElement {
	reference := types.NewReference(types.ReferenceTypesModelReference, []types.IKey{types.NewKey(types.KeyTypesSubmodel, "urn:test:submodel")})
	relationship := types.NewAnnotatedRelationshipElement()
	relationship.SetFirst(reference)
	relationship.SetSecond(reference)
	relationship.SetAnnotations(annotations)
	return setTestIDShort(relationship, idShort)
}

func testOperation(idShort string, outputValue types.ISubmodelElement) types.ISubmodelElement {
	operation := types.NewOperation()
	operation.SetInputVariables([]types.IOperationVariable{types.NewOperationVariable(namedTestProperty("Input"))})
	operation.SetOutputVariables([]types.IOperationVariable{types.NewOperationVariable(outputValue)})
	return setTestIDShort(operation, idShort)
}
