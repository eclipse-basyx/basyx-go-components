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
		name        string
		element     types.ISubmodelElement
		isListChild bool
		errorCode   string
		location    string
	}{
		{name: "named property", element: namedTestProperty("Named")},
		{name: "omitted root", element: namelessTestProperty(), errorCode: "SMREPO-TEST-MISSINGIDSHORT", location: "Submodel element in the request body has"},
		{name: "empty root", element: namedTestProperty(""), errorCode: "SMREPO-TEST-MISSINGIDSHORT", location: "Submodel element in the request body has"},
		{name: "list member root", element: namelessTestProperty(), isListChild: true},
		{name: "collection child", element: testCollection("Root", namedTestProperty("Child"), namelessTestProperty()), errorCode: "SMREPO-TEST-MISSINGIDSHORT", location: "'value[1]'"},
		{name: "list members", element: testList("Root", testCollection("", namedTestProperty("Child")))},
		{name: "child of list member", element: testList("Root", testCollection("", namelessTestProperty())), errorCode: "SMREPO-TEST-MISSINGIDSHORT", location: "'value[0].value[0]'"},
		{name: "entity statement", element: testEntity("Root", namelessTestProperty()), errorCode: "SMREPO-TEST-MISSINGIDSHORT", location: "'statements[0]'"},
		{name: "annotation", element: testAnnotatedRelationship("Root", namelessTestProperty()), errorCode: "SMREPO-TEST-MISSINGIDSHORT", location: "'annotations[0]'"},
		{name: "operation variable", element: testOperation("Root", namelessTestProperty()), errorCode: "SMREPO-TEST-MISSINGIDSHORT", location: "'outputVariables[0].value'"},
		{name: "invalid root", element: namedTestProperty("a;b"), errorCode: "SMREPO-TEST-INVALIDIDSHORT", location: "Submodel element in the request body has the invalid idShort 'a;b'"},
		{name: "invalid list member root", element: namedTestProperty("a b"), isListChild: true, errorCode: "SMREPO-TEST-INVALIDIDSHORT", location: "'a b'"},
		{name: "invalid collection child", element: testCollection("Root", namedTestProperty("Child"), namedTestProperty("x/y")), errorCode: "SMREPO-TEST-INVALIDIDSHORT", location: "'value[1]' in the request body has the invalid idShort 'x/y'"},
		{name: "invalid named list member", element: testList("Root", testCollection("1st", namedTestProperty("Child"))), errorCode: "SMREPO-TEST-INVALIDIDSHORT", location: "'value[0]'"},
		{name: "invalid child of list member", element: testList("Root", testCollection("", namedTestProperty("a.b"))), errorCode: "SMREPO-TEST-INVALIDIDSHORT", location: "'value[0].value[0]'"},
		{name: "invalid entity statement", element: testEntity("Root", namedTestProperty("a%3Bb")), errorCode: "SMREPO-TEST-INVALIDIDSHORT", location: "'statements[0]'"},
		{name: "invalid annotation", element: testAnnotatedRelationship("Root", namedTestProperty("x")), errorCode: "SMREPO-TEST-INVALIDIDSHORT", location: "'annotations[0]'"},
		{name: "invalid operation variable", element: testOperation("Root", namedTestProperty("Out-")), errorCode: "SMREPO-TEST-INVALIDIDSHORT", location: "'outputVariables[0].value'"},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()
			err := requireSubmodelElementIDShorts(testCase.element, testCase.isListChild, "SMREPO-TEST")
			if testCase.errorCode == "" {
				require.NoError(t, err)
				return
			}
			require.True(t, common.IsErrBadRequest(err), "%v", err)
			require.ErrorContains(t, err, testCase.errorCode+" ")
			require.ErrorContains(t, err, testCase.location)
		})
	}
}

