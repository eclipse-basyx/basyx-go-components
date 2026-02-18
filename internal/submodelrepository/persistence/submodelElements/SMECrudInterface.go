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

// Package submodelelements provides interfaces and implementations for CRUD operations on Submodel Elements in a PostgreSQL database.
package submodelelements

import (
	"database/sql"

	"github.com/FriedJannik/aas-go-sdk/types"
	"github.com/doug-martin/goqu/v9"
	gen "github.com/eclipse-basyx/basyx-go-components/internal/common/model"
)

// InsertQueryPart represents the type-specific insert data for a submodel element.
// It contains the target table name and the record to insert.
type InsertQueryPart struct {
	TableName string      // The name of the type-specific table (e.g., "property_element", "blob_element")
	Record    goqu.Record // The record containing column-value pairs for the insert
}

// PostgreSQLSMECrudInterface defines the CRUD operations for Submodel Elements in a PostgreSQL database.
type PostgreSQLSMECrudInterface interface {
	Update(string, string, types.ISubmodelElement, *sql.Tx, bool) error
	UpdateValueOnly(string, string, gen.SubmodelElementValue) error
	Delete(string) error
	// GetInsertQueryPart returns the type-specific insert query part for batch insertion.
	// Parameters:
	//   - tx: Active database transaction (needed for creating references)
	//   - id: The database ID of the base submodel_element record
	//   - element: The submodel element to insert
	// Returns:
	//   - *InsertQueryPart: The table name and record for type-specific insert (nil if no type-specific table)
	//   - error: An error if query part creation fails
	GetInsertQueryPart(tx *sql.Tx, id int, element types.ISubmodelElement) (*InsertQueryPart, error)
}
