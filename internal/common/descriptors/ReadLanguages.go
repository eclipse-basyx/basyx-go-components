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
// Author: Martin Stemmer ( Fraunhofer IESE )

package descriptors

import (
	"fmt"

	"github.com/FriedJannik/aas-go-sdk/types"
	"github.com/doug-martin/goqu/v9"
	"github.com/lib/pq"
)

// GetLangStringTextTypesByIDs fetches LangStringTextType rows for the given
// reference IDs and groups them by their reference ID. An empty input returns
// an empty map.
func GetLangStringTextTypesByIDs(
	db DBQueryer,
	textTypeIDs []int64,
) (map[int64][]types.ILangStringTextType, error) {
	out := make(map[int64][]types.ILangStringTextType, len(textTypeIDs))
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
		langStringTextType := types.NewLangStringTextType(language, text)
		out[refID] = append(out[refID], langStringTextType)
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
	db DBQueryer,
	nameTypeIDs []int64,
) (map[int64][]types.ILangStringNameType, error) {
	out := make(map[int64][]types.ILangStringNameType, len(nameTypeIDs))
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
		langStringNameType := types.NewLangStringNameType(language, text)
		out[refID] = append(out[refID], langStringNameType)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	return out, nil
}
