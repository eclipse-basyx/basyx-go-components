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
	"database/sql"

	"github.com/FriedJannik/aas-go-sdk/types"
	"github.com/doug-martin/goqu/v9"
	"github.com/eclipse-basyx/basyx-go-components/internal/common"
)

func createContextReference(
	tx *sql.Tx,
	ownerID int64,
	reference types.IReference,
	referenceTable string,
	referenceKeyTable string,
) error {
	if reference == nil {
		return nil
	}

	d := goqu.Dialect(common.Dialect)
	sqlStr, args, err := d.Insert(referenceTable).Rows(goqu.Record{
		common.ColID:   ownerID,
		common.ColType: reference.Type(),
	}).ToSQL()
	if err != nil {
		return err
	}
	if _, err = tx.Exec(sqlStr, args...); err != nil {
		return err
	}

	payloadTable := referenceTable + "_payload"
	parentReferencePayload, err := buildReferencePayload(reference.ReferredSemanticID())
	if err != nil {
		return err
	}

	sqlStr, args, err = d.Insert(payloadTable).Rows(goqu.Record{
		common.ColReferenceID:      ownerID,
		"parent_reference_payload": goqu.L("?::jsonb", string(parentReferencePayload)),
	}).ToSQL()
	if err != nil {
		return err
	}
	if _, err = tx.Exec(sqlStr, args...); err != nil {
		return err
	}

	keys := reference.Keys()
	if len(keys) == 0 {
		return nil
	}

	rows := make([]goqu.Record, 0, len(keys))
	for i, key := range keys {
		rows = append(rows, goqu.Record{
			common.ColReferenceID: ownerID,
			common.ColPosition:    i,
			common.ColType:        key.Type(),
			common.ColValue:       key.Value(),
		})
	}

	sqlStr, args, err = d.Insert(referenceKeyTable).Rows(rows).ToSQL()
	if err != nil {
		return err
	}
	_, err = tx.Exec(sqlStr, args...)
	return err
}

func createContextReferences1ToMany(
	tx *sql.Tx,
	ownerID int64,
	references []types.IReference,
	referenceTable string,
	ownerColumn string,
) error {
	if len(references) == 0 {
		return nil
	}

	d := goqu.Dialect(common.Dialect)
	referenceKeyTable := referenceTable + "_key"
	payloadTable := referenceTable + "_payload"

	for _, reference := range references {
		if reference == nil {
			continue
		}

		sqlStr, args, err := d.Insert(referenceTable).Rows(goqu.Record{
			ownerColumn:    ownerID,
			common.ColType: reference.Type(),
		}).Returning(goqu.C(common.ColID)).ToSQL()
		if err != nil {
			return err
		}

		var referenceID int64
		if err = tx.QueryRow(sqlStr, args...).Scan(&referenceID); err != nil {
			return err
		}

		parentReferencePayload, err := buildReferencePayload(reference.ReferredSemanticID())
		if err != nil {
			return err
		}
		sqlStr, args, err = d.Insert(payloadTable).Rows(goqu.Record{
			common.ColReferenceID:      referenceID,
			"parent_reference_payload": goqu.L("?::jsonb", string(parentReferencePayload)),
		}).ToSQL()
		if err != nil {
			return err
		}
		if _, err = tx.Exec(sqlStr, args...); err != nil {
			return err
		}

		keys := reference.Keys()
		if len(keys) == 0 {
			continue
		}

		rows := make([]goqu.Record, 0, len(keys))
		for i, key := range keys {
			rows = append(rows, goqu.Record{
				common.ColReferenceID: referenceID,
				common.ColPosition:    i,
				common.ColType:        key.Type(),
				common.ColValue:       key.Value(),
			})
		}

		sqlStr, args, err = d.Insert(referenceKeyTable).Rows(rows).ToSQL()
		if err != nil {
			return err
		}
		if _, err = tx.Exec(sqlStr, args...); err != nil {
			return err
		}
	}
	return nil
}
