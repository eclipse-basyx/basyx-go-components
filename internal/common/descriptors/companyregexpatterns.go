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

package descriptors

import (
	"context"
	"database/sql"

	"github.com/doug-martin/goqu/v9"
	"github.com/eclipse-basyx/basyx-go-components/internal/common"
	"github.com/lib/pq"
)

func createCompanyAssetIDRegexPatterns(tx *sql.Tx, descriptorID int64, patterns []string) error {
	return createCompanyRegexPatterns(tx, common.TblCompanyDescriptorAssetIDRegex, descriptorID, patterns)
}

func createCompanyRegexPatterns(tx *sql.Tx, tableName string, descriptorID int64, patterns []string) error {
	if len(patterns) == 0 {
		return nil
	}

	d := goqu.Dialect(common.Dialect)
	for i, pattern := range patterns {
		sqlStr, args, err := d.
			Insert(tableName).
			Rows(goqu.Record{
				common.ColDescriptorID: descriptorID,
				common.ColPosition:     i,
				common.ColRegexPattern: pattern,
			}).
			ToSQL()
		if err != nil {
			return err
		}
		if _, err = tx.Exec(sqlStr, args...); err != nil {
			return err
		}
	}

	return nil
}

func readCompanyAssetIDRegexPatternsByDescriptorID(ctx context.Context, db DBQueryer, descriptorID int64) ([]string, error) {
	byDescriptor, err := readCompanyAssetIDRegexPatternsByDescriptorIDs(ctx, db, []int64{descriptorID})
	if err != nil {
		return nil, err
	}
	return byDescriptor[descriptorID], nil
}

func readCompanyAssetIDRegexPatternsByDescriptorIDs(ctx context.Context, db DBQueryer, descriptorIDs []int64) (map[int64][]string, error) {
	return readCompanyRegexPatternsByDescriptorIDs(ctx, db, common.TblCompanyDescriptorAssetIDRegex, descriptorIDs)
}

func readCompanyRegexPatternsByDescriptorIDs(ctx context.Context, db DBQueryer, tableName string, descriptorIDs []int64) (map[int64][]string, error) {
	result := make(map[int64][]string, len(descriptorIDs))
	if len(descriptorIDs) == 0 {
		return result, nil
	}

	d := goqu.Dialect(common.Dialect)
	patternTbl := goqu.T(tableName).As("comp_pattern")
	arr := pq.Array(descriptorIDs)

	sqlStr, args, err := d.
		From(patternTbl).
		Select(
			patternTbl.Col(common.ColDescriptorID),
			patternTbl.Col(common.ColRegexPattern),
		).
		Where(goqu.L("? = ANY(?::bigint[])", patternTbl.Col(common.ColDescriptorID), arr)).
		Order(patternTbl.Col(common.ColDescriptorID).Asc(), patternTbl.Col(common.ColPosition).Asc()).
		ToSQL()
	if err != nil {
		return nil, err
	}

	rows, err := db.QueryContext(ctx, sqlStr, args...)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()

	for rows.Next() {
		var descriptorID int64
		var pattern string
		if err := rows.Scan(&descriptorID, &pattern); err != nil {
			return nil, err
		}
		result[descriptorID] = append(result[descriptorID], pattern)
	}

	if err := rows.Err(); err != nil {
		return nil, err
	}

	return result, nil
}
