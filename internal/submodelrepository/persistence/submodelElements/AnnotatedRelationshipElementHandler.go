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

// Package submodelelements provides handlers for different types of submodel elements in the BaSyx framework.
// This package contains PostgreSQL-based persistence implementations for various submodel element types
// including annotated relationship elements.
package submodelelements

import (
	"database/sql"
	"fmt"

	"github.com/FriedJannik/aas-go-sdk/jsonization"
	"github.com/FriedJannik/aas-go-sdk/types"
	"github.com/doug-martin/goqu/v9"
	"github.com/eclipse-basyx/basyx-go-components/internal/common"
	gen "github.com/eclipse-basyx/basyx-go-components/internal/common/model"
	persistenceutils "github.com/eclipse-basyx/basyx-go-components/internal/submodelrepository/persistence/utils"
	jsoniter "github.com/json-iterator/go"
	_ "github.com/lib/pq" // PostgreSQL Treiber
)

// PostgreSQLAnnotatedRelationshipElementHandler provides PostgreSQL-based persistence operations
// for AnnotatedRelationshipElement submodel elements. It implements CRUD operations and handles
// the complex relationships and annotations associated with annotated relationship elements.
type PostgreSQLAnnotatedRelationshipElementHandler struct {
	db        *sql.DB
	decorated *PostgreSQLSMECrudHandler
}

// NewPostgreSQLAnnotatedRelationshipElementHandler creates a new handler for AnnotatedRelationshipElement persistence.
// It initializes the handler with a database connection and sets up the decorated CRUD handler
// for common submodel element operations.
//
// Parameters:
//   - db: PostgreSQL database connection
//
// Returns:
//   - *PostgreSQLAnnotatedRelationshipElementHandler: Configured handler instance
//   - error: Error if handler initialization fails
func NewPostgreSQLAnnotatedRelationshipElementHandler(db *sql.DB) (*PostgreSQLAnnotatedRelationshipElementHandler, error) {
	decoratedHandler, err := NewPostgreSQLSMECrudHandler(db)
	if err != nil {
		return nil, err
	}
	return &PostgreSQLAnnotatedRelationshipElementHandler{db: db, decorated: decoratedHandler}, nil
}

// Update modifies an existing AnnotatedRelationshipElement identified by its idShort or path.
// This method handles both the common submodel element properties and the specific annotated
// relationship data such as the 'first' and 'second' references and annotations.
//
// Parameters:
//   - idShortOrPath: idShort or hierarchical path to the element to update
//   - submodelElement: Updated element data
//   - isPut: true: Replaces the Submodel Element with the Body Data (Deletes non-specified fields); false: Updates only passed request body data, unspecified is ignored
//
// Returns:
//   - error: Error if update fails
func (p PostgreSQLAnnotatedRelationshipElementHandler) Update(submodelID string, idShortOrPath string, submodelElement types.ISubmodelElement, tx *sql.Tx, isPut bool) error {
	are, ok := submodelElement.(*types.AnnotatedRelationshipElement)
	if !ok {
		return common.NewErrBadRequest("submodelElement is not of type AnnotatedRelationshipElement")
	}

	var err error
	cu, localTx, err := persistenceutils.StartTXIfNeeded(tx, err, p.db)
	if err != nil {
		return err
	}
	defer cu(&err)

	err = p.decorated.Update(submodelID, idShortOrPath, submodelElement, localTx, isPut)
	if err != nil {
		return err
	}

	smDbID, err := persistenceutils.GetSubmodelDatabaseID(localTx, submodelID)
	if err != nil {
		_, _ = fmt.Println(err)
		return common.NewInternalServerError("Failed to execute PostgreSQL Query - no changes applied - see console for details.")
	}
	elementID, err := p.decorated.GetDatabaseID(smDbID, idShortOrPath)
	if err != nil {
		return err
	}

	firstRef, err := serializeReference(are.First(), jsoniter.ConfigCompatibleWithStandardLibrary)
	if err != nil {
		return err
	}
	secondRef, err := serializeReference(are.Second(), jsoniter.ConfigCompatibleWithStandardLibrary)
	if err != nil {
		return err
	}

	// Update with goqu
	dialect := goqu.Dialect("postgres")

	updateQuery, updateArgs, err := dialect.Update("annotated_relationship_element").
		Set(goqu.Record{
			"first":  firstRef,
			"second": secondRef,
		}).
		Where(goqu.C("id").Eq(elementID)).
		ToSQL()
	if err != nil {
		return err
	}

	_, err = localTx.Exec(updateQuery, updateArgs...)
	if err != nil {
		return err
	}

	// Handle Annotations field based on isPut flag
	// For PUT: always delete all children (annotations) and recreate from body
	// For PATCH (TODO): only delete and recreate children if annotations are provided
	if isPut {
		// PUT -> Remove all children and then recreate the ones from the body
		// Recreation is done by the SubmodelRepositoryDatabase Update Method
		err = DeleteAllChildren(p.db, submodelID, idShortOrPath, localTx)
		if err != nil {
			return err
		}
	}

	return persistenceutils.CommitTransactionIfNeeded(tx, localTx)
}

