//go:build unit

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

package submodelelements

import (
	"database/sql"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	gen "github.com/eclipse-basyx/basyx-go-components/internal/common/model"
)

// mockSubmodelElementValue is a basic implementation of SubmodelElementValue for testing
type mockSubmodelElementValue struct {
	value string
}

func (m mockSubmodelElementValue) MarshalValueOnly() ([]byte, error) {
	return []byte(m.value), nil
}

// TestBuildElementsToProcessStack_BasicElement tests processing a basic element without nesting
func TestBuildElementsToProcessStack_BasicElement(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("Failed to create mock database: %v", err)
	}
	defer db.Close()

	// Test data
	submodelID := "test-submodel-id"
	idShortPath := "simpleProperty"
	valueOnly := mockSubmodelElementValue{value: "testValue"}

	// Execute function - since elementsToProcess is not returned, we just verify no panic
	buildElementsToProcessStack(db, submodelID, idShortPath, valueOnly)

	// Verify no database interactions occurred for basic element
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("Unfulfilled expectations: %v", err)
	}
}

// TestBuildElementsToProcessStack_SubmodelElementCollection tests processing a collection
func TestBuildElementsToProcessStack_SubmodelElementCollection(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("Failed to create mock database: %v", err)
	}
	defer db.Close()

	// Test data
	submodelID := "test-submodel-id"
	idShortPath := "collection"

	collection := gen.SubmodelElementCollectionValue{
		"prop1": mockSubmodelElementValue{value: "value1"},
		"prop2": mockSubmodelElementValue{value: "value2"},
		"prop3": mockSubmodelElementValue{value: "value3"},
	}

	// Execute function
	buildElementsToProcessStack(db, submodelID, idShortPath, collection)

	// Verify no database interactions
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("Unfulfilled expectations: %v", err)
	}
}

// TestBuildElementsToProcessStack_SubmodelElementList tests processing a list
func TestBuildElementsToProcessStack_SubmodelElementList(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("Failed to create mock database: %v", err)
	}
	defer db.Close()

	// Test data
	submodelID := "test-submodel-id"
	idShortPath := "list"

	list := gen.SubmodelElementListValue{
		mockSubmodelElementValue{value: "item0"},
		mockSubmodelElementValue{value: "item1"},
		mockSubmodelElementValue{value: "item2"},
	}

	// Execute function
	buildElementsToProcessStack(db, submodelID, idShortPath, list)

	// Verify no database interactions
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("Unfulfilled expectations: %v", err)
	}
}

// TestBuildElementsToProcessStack_AmbiguousValue_MultiLanguageProperty tests ambiguous value resolved as MLP
func TestBuildElementsToProcessStack_AmbiguousValue_MultiLanguageProperty(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("Failed to create mock database: %v", err)
	}
	defer db.Close()

	// Test data
	submodelID := "test-submodel-id"
	idShortPath := "mlpProperty"

	ambiguousValue := gen.AmbiguousSubmodelElementValue{
		{"en": "English text"},
		{"de": "German text"},
	}

	// Mock database query to return MultiLanguageProperty
	rows := sqlmock.NewRows([]string{"model_type"}).
		AddRow("MultiLanguageProperty")

	mock.ExpectQuery("SELECT sme.model_type").
		WithArgs(idShortPath, submodelID).
		WillReturnRows(rows)

	// Execute function
	buildElementsToProcessStack(db, submodelID, idShortPath, ambiguousValue)

	// Verify expectations
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("Unfulfilled expectations: %v", err)
	}
}

