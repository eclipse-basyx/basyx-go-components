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
// Author: Martin Stemmer ( Fraunhofer IESE )
package descriptors

import (
	"database/sql"
	"fmt"

	"github.com/doug-martin/goqu/v9"
	"github.com/eclipse-basyx/basyx-go-components/internal/common/model"
	"github.com/lib/pq"
)

// GetLangStringTextTypesByIDs fetches LangStringTextType rows for the given
// reference IDs and groups them by their reference ID. An empty input returns
// an empty map.
func GetLangStringTextTypesByIDs(
	db *sql.DB,
	textTypeIDs []int64,
) (map[int64][]model.LangStringTextType, error) {

	out := make(map[int64][]model.LangStringTextType, len(textTypeIDs))
	if len(textTypeIDs) == 0 {
		return out, nil
	}

	dialect := goqu.Dialect(dialect)

	arr := pq.Array(textTypeIDs)
	ds := dialect.
		From(goqu.T(tblLangStringTextType)).
		Select(colLangStringTextTypeReferenceID, colText, colLanguage).
		Where(goqu.L(fmt.Sprintf("%s = ANY(?::bigint[])", colLangStringTextTypeReferenceID), arr))

	sqlStr, args, err := ds.ToSQL()
	if err != nil {
		return nil, err
	}

	rows, err := db.Query(sqlStr, args...)
	if err != nil {
		return nil, err
	}
	defer func() {
		_ = rows.Close()
	}()

	for rows.Next() {
		var refID int64
		var text, language string
		if err := rows.Scan(&refID, &text, &language); err != nil {
			return nil, err
		}
		out[refID] = append(out[refID], model.LangStringTextType{
			Text:     text,
			Language: language,
		})
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	return out, nil
}

// GetLangStringNameTypesByIDs fetches LangStringNameType rows for the given
// reference IDs and groups them by their reference ID. An empty input returns
// an empty map.
func GetLangStringNameTypesByIDs(
	db *sql.DB,
	nameTypeIDs []int64,
) (map[int64][]model.LangStringNameType, error) {

	out := make(map[int64][]model.LangStringNameType, len(nameTypeIDs))
	if len(nameTypeIDs) == 0 {
		return out, nil
	}

	dialect := goqu.Dialect(dialect)

	// Build query
	arr := pq.Array(nameTypeIDs)
	ds := dialect.
		From(goqu.T(tblLangStringNameType)).
		Select(colLangStringNameTypeReferenceID, colText, colLanguage).
		Where(goqu.L(fmt.Sprintf("%s = ANY(?::bigint[])", colLangStringNameTypeReferenceID), arr))

	sqlStr, args, err := ds.ToSQL()
	if err != nil {
		return nil, err
	}

	rows, err := db.Query(sqlStr, args...)
	if err != nil {
		return nil, err
	}
	defer func() {
		_ = rows.Close()
	}()

	for rows.Next() {
		var refID int64
		var text, language string
		if err := rows.Scan(&refID, &text, &language); err != nil {
			return nil, err
		}
		out[refID] = append(out[refID], model.LangStringNameType{
			Text:     text,
			Language: language,
		})
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	return out, nil
}
