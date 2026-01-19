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

// Author: Jannik Fried ( Fraunhofer IESE )

package persistencepostgresql

import (
	"database/sql"
	"fmt"
	"strconv"

	"github.com/eclipse-basyx/basyx-go-components/internal/common"
	gen "github.com/eclipse-basyx/basyx-go-components/internal/common/model"
	jsoniter "github.com/json-iterator/go"
)

// ElementToProcess represents a submodel element to be processed during iterative creation.
// This struct is used internally by the AddNestedSubmodelElementsIteratively method to manage
// the stack-based processing of nested elements in collections and lists.
type ElementToProcess struct {
	element                   gen.SubmodelElement
	parentID                  int
	currentIDShortPath        string
	isFromSubmodelElementList bool // Indicates if the current element is from a SubmodelElementList
	position                  int  // Position/index within the parent collection or list
}

type result struct {
	sm     []*gen.Submodel
	smMap  map[string]*gen.Submodel
	cursor string
	err    error
}

type smeJob struct {
	id string
}

type smeResult struct {
	id   string
	smes []gen.SubmodelElement
	err  error
}

func getElementsToProcess(topLevelElement gen.SubmodelElement, parentID int, startPath string) ([]ElementToProcess, error) {
	stack := []ElementToProcess{}

	switch string(topLevelElement.GetModelType()) {
	case "SubmodelElementCollection":
		submodelElementCollection, ok := topLevelElement.(*gen.SubmodelElementCollection)
		if !ok {
			return nil, common.NewInternalServerError("SubmodelElement with modelType 'SubmodelElementCollection' is not of type SubmodelElementCollection")
		}
		for index, nestedElement := range submodelElementCollection.Value {
			var currentPath string
			if startPath == "" {
				currentPath = submodelElementCollection.IdShort
			} else {
				currentPath = startPath
			}
			stack = append(stack, ElementToProcess{
				element:                   nestedElement,
				parentID:                  parentID,
				currentIDShortPath:        currentPath,
				isFromSubmodelElementList: false,
				position:                  index,
			})
		}
	case "SubmodelElementList":
		submodelElementList, ok := topLevelElement.(*gen.SubmodelElementList)
		if !ok {
			return nil, common.NewInternalServerError("SubmodelElement with modelType 'SubmodelElementList' is not of type SubmodelElementList")
		}
		// Add nested elements to stack with index-based paths
		for index, nestedElement := range submodelElementList.Value {
			var idShortPath string
			if startPath == "" {
				idShortPath = submodelElementList.IdShort + "[" + strconv.Itoa(index) + "]"
			} else {
				idShortPath = startPath + "[" + strconv.Itoa(index) + "]"
			}
			stack = append(stack, ElementToProcess{
				element:                   nestedElement,
				parentID:                  parentID,
				currentIDShortPath:        idShortPath,
				isFromSubmodelElementList: true,
				position:                  index,
			})
		}
	case "AnnotatedRelationshipElement":
		submodelElementCollection, ok := topLevelElement.(*gen.AnnotatedRelationshipElement)
		if !ok {
			return nil, common.NewInternalServerError("AnnotatedRelationshipElement with modelType 'AnnotatedRelationshipElement' is not of type AnnotatedRelationshipElement")
		}
		for index, nestedElement := range submodelElementCollection.Annotations {
			var currentPath string
			if startPath == "" {
				currentPath = submodelElementCollection.IdShort
			} else {
				currentPath = startPath
			}
			stack = append(stack, ElementToProcess{
				element:                   nestedElement,
				parentID:                  parentID,
				currentIDShortPath:        currentPath,
				isFromSubmodelElementList: false,
				position:                  index,
			})
		}
	case "Entity":
		submodelElementCollection, ok := topLevelElement.(*gen.Entity)
		if !ok {
			return nil, common.NewInternalServerError("Entity with modelType 'Entity' is not of type Entity")
		}
		for index, nestedElement := range submodelElementCollection.Statements {
			var currentPath string
			if startPath == "" {
				currentPath = submodelElementCollection.IdShort
			} else {
				currentPath = startPath
			}
			stack = append(stack, ElementToProcess{
				element:                   nestedElement,
				parentID:                  parentID,
				currentIDShortPath:        currentPath,
				isFromSubmodelElementList: false,
				position:                  index,
			})
		}
	}
	return stack, nil
}

func buildCurrentIDShortPath(current ElementToProcess) string {
	var idShortPath string
	if current.currentIDShortPath == "" {
		idShortPath = current.element.GetIdShort()
	} else {
		// If element comes from a SubmodelElementList, use the path as-is (includes [index])
		if current.isFromSubmodelElementList {
			idShortPath = current.currentIDShortPath
		} else {
			// For SubmodelElementCollection, append element's idShort with dot notation
			idShortPath = current.currentIDShortPath + "." + current.element.GetIdShort()
		}
	}
	return idShortPath
}

func addNestedElementToStackWithNormalPath(elem gen.SubmodelElement, i int, stack []ElementToProcess, newParentID int, idShortPath string) []ElementToProcess {
	var nestedElement gen.SubmodelElement
	switch elem.GetModelType() {
	case "AnnotatedRelationshipElement":
		annotatedRelElement, ok := elem.(*gen.AnnotatedRelationshipElement)
		if !ok {
			return stack
		}
		nestedElement = annotatedRelElement.Annotations[i]
	case "Entity":
		entityElement, ok := elem.(*gen.Entity)
		if !ok {
			return stack
		}
		nestedElement = entityElement.Statements[i]
	case "SubmodelElementCollection":
		submodelElementCollection, ok := elem.(*gen.SubmodelElementCollection)
		if !ok {
			return stack
		}
		nestedElement = submodelElementCollection.Value[i]
	default:
		return stack
	}
	stack = append(stack, ElementToProcess{
		element:                   nestedElement,
		parentID:                  newParentID,
		currentIDShortPath:        idShortPath,
		isFromSubmodelElementList: false, // Children of collection are not from list
		position:                  i,
	})
	return stack
}