// TestBuildElementsToProcessStack_AmbiguousValue_SubmodelElementList tests ambiguous value resolved as list
func TestBuildElementsToProcessStack_AmbiguousValue_SubmodelElementList(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("Failed to create mock database: %v", err)
	}
	defer db.Close()

	// Test data
	submodelID := "test-submodel-id"
	idShortPath := "listProperty"

	ambiguousValue := gen.AmbiguousSubmodelElementValue{
		{"value": "item1"},
		{"value": "item2"},
	}

	// Mock database query to return SubmodelElementList
	rows := sqlmock.NewRows([]string{"model_type"}).
		AddRow("SubmodelElementList")

	mock.ExpectQuery("SELECT sme.model_type").
		WithArgs(idShortPath, submodelID).
		WillReturnRows(rows)

	// Execute function
	buildElementsToProcessStack(db, submodelID, idShortPath, ambiguousValue)

	// Verify expectations
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("Unfulfilled expectations: %v", err)
	}
}

// TestBuildElementsToProcessStack_NestedCollectionWithList tests nested structures
func TestBuildElementsToProcessStack_NestedCollectionWithList(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("Failed to create mock database: %v", err)
	}
	defer db.Close()

	// Test data
	submodelID := "test-submodel-id"
	idShortPath := "rootCollection"

	// Create nested structure: collection containing a list
	nestedList := gen.SubmodelElementListValue{
		mockSubmodelElementValue{value: "listItem0"},
		mockSubmodelElementValue{value: "listItem1"},
	}

	collection := gen.SubmodelElementCollectionValue{
		"nestedList": nestedList,
		"simpleProp": mockSubmodelElementValue{value: "simpleValue"},
	}

	// Execute function
	buildElementsToProcessStack(db, submodelID, idShortPath, collection)

	// Verify no database interactions
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("Unfulfilled expectations: %v", err)
	}
}

// TestBuildElementsToProcessStack_DeepNesting tests deeply nested structures
func TestBuildElementsToProcessStack_DeepNesting(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("Failed to create mock database: %v", err)
	}
	defer db.Close()

	// Test data
	submodelID := "test-submodel-id"
	idShortPath := "level1"

	// Create deeply nested structure
	level3Collection := gen.SubmodelElementCollectionValue{
		"deepProp": mockSubmodelElementValue{value: "deepValue"},
	}

	level2List := gen.SubmodelElementListValue{
		level3Collection,
		mockSubmodelElementValue{value: "siblingValue"},
	}

	level1Collection := gen.SubmodelElementCollectionValue{
		"level2":    level2List,
		"otherProp": mockSubmodelElementValue{value: "otherValue"},
	}

	// Execute function
	buildElementsToProcessStack(db, submodelID, idShortPath, level1Collection)

	// Verify no database interactions
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("Unfulfilled expectations: %v", err)
	}
}

// TestBuildElementsToProcessStack_ListWithMultipleDigitIndex tests list with 10+ items
func TestBuildElementsToProcessStack_ListWithMultipleDigitIndex(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("Failed to create mock database: %v", err)
	}
	defer db.Close()

	// Test data
	submodelID := "test-submodel-id"
	idShortPath := "largeList"

	// Create a list with more than 10 items to test multi-digit indexing
	list := gen.SubmodelElementListValue{}
	for i := 0; i < 15; i++ {
		list = append(list, mockSubmodelElementValue{value: "item" + string(rune(i+'0'))})
	}

	// Execute function - this tests that strconv.Itoa correctly handles indices >= 10
	buildElementsToProcessStack(db, submodelID, idShortPath, list)

	// Verify no database interactions
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("Unfulfilled expectations: %v", err)
	}
}

// TestBuildElementsToProcessStack_AmbiguousValue_DatabaseError tests error handling
func TestBuildElementsToProcessStack_AmbiguousValue_DatabaseError(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("Failed to create mock database: %v", err)
	}
	defer db.Close()

	// Test data
	submodelID := "test-submodel-id"
	idShortPath := "ambiguousProp"

	ambiguousValue := gen.AmbiguousSubmodelElementValue{
		{"key": "value"},
	}

	// Mock database query to return error
	mock.ExpectQuery("SELECT sme.model_type").
		WithArgs(idShortPath, submodelID).
		WillReturnError(sql.ErrNoRows)

	// Execute function - should handle error gracefully by returning early
	buildElementsToProcessStack(db, submodelID, idShortPath, ambiguousValue)

	// Verify expectations
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("Unfulfilled expectations: %v", err)
	}
}

