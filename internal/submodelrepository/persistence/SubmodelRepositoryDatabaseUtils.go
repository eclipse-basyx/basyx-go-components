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

	"github.com/FriedJannik/aas-go-sdk/jsonization"
	"github.com/FriedJannik/aas-go-sdk/types"
	"github.com/eclipse-basyx/basyx-go-components/internal/common"
	jsoniter "github.com/json-iterator/go"
)

// ElementToProcess represents a submodel element to be processed during iterative creation.
// This struct is used internally by the AddNestedSubmodelElementsIteratively method to manage
// the stack-based processing of nested elements in collections and lists.
type ElementToProcess struct {
	element                   types.ISubmodelElement
	parentID                  int
	currentIDShortPath        string
	isFromSubmodelElementList bool // Indicates if the current element is from a SubmodelElementList
	position                  int  // Position/index within the parent collection or list
}

type result struct {
	sm     []*types.Submodel
	smMap  map[string]*types.Submodel
	cursor string
	err    error
}

type smeJob struct {
	id string
}

type smeResult struct {
	id   string
	smes []types.ISubmodelElement
	err  error
}

func getElementsToProcess(topLevelElement types.ISubmodelElement, parentID int, startPath string) ([]ElementToProcess, error) {
	stack := []ElementToProcess{}

	switch topLevelElement.ModelType() {
	case types.ModelTypeSubmodelElementCollection:
		submodelElementCollection, ok := topLevelElement.(*types.SubmodelElementCollection)
		if !ok {
			return nil, common.NewInternalServerError("SubmodelElement with modelType 'SubmodelElementCollection' is not of type SubmodelElementCollection")
		}
		for index, nestedElement := range submodelElementCollection.Value() {
			var currentPath string
			if startPath == "" {
				currentPath = *submodelElementCollection.IDShort()
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
	case types.ModelTypeSubmodelElementList:
		submodelElementList, ok := topLevelElement.(*types.SubmodelElementList)
		if !ok {
			return nil, common.NewInternalServerError("SubmodelElement with modelType 'SubmodelElementList' is not of type SubmodelElementList")
		}
		// Add nested elements to stack with index-based paths
		for index, nestedElement := range submodelElementList.Value() {
			var idShortPath string
			if startPath == "" {
				idShortPath = *submodelElementList.IDShort() + "[" + strconv.Itoa(index) + "]"
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
	case types.ModelTypeAnnotatedRelationshipElement:
		submodelElementCollection, ok := topLevelElement.(*types.AnnotatedRelationshipElement)
		if !ok {
			return nil, common.NewInternalServerError("AnnotatedRelationshipElement with modelType 'AnnotatedRelationshipElement' is not of type AnnotatedRelationshipElement")
		}
		for index, nestedElement := range submodelElementCollection.Annotations() {
			var currentPath string
			if startPath == "" {
				currentPath = *submodelElementCollection.IDShort()
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
	case types.ModelTypeEntity:
		submodelElementCollection, ok := topLevelElement.(*types.Entity)
		if !ok {
			return nil, common.NewInternalServerError("Entity with modelType 'Entity' is not of type Entity")
		}
		for index, nestedElement := range submodelElementCollection.Statements() {
			var currentPath string
			if startPath == "" {
				currentPath = *submodelElementCollection.IDShort()
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
		idShortPath = *current.element.IDShort()
	} else {
		// If element comes from a SubmodelElementList, use the path as-is (includes [index])
		if current.isFromSubmodelElementList {
			idShortPath = current.currentIDShortPath
		} else {
			// For SubmodelElementCollection, append element's idShort with dot notation
			idShortPath = current.currentIDShortPath + "." + *current.element.IDShort()
		}
	}
	return idShortPath
}

func addNestedElementToStackWithNormalPath(elem types.ISubmodelElement, i int, stack []ElementToProcess, newParentID int, idShortPath string) []ElementToProcess {
	var nestedElement types.ISubmodelElement
	switch elem.ModelType() {
	case types.ModelTypeAnnotatedRelationshipElement:
		annotatedRelElement, ok := elem.(*types.AnnotatedRelationshipElement)
		if !ok {
			return stack
		}
		nestedElement = annotatedRelElement.Annotations()[i]
	case types.ModelTypeEntity:
		entityElement, ok := elem.(*types.Entity)
		if !ok {
			return stack
		}
		nestedElement = entityElement.Statements()[i]
	case types.ModelTypeSubmodelElementCollection:
		submodelElementCollection, ok := elem.(*types.SubmodelElementCollection)
		if !ok {
			return stack
		}
		nestedElement = submodelElementCollection.Value()[i]
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

func addNestedElementToStackWithIndexPath(submodelElementList *types.SubmodelElementList, index int, idShortPath string, stack []ElementToProcess, newParentID int) []ElementToProcess {
	nestedElement := submodelElementList.Value()[index]
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

func getEDSJSONStringFromSubmodel(sm *types.Submodel) (string, error) {
	json := jsoniter.ConfigCompatibleWithStandardLibrary
	edsJSONString := "[]"
	if len(sm.EmbeddedDataSpecifications()) > 0 {
		edsBytes, err := json.Marshal(sm.EmbeddedDataSpecifications())
		if err != nil {
			_, _ = fmt.Println("SMREPO-BLD-EDS-JSON " + err.Error())
			return "", common.NewInternalServerError("Failed to marshal EmbeddedDataSpecifications - no changes applied - see console for details")
		}
		if edsBytes != nil {
			edsJSONString = string(edsBytes)
		}
	}
	return edsJSONString, nil
}

func getExtensionJSONStringFromSubmodel(sm *types.Submodel) (string, error) {
	json := jsoniter.ConfigCompatibleWithStandardLibrary
	extensionJSONString := "[]"
	if len(sm.Extensions()) > 0 {
		extensionBytes, err := json.Marshal(sm.Extensions())
		if err != nil {
			_, _ = fmt.Println("SMREPO-BLD-EXT-JSON " + err.Error())
			return "", common.NewInternalServerError("Failed to marshal Extension - no changes applied - see console for details")
		}
		if extensionBytes != nil {
			extensionJSONString = string(extensionBytes)
		}
	}
	return extensionJSONString, nil
}

func getSupplementalSemanticIDsJSONStringFromSubmodel(sm *types.Submodel) (string, error) {
	json := jsoniter.ConfigCompatibleWithStandardLibrary
	supplementalSemanticIDs := "[]"
	if len(sm.SupplementalSemanticIDs()) > 0 {
		var toJson []map[string]any
		for _, ref := range sm.SupplementalSemanticIDs() {
			jsonObj, err := jsonization.ToJsonable(ref)
			if err != nil {
				return "", common.NewErrBadRequest("Failed to convert Reference to jsonable object - no changes applied")
			}
			toJson = append(toJson, jsonObj)
		}
		supplBytes, err := json.Marshal(toJson)
		if err != nil {
			_, _ = fmt.Println("SMREPO-BLD-SUPPL-JSON " + err.Error())
			return "", common.NewInternalServerError("Failed to marshal SupplementalSemanticIds - no changes applied - see console for details")
		}
		if supplBytes != nil {
			supplementalSemanticIDs = string(supplBytes)
		}
	}
	return supplementalSemanticIDs, nil
}

func getSubmodelElementModelTypeByIDShortPathAndSubmodelID(db *sql.DB, submodelID string, idShortOrPath string) (*types.ModelType, error) {
	var modelType types.ModelType
	err := db.QueryRow(`SELECT model_type FROM submodel_element WHERE submodel_id = $1 AND idshort_path = $2`, submodelID, idShortOrPath).Scan(&modelType)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, common.NewErrNotFound("Submodel element not found")
		}
		return nil, common.NewInternalServerError(fmt.Sprintf("Failed to get ModelType for SubmodelElement %s", idShortOrPath))
	}
	return &modelType, nil
}

func processByModelType(newParentID int, idShortPath string, current ElementToProcess, stack []ElementToProcess) ([]ElementToProcess, error) {
	switch current.element.ModelType() {
	case types.ModelTypeSubmodelElementCollection:
		submodelElementCollection, ok := current.element.(*types.SubmodelElementCollection)
		if !ok {
			return nil, common.NewInternalServerError("SubmodelElement with modelType 'SubmodelElementCollection' is not of type SubmodelElementCollection")
		}
		for i := len(submodelElementCollection.Value()) - 1; i >= 0; i-- {
			stack = addNestedElementToStackWithNormalPath(submodelElementCollection, i, stack, newParentID, idShortPath)
		}
	case types.ModelTypeSubmodelElementList:
		submodelElementList, ok := current.element.(*types.SubmodelElementList)
		if !ok {
			return nil, common.NewInternalServerError("SubmodelElement with modelType 'SubmodelElementList' is not of type SubmodelElementList")
		}
		for index := len(submodelElementList.Value()) - 1; index >= 0; index-- {
			stack = addNestedElementToStackWithIndexPath(submodelElementList, index, idShortPath, stack, newParentID)
		}
	case types.ModelTypeAnnotatedRelationshipElement:
		annotatedRelElement, ok := current.element.(*types.AnnotatedRelationshipElement)
		if !ok {
			return nil, common.NewInternalServerError("SubmodelElement with modelType 'AnnotatedRelationshipElement' is not of type AnnotatedRelationshipElement")
		}
		for i := len(annotatedRelElement.Annotations()) - 1; i >= 0; i-- {
			stack = addNestedElementToStackWithNormalPath(annotatedRelElement, i, stack, newParentID, idShortPath)
		}
	case types.ModelTypeEntity:
		entityElement, ok := current.element.(*types.Entity)
		if !ok {
			return nil, common.NewInternalServerError("SubmodelElement with modelType 'Entity' is not of type Entity")
		}
		for i := len(entityElement.Statements()) - 1; i >= 0; i-- {
			stack = addNestedElementToStackWithNormalPath(entityElement, i, stack, newParentID, idShortPath)
		}
	}
	return stack, nil
}
