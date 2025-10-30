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

// Author: Jannik Fried ( Fraunhofer IESE )
package submodelelements

import (
	"database/sql"
	"errors"

	gen "github.com/eclipse-basyx/basyx-go-components/internal/common/model"
	_ "github.com/lib/pq" // PostgreSQL Treiber
)

type PostgreSQLFileHandler struct {
	db        *sql.DB
	decorated *PostgreSQLSMECrudHandler
}

func NewPostgreSQLFileHandler(db *sql.DB) (*PostgreSQLFileHandler, error) {
	decoratedHandler, err := NewPostgreSQLSMECrudHandler(db)
	if err != nil {
		return nil, err
	}
	return &PostgreSQLFileHandler{db: db, decorated: decoratedHandler}, nil
}

func (p PostgreSQLFileHandler) Create(tx *sql.Tx, submodelId string, submodelElement gen.SubmodelElement) (int, error) {
	file, ok := submodelElement.(*gen.File)
	if !ok {
		return 0, errors.New("submodelElement is not of type File")
	}
	// First, perform base SubmodelElement operations within the transaction
	id, err := p.decorated.Create(tx, submodelId, submodelElement)
	if err != nil {
		return 0, err
	}

	// File-specific database insertion
	_, err = tx.Exec(`INSERT INTO file_element (id, content_type, value) VALUES ($1, $2, $3)`,
		id, file.ContentType, file.Value)
	if err != nil {
		return 0, err
	}

	return id, nil
}

func (p PostgreSQLFileHandler) CreateNested(tx *sql.Tx, submodelId string, parentId int, idShortPath string, submodelElement gen.SubmodelElement, pos int) (int, error) {
	file, ok := submodelElement.(*gen.File)
	if !ok {
		return 0, errors.New("submodelElement is not of type File")
	}

	// Create the nested file with the provided idShortPath using the decorated handler
	id, err := p.decorated.CreateAndPath(tx, submodelId, parentId, idShortPath, submodelElement, pos)
	if err != nil {
		return 0, err
	}

	// File-specific database insertion for nested element
	_, err = tx.Exec(`INSERT INTO file_element (id, content_type, value) VALUES ($1, $2, $3)`,
		id, file.ContentType, file.Value)
	if err != nil {
		return 0, err
	}

	return id, nil
}

func (p PostgreSQLFileHandler) Update(idShortOrPath string, submodelElement gen.SubmodelElement) error {
	if dErr := p.decorated.Update(idShortOrPath, submodelElement); dErr != nil {
		return dErr
	}
	return nil
}
func (p PostgreSQLFileHandler) Delete(idShortOrPath string) error {
	if dErr := p.decorated.Delete(idShortOrPath); dErr != nil {
		return dErr
	}
	return nil
}