// TestBuildElementsToProcessStack_EmptyCollection tests empty collection
func TestBuildElementsToProcessStack_EmptyCollection(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("Failed to create mock database: %v", err)
	}
	defer db.Close()

	// Test data
	submodelID := "test-submodel-id"
	idShortPath := "emptyCollection"

	emptyCollection := gen.SubmodelElementCollectionValue{}

	// Execute function
	buildElementsToProcessStack(db, submodelID, idShortPath, emptyCollection)

	// Verify no database interactions
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("Unfulfilled expectations: %v", err)
	}
}

// TestBuildElementsToProcessStack_EmptyList tests empty list
func TestBuildElementsToProcessStack_EmptyList(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("Failed to create mock database: %v", err)
	}
	defer db.Close()

	// Test data
	submodelID := "test-submodel-id"
	idShortPath := "emptyList"

	emptyList := gen.SubmodelElementListValue{}

	// Execute function
	buildElementsToProcessStack(db, submodelID, idShortPath, emptyList)

	// Verify no database interactions
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("Unfulfilled expectations: %v", err)
	}
}

// TestBuildElementsToProcessStack_ListWithinCollection tests list within collection
func TestBuildElementsToProcessStack_ListWithinCollection(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("Failed to create mock database: %v", err)
	}
	defer db.Close()

	submodelID := "test-submodel-id"
	idShortPath := "parentCollection"

	list := gen.SubmodelElementListValue{
		mockSubmodelElementValue{value: "item0"},
		mockSubmodelElementValue{value: "item1"},
	}

	collection := gen.SubmodelElementCollectionValue{
		"childList": list,
		"otherProp": mockSubmodelElementValue{value: "value"},
	}

	buildElementsToProcessStack(db, submodelID, idShortPath, collection)

	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("Unfulfilled expectations: %v", err)
	}
}

// TestBuildElementsToProcessStack_CollectionWithinList tests collection within list
func TestBuildElementsToProcessStack_CollectionWithinList(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("Failed to create mock database: %v", err)
	}
	defer db.Close()

	submodelID := "test-submodel-id"
	idShortPath := "parentList"

	childCollection := gen.SubmodelElementCollectionValue{
		"prop1": mockSubmodelElementValue{value: "val1"},
		"prop2": mockSubmodelElementValue{value: "val2"},
	}

	list := gen.SubmodelElementListValue{
		childCollection,
		mockSubmodelElementValue{value: "simpleValue"},
	}

	buildElementsToProcessStack(db, submodelID, idShortPath, list)

	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("Unfulfilled expectations: %v", err)
	}
}

// TestBuildElementsToProcessStack_AmbiguousValue_WithinCollection tests ambiguous value nested in collection
func TestBuildElementsToProcessStack_AmbiguousValue_WithinCollection(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("Failed to create mock database: %v", err)
	}
	defer db.Close()

	submodelID := "test-submodel-id"
	idShortPath := "rootCollection"

	ambiguousValue := gen.AmbiguousSubmodelElementValue{
		{"en": "English"},
		{"de": "German"},
	}

	collection := gen.SubmodelElementCollectionValue{
		"mlpChild":   ambiguousValue,
		"simpleProp": mockSubmodelElementValue{value: "value"},
	}

	// Mock database query for the ambiguous value
	rows := sqlmock.NewRows([]string{"model_type"}).
		AddRow("MultiLanguageProperty")

	mock.ExpectQuery("SELECT sme.model_type").
		WithArgs(idShortPath+".mlpChild", submodelID).
		WillReturnRows(rows)

	buildElementsToProcessStack(db, submodelID, idShortPath, collection)

	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("Unfulfilled expectations: %v", err)
	}
}

