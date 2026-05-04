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

func createCompanyNameOptions(tx *sql.Tx, descriptorID int64, nameOptions []string) error {
	if len(nameOptions) == 0 {
		return nil
	}

	d := goqu.Dialect(common.Dialect)
	for i, option := range nameOptions {
		sqlStr, args, err := d.
			Insert(common.TblCompanyDescriptorNameOption).
			Rows(goqu.Record{
				common.ColDescriptorID: descriptorID,
				common.ColPosition:     i,
				common.ColNameOption:   option,
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

func readCompanyNameOptionsByDescriptorID(ctx context.Context, db DBQueryer, descriptorID int64) ([]string, error) {
	byDescriptor, err := readCompanyNameOptionsByDescriptorIDs(ctx, db, []int64{descriptorID})
	if err != nil {
		return nil, err
	}
	return byDescriptor[descriptorID], nil
}

func readCompanyNameOptionsByDescriptorIDs(ctx context.Context, db DBQueryer, descriptorIDs []int64) (map[int64][]string, error) {
	result := make(map[int64][]string, len(descriptorIDs))
	if len(descriptorIDs) == 0 {
		return result, nil
	}

	d := goqu.Dialect(common.Dialect)
	nameOpt := goqu.T(common.TblCompanyDescriptorNameOption).As("comp_name_opt")
	arr := pq.Array(descriptorIDs)

	sqlStr, args, err := d.
		From(nameOpt).
		Select(
			nameOpt.Col(common.ColDescriptorID),
			nameOpt.Col(common.ColNameOption),
		).
		Where(goqu.L("? = ANY(?::bigint[])", nameOpt.Col(common.ColDescriptorID), arr)).
		Order(nameOpt.Col(common.ColDescriptorID).Asc(), nameOpt.Col(common.ColPosition).Asc()).
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
		var option string
		if err := rows.Scan(&descriptorID, &option); err != nil {
			return nil, err
		}
		result[descriptorID] = append(result[descriptorID], option)
	}

	if err := rows.Err(); err != nil {
		return nil, err
	}

	return result, nil
}
