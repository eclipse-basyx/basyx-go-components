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

type PostgreSQLMultiLanguagePropertyHandler struct {
	db        *sql.DB
	decorated *PostgreSQLSMECrudHandler
}

func NewPostgreSQLMultiLanguagePropertyHandler(db *sql.DB) (*PostgreSQLMultiLanguagePropertyHandler, error) {
	decoratedHandler, err := NewPostgreSQLSMECrudHandler(db)
	if err != nil {
		return nil, err
	}
	return &PostgreSQLMultiLanguagePropertyHandler{db: db, decorated: decoratedHandler}, nil
}

func (p PostgreSQLMultiLanguagePropertyHandler) Create(tx *sql.Tx, submodelId string, submodelElement gen.SubmodelElement) (int, error) {
	mlp, ok := submodelElement.(*gen.MultiLanguageProperty)
	if !ok {
		return 0, errors.New("submodelElement is not of type MultiLanguageProperty")
	}
	// First, perform base SubmodelElement operations within the transaction
	id, err := p.decorated.Create(tx, submodelId, submodelElement)
	if err != nil {
		return 0, err
	}

	// MultiLanguageProperty-specific database insertion
	err = insertMultiLanguageProperty(mlp, tx, id)
	if err != nil {
		return 0, err
	}

	return id, nil
}

func (p PostgreSQLMultiLanguagePropertyHandler) CreateNested(tx *sql.Tx, submodelId string, parentId int, idShortPath string, submodelElement gen.SubmodelElement, pos int) (int, error) {
	mlp, ok := submodelElement.(*gen.MultiLanguageProperty)
	if !ok {
		return 0, errors.New("submodelElement is not of type MultiLanguageProperty")
	}

	// Create the nested mlp with the provided idShortPath using the decorated handler
	id, err := p.decorated.CreateAndPath(tx, submodelId, parentId, idShortPath, submodelElement, pos)
	if err != nil {
		return 0, err
	}

	// MultiLanguageProperty-specific database insertion for nested element
	err = insertMultiLanguageProperty(mlp, tx, id)
	if err != nil {
		return 0, err
	}

	return id, nil
}

func (p PostgreSQLMultiLanguagePropertyHandler) Read(tx *sql.Tx, submodelId string, idShortOrPath string) (gen.SubmodelElement, error) {
	var sme gen.SubmodelElement = &gen.MultiLanguageProperty{}
	id, err := p.decorated.Read(tx, submodelId, idShortOrPath, &sme)
	if err != nil {
		return nil, err
	}

	// Read values
	rows, err := tx.Query(`SELECT language, text FROM multilanguage_property_value WHERE mlp_id = $1`, id)
	if err != nil {
		return sme, nil
	}
	defer rows.Close()

	var values []gen.LangStringTextType
	for rows.Next() {
		var lang, text string
		if err := rows.Scan(&lang, &text); err != nil {
			return nil, err
		}
		values = append(values, gen.LangStringTextType{Language: lang, Text: text})
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	mlp := sme.(*gen.MultiLanguageProperty)
	mlp.Value = values
	return sme, nil
}
func (p PostgreSQLMultiLanguagePropertyHandler) Update(idShortOrPath string, submodelElement gen.SubmodelElement) error {
	if dErr := p.decorated.Update(idShortOrPath, submodelElement); dErr != nil {
		return dErr
	}
	return nil
}
func (p PostgreSQLMultiLanguagePropertyHandler) Delete(idShortOrPath string) error {
	if dErr := p.decorated.Delete(idShortOrPath); dErr != nil {
		return dErr
	}
	return nil
}

func insertMultiLanguageProperty(mlp *gen.MultiLanguageProperty, tx *sql.Tx, id int) error {
	// Insert into multilanguage_property
	_, err := tx.Exec(`INSERT INTO multilanguage_property (id) VALUES ($1)`, id)
	if err != nil {
		return err
	}

	// Insert values
	for _, val := range mlp.Value {
		_, err = tx.Exec(`INSERT INTO multilanguage_property_value (mlp_id, language, text) VALUES ($1, $2, $3)`,
			id, val.Language, val.Text)
		if err != nil {
			return err
		}
	}
	return nil
}