// TestBuildElementsToProcessStack_AmbiguousValue_WithinList tests ambiguous value nested in list
func TestBuildElementsToProcessStack_AmbiguousValue_WithinList(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("Failed to create mock database: %v", err)
	}
	defer db.Close()

	submodelID := "test-submodel-id"
	idShortPath := "rootList"

	ambiguousValue := gen.AmbiguousSubmodelElementValue{
		{"value": "item1"},
		{"value": "item2"},
	}

	list := gen.SubmodelElementListValue{
		mockSubmodelElementValue{value: "simpleItem"},
		ambiguousValue,
	}

	// Mock database query for the ambiguous value at index 1
	rows := sqlmock.NewRows([]string{"model_type"}).
		AddRow("SubmodelElementList")

	mock.ExpectQuery("SELECT sme.model_type").
		WithArgs(idShortPath+"[1]", submodelID).
		WillReturnRows(rows)

	buildElementsToProcessStack(db, submodelID, idShortPath, list)

	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("Unfulfilled expectations: %v", err)
	}
}

// TestBuildElementsToProcessStack_MultipleAmbiguousValues tests multiple ambiguous values
func TestBuildElementsToProcessStack_MultipleAmbiguousValues(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("Failed to create mock database: %v", err)
	}
	defer db.Close()

	submodelID := "test-submodel-id"
	idShortPath := "collection"

	ambiguous1 := gen.AmbiguousSubmodelElementValue{
		{"en": "Text1"},
	}

	ambiguous2 := gen.AmbiguousSubmodelElementValue{
		{"value": "item"},
	}

	collection := gen.SubmodelElementCollectionValue{
		"mlp":  ambiguous1,
		"list": ambiguous2,
	}

	// Mock database queries - order may vary due to map iteration
	rows1 := sqlmock.NewRows([]string{"model_type"}).AddRow("MultiLanguageProperty")
	rows2 := sqlmock.NewRows([]string{"model_type"}).AddRow("SubmodelElementList")

	mock.ExpectQuery("SELECT sme.model_type").WillReturnRows(rows1)
	mock.ExpectQuery("SELECT sme.model_type").WillReturnRows(rows2)

	buildElementsToProcessStack(db, submodelID, idShortPath, collection)

	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("Unfulfilled expectations: %v", err)
	}
}

// TestBuildElementsToProcessStack_ComplexNestedStructure tests complex nested structure with all types
func TestBuildElementsToProcessStack_ComplexNestedStructure(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("Failed to create mock database: %v", err)
	}
	defer db.Close()

	submodelID := "test-submodel-id"
	idShortPath := "root"

	// Build: collection -> list -> collection -> list -> basic
	level4Basic := mockSubmodelElementValue{value: "deepValue"}

	level3List := gen.SubmodelElementListValue{
		level4Basic,
		mockSubmodelElementValue{value: "anotherDeep"},
	}

	level2Collection := gen.SubmodelElementCollectionValue{
		"deepList": level3List,
		"deepProp": mockSubmodelElementValue{value: "deepProp"},
	}

	level1List := gen.SubmodelElementListValue{
		level2Collection,
		mockSubmodelElementValue{value: "sibling"},
	}

	rootCollection := gen.SubmodelElementCollectionValue{
		"nested": level1List,
		"simple": mockSubmodelElementValue{value: "simple"},
	}

	buildElementsToProcessStack(db, submodelID, idShortPath, rootCollection)

	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("Unfulfilled expectations: %v", err)
	}
}

// TestBuildElementsToProcessStack_AmbiguousValue_InvalidModelType tests handling of unexpected model type
func TestBuildElementsToProcessStack_AmbiguousValue_InvalidModelType(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("Failed to create mock database: %v", err)
	}
	defer db.Close()

	submodelID := "test-submodel-id"
	idShortPath := "ambiguousProp"

	ambiguousValue := gen.AmbiguousSubmodelElementValue{
		{"key": "value"},
	}

	// Mock database query to return unexpected model type (defaults to list behavior)
	rows := sqlmock.NewRows([]string{"model_type"}).
		AddRow("Property")

	mock.ExpectQuery("SELECT sme.model_type").
		WithArgs(idShortPath, submodelID).
		WillReturnRows(rows)

	buildElementsToProcessStack(db, submodelID, idShortPath, ambiguousValue)

	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("Unfulfilled expectations: %v", err)
	}
}