func TestRequireTargetIDShort(t *testing.T) {
	t.Parallel()
	testCases := []struct {
		name                 string
		idShortPath          string
		element              types.ISubmodelElement
		payloadRenamesTarget bool
		invalidIDShort       string
	}{
		{name: "valid stored idShort", idShortPath: "Coll.Valid", element: namelessTestProperty()},
		{name: "invalid stored idShort", idShortPath: "Coll.a;b", element: namelessTestProperty(), invalidIDShort: "a;b"},
		{name: "invalid stored idShort kept by PATCH", idShortPath: "Coll.a;b", element: namedTestProperty("Valid"), invalidIDShort: "a;b"},
		{name: "encoded stored idShort", idShortPath: "Coll.a%3Bb", element: namelessTestProperty(), invalidIDShort: "a%3Bb"},
		{name: "invalid stored idShort renamed by PUT", idShortPath: "Coll.a;b", element: namedTestProperty("Valid"), payloadRenamesTarget: true},
		{name: "invalid payload idShort", idShortPath: "Coll.Valid", element: namedTestProperty("x"), payloadRenamesTarget: true, invalidIDShort: "x"},
		{name: "invalid payload idShort not renaming", idShortPath: "Coll.Valid", element: namedTestProperty("x"), invalidIDShort: "x"},
		{name: "list member", idShortPath: "Coll.List[0]", element: namelessTestProperty()},
		{name: "invalid parent only", idShortPath: "a;b.Valid", element: namelessTestProperty()},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()
			err := requireTargetIDShort(testCase.idShortPath, testCase.element, testCase.payloadRenamesTarget, "SMREPO-TEST")
			if testCase.invalidIDShort == "" {
				require.NoError(t, err)
				return
			}
			require.True(t, common.IsErrBadRequest(err), "%v", err)
			require.ErrorContains(t, err, "SMREPO-TEST-INVALIDIDSHORT ")
			require.ErrorContains(t, err, "invalid idShort '"+testCase.invalidIDShort+"'")
		})
	}
}

func TestRequireSubmodelIDShorts(t *testing.T) {
	t.Parallel()
	submodel := types.NewSubmodel("urn:test:submodel")
	require.NoError(t, requireSubmodelIDShorts(submodel, "SMREPO-TEST"))

	submodel.SetSubmodelElements([]types.ISubmodelElement{namedTestProperty("Named"), testCollection("", namedTestProperty("Child"))})
	err := requireSubmodelIDShorts(submodel, "SMREPO-TEST")
	require.True(t, common.IsErrBadRequest(err), "%v", err)
	require.ErrorContains(t, err, "'submodelElements[1]'")

	submodel.SetSubmodelElements([]types.ISubmodelElement{namedTestProperty("Named"), testCollection("Coll", namelessTestProperty())})
	require.ErrorContains(t, requireSubmodelIDShorts(submodel, "SMREPO-TEST"), "'submodelElements[1].value[0]'")

	submodel.SetSubmodelElements([]types.ISubmodelElement{namedTestProperty("Named"), testCollection("Coll", namedTestProperty("a b"))})
	require.ErrorContains(t, requireSubmodelIDShorts(submodel, "SMREPO-TEST"), "SMREPO-TEST-INVALIDIDSHORT Submodel element at 'submodelElements[1].value[0]'")

	invalidIDShort := "Sub:model"
	submodel.SetIDShort(&invalidIDShort)
	require.ErrorContains(t, requireSubmodelIDShorts(submodel, "SMREPO-TEST"), "SMREPO-TEST-INVALIDIDSHORT Submodel in the request body has the invalid idShort 'Sub:model'")
}

func TestRequireChildSubmodelElementIDShortsIgnoresRoot(t *testing.T) {
	t.Parallel()
	require.NoError(t, requireChildSubmodelElementIDShorts(testCollection("", namedTestProperty("Child")), "SMREPO-TEST"))
	require.NoError(t, requireChildSubmodelElementIDShorts(testCollection("a;b", namedTestProperty("Child")), "SMREPO-TEST"))
	require.Error(t, requireChildSubmodelElementIDShorts(testCollection("", namelessTestProperty()), "SMREPO-TEST"))
}

func TestRequireOwnIDShortIgnoresChildren(t *testing.T) {
	t.Parallel()
	require.NoError(t, requireOwnIDShort(testCollection("Root", namelessTestProperty()), false, "SMREPO-TEST"))
	require.NoError(t, requireOwnIDShort(namelessTestProperty(), true, "SMREPO-TEST"))
	require.NoError(t, requireOwnIDShort(testCollection("Root", namedTestProperty("a;b")), false, "SMREPO-TEST"))
	require.ErrorContains(t, requireOwnIDShort(namedTestProperty("a;b"), true, "SMREPO-TEST"), "SMREPO-TEST-INVALIDIDSHORT")
	require.ErrorContains(t, requireOwnIDShort(namelessTestProperty(), false, "SMREPO-TEST"), "Submodel element in the request body has no idShort (AASd-117)")
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