func addNestedElementToStackWithIndexPath(submodelElementList *gen.SubmodelElementList, index int, idShortPath string, stack []ElementToProcess, newParentID int) []ElementToProcess {
	nestedElement := submodelElementList.Value[index]
	nestedIDShortPath := idShortPath + "[" + strconv.Itoa(index) + "]"
	stack = append(stack, ElementToProcess{
		element:                   nestedElement,
		parentID:                  newParentID,
		currentIDShortPath:        nestedIDShortPath,
		isFromSubmodelElementList: true,  // Children of list are from list
		position:                  index, // For lists, position is the actual index
	})
	return stack
}

func getEDSJSONStringFromSubmodel(sm *gen.Submodel) (string, error) {
	json := jsoniter.ConfigCompatibleWithStandardLibrary
	edsJSONString := "[]"
	if sm.EmbeddedDataSpecifications != nil {
		edsBytes, err := json.Marshal(sm.EmbeddedDataSpecifications)
		if err != nil {
			_, _ = fmt.Println(err)
			return "", common.NewInternalServerError("Failed to marshal EmbeddedDataSpecifications - no changes applied - see console for details")
		}
		if edsBytes != nil {
			edsJSONString = string(edsBytes)
		}
	}
	return edsJSONString, nil
}

func getExtensionJSONStringFromSubmodel(sm *gen.Submodel) (string, error) {
	json := jsoniter.ConfigCompatibleWithStandardLibrary
	extensionJSONString := "[]"
	if sm.Extensions != nil {
		extensionBytes, err := json.Marshal(sm.Extensions)
		if err != nil {
			_, _ = fmt.Println(err)
			return "", common.NewInternalServerError("Failed to marshal Extension - no changes applied - see console for details")
		}
		if extensionBytes != nil {
			extensionJSONString = string(extensionBytes)
		}
	}
	return extensionJSONString, nil
}

func getSupplementalSemanticIDsJSONStringFromSubmodel(sm *gen.Submodel) (string, error) {
	json := jsoniter.ConfigCompatibleWithStandardLibrary
	supplementalSemanticIDs := "[]"
	if sm.SupplementalSemanticIds != nil {
		supplBytes, err := json.Marshal(sm.SupplementalSemanticIds)
		if err != nil {
			_, _ = fmt.Println(err)
			return "", common.NewInternalServerError("Failed to marshal SupplementalSemanticIds - no changes applied - see console for details")
		}
		if supplBytes != nil {
			supplementalSemanticIDs = string(supplBytes)
		}
	}
	return supplementalSemanticIDs, nil
}

func getSubmodelElementModelTypeByIDShortPathAndSubmodelID(db *sql.DB, submodelID string, idShortOrPath string) (string, error) {
	var modelType string
	err := db.QueryRow(`SELECT model_type FROM submodel_element WHERE submodel_id = $1 AND idshort_path = $2`, submodelID, idShortOrPath).Scan(&modelType)
	if err != nil {
		if err == sql.ErrNoRows {
			return "", common.NewErrNotFound("Submodel element not found")
		}
		return "", common.NewInternalServerError(fmt.Sprintf("Failed to get ModelType for SubmodelElement %s", idShortOrPath))
	}
	return modelType, nil
}

func processByModelType(newParentID int, idShortPath string, current ElementToProcess, stack []ElementToProcess) ([]ElementToProcess, error) {
	switch string(current.element.GetModelType()) {
	case "SubmodelElementCollection":
		submodelElementCollection, ok := current.element.(*gen.SubmodelElementCollection)
		if !ok {
			return nil, common.NewInternalServerError("SubmodelElement with modelType 'SubmodelElementCollection' is not of type SubmodelElementCollection")
		}
		for i := len(submodelElementCollection.Value) - 1; i >= 0; i-- {
			stack = addNestedElementToStackWithNormalPath(submodelElementCollection, i, stack, newParentID, idShortPath)
		}
	case "SubmodelElementList":
		submodelElementList, ok := current.element.(*gen.SubmodelElementList)
		if !ok {
			return nil, common.NewInternalServerError("SubmodelElement with modelType 'SubmodelElementList' is not of type SubmodelElementList")
		}
		for index := len(submodelElementList.Value) - 1; index >= 0; index-- {
			stack = addNestedElementToStackWithIndexPath(submodelElementList, index, idShortPath, stack, newParentID)
		}
	case "AnnotatedRelationshipElement":
		annotatedRelElement, ok := current.element.(*gen.AnnotatedRelationshipElement)
		if !ok {
			return nil, common.NewInternalServerError("SubmodelElement with modelType 'AnnotatedRelationshipElement' is not of type AnnotatedRelationshipElement")
		}
		for i := len(annotatedRelElement.Annotations) - 1; i >= 0; i-- {
			stack = addNestedElementToStackWithNormalPath(annotatedRelElement, i, stack, newParentID, idShortPath)
		}
	case "Entity":
		entityElement, ok := current.element.(*gen.Entity)
		if !ok {
			return nil, common.NewInternalServerError("SubmodelElement with modelType 'Entity' is not of type Entity")
		}
		for i := len(entityElement.Statements) - 1; i >= 0; i-- {
			stack = addNestedElementToStackWithNormalPath(entityElement, i, stack, newParentID, idShortPath)
		}
	}
	return stack, nil
}
