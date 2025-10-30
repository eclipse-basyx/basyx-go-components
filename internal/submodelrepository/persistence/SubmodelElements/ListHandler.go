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

type PostgreSQLSubmodelElementListHandler struct {
	db        *sql.DB
	decorated *PostgreSQLSMECrudHandler
}

func NewPostgreSQLSubmodelElementListHandler(db *sql.DB) (*PostgreSQLSubmodelElementListHandler, error) {
	decoratedHandler, err := NewPostgreSQLSMECrudHandler(db)
	if err != nil {
		return nil, err
	}
	return &PostgreSQLSubmodelElementListHandler{db: db, decorated: decoratedHandler}, nil
}

func (p PostgreSQLSubmodelElementListHandler) Create(tx *sql.Tx, submodelId string, submodelElement gen.SubmodelElement) (int, error) {
	smeList, ok := submodelElement.(*gen.SubmodelElementList)
	if !ok {
		return 0, errors.New("submodelElement is not of type SubmodelElementList")
	}

	// First, perform base SubmodelElement operations within the transaction
	id, err := p.decorated.Create(tx, submodelId, submodelElement)
	if err != nil {
		return 0, err
	}

	// SubmodelElementList-specific database insertion
	err = insertSubmodelElementList(smeList, tx, id)
	if err != nil {
		return 0, err
	}

	return id, nil
}

func (p PostgreSQLSubmodelElementListHandler) CreateNested(tx *sql.Tx, submodelId string, parentId int, idShortPath string, submodelElement gen.SubmodelElement, pos int) (int, error) {
	smeList, ok := submodelElement.(*gen.SubmodelElementList)
	if !ok {
		return 0, errors.New("submodelElement is not of type SubmodelElementList")
	}

	// First, perform base SubmodelElement operations within the transaction
	id, err := p.decorated.CreateAndPath(tx, submodelId, parentId, idShortPath, submodelElement, pos)
	if err != nil {
		return 0, err
	}

	// SubmodelElementList-specific database insertion
	err = insertSubmodelElementList(smeList, tx, id)
	if err != nil {
		return 0, err
	}

	return id, nil
}

func (p PostgreSQLSubmodelElementListHandler) Read(tx *sql.Tx, submodelId string, idShortOrPath string) (gen.SubmodelElement, error) {
	return nil, nil
	// var sme gen.SubmodelElement = &gen.SubmodelElementList{}
	// id, err := p.decorated.Read(tx, submodelId, idShortOrPath, &sme)
	// if err != nil {
	// 	return nil, err
	// }

	// // Check if there are Children and load them if necessary
	// var idShortPath string
	// rows, err := tx.Query(`
	// 	SELECT idshort_path FROM submodel_element WHERE parent_sme_id = $1 ORDER BY position
	// `, id)
	// if err != nil {
	// 	return nil, err
	// }
	// defer rows.Close()
	// var children []gen.SubmodelElement
	// for rows.Next() {
	// 	if err := rows.Scan(&idShortPath); err != nil {
	// 		return nil, err
	// 	}
	// 	sme, err := GetSubmodelElement(p.db, submodelId, idShortPath)
	// 	if err != nil {
	// 		return nil, err
	// 	}
	// 	children = append(children, sme)
	// }
	// sml, ok := sme.(*gen.SubmodelElementList)
	// if ok {
	// 	sml.Value = children
	// }
	// return sme, nil
}
func (p PostgreSQLSubmodelElementListHandler) Update(idShortOrPath string, submodelElement gen.SubmodelElement) error {
	if dErr := p.decorated.Update(idShortOrPath, submodelElement); dErr != nil {
		return dErr
	}
	return nil
}
func (p PostgreSQLSubmodelElementListHandler) Delete(idShortOrPath string) error {
	if dErr := p.decorated.Delete(idShortOrPath); dErr != nil {
		return dErr
	}
	return nil
}

func insertSubmodelElementList(smeList *gen.SubmodelElementList, tx *sql.Tx, id int) error {
	var semanticId sql.NullInt64
	if smeList.SemanticIdListElement != nil && !isEmptyReference(smeList.SemanticIdListElement) {
		refId, err := insertReference(tx, *smeList.SemanticIdListElement)
		if err != nil {
			return err
		}
		semanticId = sql.NullInt64{Int64: int64(refId), Valid: true}
	}

	var typeValue, valueType sql.NullString
	if smeList.TypeValueListElement != nil {
		typeValue = sql.NullString{String: string(*smeList.TypeValueListElement), Valid: true}
	}
	if smeList.ValueTypeListElement != "" {
		valueType = sql.NullString{String: string(smeList.ValueTypeListElement), Valid: true}
	}

	_, err := tx.Exec(`INSERT INTO submodel_element_list (id, order_relevant, semantic_id_list_element, type_value_list_element, value_type_list_element)
					 VALUES ($1, $2, $3, $4, $5)`,
		id, smeList.OrderRelevant, semanticId, typeValue, valueType)
	return err
}
