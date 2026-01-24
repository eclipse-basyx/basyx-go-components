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

// Package submodelelements provides persistence handlers for various submodel element types.
package submodelelements

import (
	"database/sql"
	"fmt"

	"github.com/FriedJannik/aas-go-sdk/types"
	"github.com/eclipse-basyx/basyx-go-components/internal/common"
	"github.com/eclipse-basyx/basyx-go-components/internal/submodelrepository/logger"
)

// HandlerFactory is a function type that creates a PostgreSQLSMECrudInterface for a given database connection.
type HandlerFactory func(*sql.DB) (PostgreSQLSMECrudInterface, error)

// handlerRegistry maps model type names to their factory functions.
// This centralizes handler creation and eliminates the large switch statement.
var handlerRegistry = map[types.ModelType]HandlerFactory{
	types.ModelTypeAnnotatedRelationshipElement: func(db *sql.DB) (PostgreSQLSMECrudInterface, error) {
		return NewPostgreSQLAnnotatedRelationshipElementHandler(db)
	},
	types.ModelTypeBasicEventElement: func(db *sql.DB) (PostgreSQLSMECrudInterface, error) {
		return NewPostgreSQLBasicEventElementHandler(db)
	},
	types.ModelTypeBlob: func(db *sql.DB) (PostgreSQLSMECrudInterface, error) {
		return NewPostgreSQLBlobHandler(db)
	},
	types.ModelTypeCapability: func(db *sql.DB) (PostgreSQLSMECrudInterface, error) {
		return NewPostgreSQLCapabilityHandler(db)
	},
	types.ModelTypeEntity: func(db *sql.DB) (PostgreSQLSMECrudInterface, error) {
		return NewPostgreSQLEntityHandler(db)
	},
	types.ModelTypeFile: func(db *sql.DB) (PostgreSQLSMECrudInterface, error) {
		return NewPostgreSQLFileHandler(db)
	},
	types.ModelTypeMultiLanguageProperty: func(db *sql.DB) (PostgreSQLSMECrudInterface, error) {
		return NewPostgreSQLMultiLanguagePropertyHandler(db)
	},
	types.ModelTypeOperation: func(db *sql.DB) (PostgreSQLSMECrudInterface, error) {
		return NewPostgreSQLOperationHandler(db)
	},
	types.ModelTypeProperty: func(db *sql.DB) (PostgreSQLSMECrudInterface, error) {
		return NewPostgreSQLPropertyHandler(db)
	},
	types.ModelTypeRange: func(db *sql.DB) (PostgreSQLSMECrudInterface, error) {
		return NewPostgreSQLRangeHandler(db)
	},
	types.ModelTypeReferenceElement: func(db *sql.DB) (PostgreSQLSMECrudInterface, error) {
		return NewPostgreSQLReferenceElementHandler(db)
	},
	types.ModelTypeRelationshipElement: func(db *sql.DB) (PostgreSQLSMECrudInterface, error) {
		return NewPostgreSQLRelationshipElementHandler(db)
	},
	types.ModelTypeSubmodelElementCollection: func(db *sql.DB) (PostgreSQLSMECrudInterface, error) {
		return NewPostgreSQLSubmodelElementCollectionHandler(db)
	},
	types.ModelTypeSubmodelElementList: func(db *sql.DB) (PostgreSQLSMECrudInterface, error) {
		return NewPostgreSQLSubmodelElementListHandler(db)
	},
}

// GetHandlerFromRegistry creates the appropriate handler using the handler registry.
// This provides a cleaner alternative to the large switch statement in GetSMEHandlerByModelType.
//
// Parameters:
//   - modelType: String representation of the submodel element type
//   - db: Database connection to be used by the handler
//
// Returns:
//   - PostgreSQLSMECrudInterface: Type-specific handler implementing CRUD operations
//   - error: An error if the model type is unsupported or handler creation fails
func GetHandlerFromRegistry(modelType types.ModelType, db *sql.DB) (PostgreSQLSMECrudInterface, error) {
	factory, exists := handlerRegistry[modelType]
	if !exists {
		return nil, common.NewErrBadRequest(fmt.Sprintf("unknown model type: %s", modelType))
	}

	handler, err := factory(db)
	if err != nil {
		logger.LogHandlerCreationError(modelType, err)
		return nil, common.NewInternalServerError(fmt.Sprintf("Failed to create %s handler. See console for details.", modelType))
	}
	return handler, nil
}

// RegisterHandler registers a custom handler factory for a model type.
// This allows for extending the registry with custom element types.
//
// Parameters:
//   - modelType: The model type name to register
//   - factory: The factory function that creates handlers for this type
func RegisterHandler(modelType types.ModelType, factory HandlerFactory) {
	handlerRegistry[modelType] = factory
}

// GetSupportedModelTypes returns a list of all model types supported by the registry.
func GetSupportedModelTypes() []types.ModelType {
	types := make([]types.ModelType, 0, len(handlerRegistry))
	for t := range handlerRegistry {
		types = append(types, t)
	}
	return types
}