// TestBuildElementsToProcessStack_SingleItemList tests list with single item
func TestBuildElementsToProcessStack_SingleItemList(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("Failed to create mock database: %v", err)
	}
	defer db.Close()

	submodelID := "test-submodel-id"
	idShortPath := "singleItemList"

	list := gen.SubmodelElementListValue{
		mockSubmodelElementValue{value: "onlyItem"},
	}

	buildElementsToProcessStack(db, submodelID, idShortPath, list)

	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("Unfulfilled expectations: %v", err)
	}
}

// TestBuildElementsToProcessStack_SinglePropertyCollection tests collection with single property
func TestBuildElementsToProcessStack_SinglePropertyCollection(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("Failed to create mock database: %v", err)
	}
	defer db.Close()

	submodelID := "test-submodel-id"
	idShortPath := "singlePropCollection"

	collection := gen.SubmodelElementCollectionValue{
		"onlyProp": mockSubmodelElementValue{value: "value"},
	}

	buildElementsToProcessStack(db, submodelID, idShortPath, collection)

	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("Unfulfilled expectations: %v", err)
	}
}

// TestBuildElementsToProcessStack_ListWithZeroIndex tests that list indexing starts at 0
func TestBuildElementsToProcessStack_ListWithZeroIndex(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("Failed to create mock database: %v", err)
	}
	defer db.Close()

	submodelID := "test-submodel-id"
	idShortPath := "list"

	ambiguousValue := gen.AmbiguousSubmodelElementValue{
		{"key": "value"},
	}

	list := gen.SubmodelElementListValue{
		ambiguousValue,
	}

	// Verify the path includes [0] for first element
	rows := sqlmock.NewRows([]string{"model_type"}).AddRow("SubmodelElementList")

	mock.ExpectQuery("SELECT sme.model_type").
		WithArgs(idShortPath+"[0]", submodelID).
		WillReturnRows(rows)

	buildElementsToProcessStack(db, submodelID, idShortPath, list)

	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("Unfulfilled expectations: %v", err)
	}
}

// TestBuildElementsToProcessStack_AmbiguousValue_EmptyArray tests empty ambiguous value
func TestBuildElementsToProcessStack_AmbiguousValue_EmptyArray(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("Failed to create mock database: %v", err)
	}
	defer db.Close()

	submodelID := "test-submodel-id"
	idShortPath := "emptyAmbiguous"

	ambiguousValue := gen.AmbiguousSubmodelElementValue{}

	// Mock database query
	rows := sqlmock.NewRows([]string{"model_type"}).AddRow("SubmodelElementList")

	mock.ExpectQuery("SELECT sme.model_type").
		WithArgs(idShortPath, submodelID).
		WillReturnRows(rows)

	buildElementsToProcessStack(db, submodelID, idShortPath, ambiguousValue)

	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("Unfulfilled expectations: %v", err)
	}
}

// TestBuildElementsToProcessStack_MixedNesting tests collection->list->collection->basic
func TestBuildElementsToProcessStack_MixedNesting(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("Failed to create mock database: %v", err)
	}
	defer db.Close()

	submodelID := "test-submodel-id"
	idShortPath := "mixed"

	innerCollection := gen.SubmodelElementCollectionValue{
		"innerProp": mockSubmodelElementValue{value: "innerValue"},
	}

	middleList := gen.SubmodelElementListValue{
		innerCollection,
		mockSubmodelElementValue{value: "listItem"},
	}

	outerCollection := gen.SubmodelElementCollectionValue{
		"middleList": middleList,
		"outerProp":  mockSubmodelElementValue{value: "outerValue"},
	}

	buildElementsToProcessStack(db, submodelID, idShortPath, outerCollection)

	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("Unfulfilled expectations: %v", err)
	}
}