// UpdateValueOnly updates only the value of an existing AnnotatedRelationshipElement submodel element identified by its idShort or path.
// It updates the 'first' and 'second' references based on the provided value.
//
// Parameters:
//   - submodelID: The ID of the parent submodel
//   - idShortOrPath: The idShort or path identifying the element to update
//   - valueOnly: The new value to set (must be of type gen.AnnotatedRelationshipElementValue)
//
// Returns:
//   - error: An error if the update operation fails
func (p PostgreSQLAnnotatedRelationshipElementHandler) UpdateValueOnly(submodelID string, idShortOrPath string, valueOnly gen.SubmodelElementValue) error {
	// Start transaction
	tx, err := p.db.Begin()
	if err != nil {
		return err
	}
	defer func() {
		if err != nil {
			_ = tx.Rollback()
		}
	}()

	elems, err := buildElementsToProcessStackValueOnly(p.db, submodelID, idShortOrPath, valueOnly)
	if err != nil {
		return err
	}

	// Update 'first' and 'second' references for AnnotatedRelationshipElement
	if areValue, ok := valueOnly.(gen.AnnotatedRelationshipElementValue); ok {
		dialect := goqu.Dialect("postgres")
		smDbID, err := persistenceutils.GetSubmodelDatabaseID(tx, submodelID)
		if err != nil {
			if err == sql.ErrNoRows {
				return common.NewErrNotFound("submodel not found")
			}
			return err
		}

		// Get the element ID from the database using goqu
		var elementID int
		idQuery, args, err := dialect.From("submodel_element").
			Select("id").
			Where(goqu.Ex{
				"idshort_path": idShortOrPath,
				"submodel_id":  smDbID,
			}).ToSQL()
		if err != nil {
			return err
		}

		err = tx.QueryRow(idQuery, args...).Scan(&elementID)
		if err != nil {
			return err
		}

		// Marshal the references to JSON
		var json = jsoniter.ConfigCompatibleWithStandardLibrary
		var firstRef, secondRef *string

		firstRefJson, err := json.Marshal(areValue.First)
		if err != nil {
			return err
		}
		firstRefStr := string(firstRefJson)
		firstRef = &firstRefStr

		secondRefJson, err := json.Marshal(areValue.Second)
		if err != nil {
			return err
		}
		secondRefStr := string(secondRefJson)
		secondRef = &secondRefStr

		// Update the references in the database using goqu
		updateQuery, updateArgs, err := dialect.Update("annotated_relationship_element").
			Set(goqu.Record{
				"first":  firstRef,
				"second": secondRef,
			}).
			Where(goqu.C("id").Eq(elementID)).
			ToSQL()
		if err != nil {
			return err
		}

		_, err = tx.Exec(updateQuery, updateArgs...)
		if err != nil {
			return err
		}
	}

	err = UpdateNestedElementsValueOnly(p.db, elems, idShortOrPath, submodelID)
	if err != nil {
		return err
	}

	err = tx.Commit()
	return err
}

// Delete removes an AnnotatedRelationshipElement identified by its idShort or path from the database.
// This method delegates the deletion operation to the decorated CRUD handler which handles
// the cascading deletion of all related data and child elements.
//
// Parameters:
//   - idShortOrPath: idShort or hierarchical path to the element to delete
//
// Returns:
//   - error: Error if deletion fails
func (p PostgreSQLAnnotatedRelationshipElementHandler) Delete(idShortOrPath string) error {
	return p.decorated.Delete(idShortOrPath)
}

// GetInsertQueryPart returns the type-specific insert query part for batch insertion of AnnotatedRelationshipElement elements.
// It returns the table name and record for inserting into the annotated_relationship_element table.
//
// Parameters:
//   - tx: Active database transaction (not used for AnnotatedRelationshipElement)
//   - id: The database ID of the base submodel_element record
//   - element: The AnnotatedRelationshipElement element to insert
//
// Returns:
//   - *InsertQueryPart: The table name and record for annotated_relationship_element insert
//   - error: An error if the element is not of type AnnotatedRelationshipElement
func (p PostgreSQLAnnotatedRelationshipElementHandler) GetInsertQueryPart(_ *sql.Tx, id int, element types.ISubmodelElement) (*InsertQueryPart, error) {
	areElem, ok := element.(*types.AnnotatedRelationshipElement)
	if !ok {
		return nil, common.NewErrBadRequest("submodelElement is not of type AnnotatedRelationshipElement")
	}

	var json = jsoniter.ConfigCompatibleWithStandardLibrary

	firstRef, err := serializeReference(areElem.First(), json)
	if err != nil {
		return nil, err
	}

	secondRef, err := serializeReference(areElem.Second(), json)
	if err != nil {
		return nil, err
	}

	return &InsertQueryPart{
		TableName: "annotated_relationship_element",
		Record: goqu.Record{
			"id":     id,
			"first":  firstRef,
			"second": secondRef,
		},
	}, nil
}

func serializeReference(ref types.IReference, json jsoniter.API) (string, error) {
	var firstRef string
	if !isEmptyReference(ref) {
		jsonable, err := jsonization.ToJsonable(ref)
		if err != nil {
			return "", common.NewErrBadRequest("SMREPO-SERREF-JSONABLE Failed to convert reference to jsonable: " + err.Error())
		}
		refBytes, err := json.Marshal(jsonable)
		if err != nil {
			return "", err
		}
		firstRef = string(refBytes)
	}
	return firstRef, nil
}
