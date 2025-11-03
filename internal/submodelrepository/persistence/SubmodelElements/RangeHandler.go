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

type PostgreSQLRangeHandler struct {
	db        *sql.DB
	decorated *PostgreSQLSMECrudHandler
}

func NewPostgreSQLRangeHandler(db *sql.DB) (*PostgreSQLRangeHandler, error) {
	decoratedHandler, err := NewPostgreSQLSMECrudHandler(db)
	if err != nil {
		return nil, err
	}
	return &PostgreSQLRangeHandler{db: db, decorated: decoratedHandler}, nil
}

func (p PostgreSQLRangeHandler) Create(tx *sql.Tx, submodelID string, submodelElement gen.SubmodelElement) (int, error) {
	rangeElem, ok := submodelElement.(*gen.Range)
	if !ok {
		return 0, errors.New("submodelElement is not of type Range")
	}

	// First, perform base SubmodelElement operations within the transaction
	id, err := p.decorated.Create(tx, submodelID, submodelElement)
	if err != nil {
		return 0, err
	}

	// Range-specific database insertion
	err = insertRange(rangeElem, tx, id)
	if err != nil {
		return 0, err
	}

	return id, nil
}

func (p PostgreSQLRangeHandler) CreateNested(tx *sql.Tx, submodelID string, parentID int, idShortPath string, submodelElement gen.SubmodelElement, pos int) (int, error) {
	rangeElem, ok := submodelElement.(*gen.Range)
	if !ok {
		return 0, errors.New("submodelElement is not of type Range")
	}

	// Create the nested range with the provided idShortPath using the decorated handler
	id, err := p.decorated.CreateAndPath(tx, submodelID, parentID, idShortPath, submodelElement, pos)
	if err != nil {
		return 0, err
	}

	// Range-specific database insertion for nested element
	err = insertRange(rangeElem, tx, id)
	if err != nil {
		return 0, err
	}

	return id, nil
}

func (p PostgreSQLRangeHandler) Update(idShortOrPath string, submodelElement gen.SubmodelElement) error {
	if dErr := p.decorated.Update(idShortOrPath, submodelElement); dErr != nil {
		return dErr
	}
	return nil
}
func (p PostgreSQLRangeHandler) Delete(idShortOrPath string) error {
	if dErr := p.decorated.Delete(idShortOrPath); dErr != nil {
		return dErr
	}
	return nil
}

func insertRange(rangeElem *gen.Range, tx *sql.Tx, id int) error {
	var minText, maxText, minNum, maxNum, minTime, maxTime, minDatetime, maxDatetime sql.NullString

	switch rangeElem.ValueType {
	case "xs:string", "xs:anyURI", "xs:base64Binary", "xs:hexBinary":
		minText = sql.NullString{String: rangeElem.Min, Valid: rangeElem.Min != ""}
		maxText = sql.NullString{String: rangeElem.Max, Valid: rangeElem.Max != ""}
	case "xs:int", "xs:integer", "xs:long", "xs:short",
		"xs:unsignedInt", "xs:unsignedLong", "xs:unsignedShort", "xs:unsignedByte",
		"xs:positiveInteger", "xs:negativeInteger", "xs:nonNegativeInteger", "xs:nonPositiveInteger",
		"xs:decimal", "xs:double", "xs:float":
		minNum = sql.NullString{String: rangeElem.Min, Valid: rangeElem.Min != ""}
		maxNum = sql.NullString{String: rangeElem.Max, Valid: rangeElem.Max != ""}
	case "xs:time":
		minTime = sql.NullString{String: rangeElem.Min, Valid: rangeElem.Min != ""}
		maxTime = sql.NullString{String: rangeElem.Max, Valid: rangeElem.Max != ""}
	case "xs:date", "xs:dateTime", "xs:duration", "xs:gDay", "xs:gMonth",
		"xs:gMonthDay", "xs:gYear", "xs:gYearMonth":
		minDatetime = sql.NullString{String: rangeElem.Min, Valid: rangeElem.Min != ""}
		maxDatetime = sql.NullString{String: rangeElem.Max, Valid: rangeElem.Max != ""}
	default:
		// Fallback to text
		minText = sql.NullString{String: rangeElem.Min, Valid: rangeElem.Min != ""}
		maxText = sql.NullString{String: rangeElem.Max, Valid: rangeElem.Max != ""}
	}

	// Insert Range-specific data
	_, err := tx.Exec(`INSERT INTO range_element (id, value_type, min_text, max_text, min_num, max_num, min_time, max_time, min_datetime, max_datetime)
					 VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10)`,
		id, rangeElem.ValueType,
		minText, maxText, minNum, maxNum, minTime, maxTime, minDatetime, maxDatetime)
	return err
}
