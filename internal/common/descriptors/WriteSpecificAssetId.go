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
	"encoding/json"

	"github.com/doug-martin/goqu/v9"
	"github.com/eclipse-basyx/basyx-go-components/internal/common/model"
	persistence_utils "github.com/eclipse-basyx/basyx-go-components/internal/submodelrepository/persistence/utils"
)

func createSpecificAssetID(tx *sql.Tx, descriptorID int64, specificAssetIDs []model.SpecificAssetID) error {
	if specificAssetIDs == nil {
		return nil
	}
	if len(specificAssetIDs) > 0 {
		d := goqu.Dialect(dialect)
		for i, val := range specificAssetIDs {
			var a sql.NullInt64

			externalSubjectReferenceID, err := persistence_utils.CreateReference(tx, val.ExternalSubjectID, a, a)
			if err != nil {
				return err
			}

			semanticIDValue, err := json.Marshal(val.SemanticID)
			if err != nil {
				return err
			}
			supplementalSemanticIDsValue, err := json.Marshal(val.SupplementalSemanticIds)
			if err != nil {
				return err
			}
			sqlStr, args, err := d.
				Insert(tblSpecificAssetID).
				Rows(goqu.Record{
					colDescriptorID:             descriptorID,
					colPosition:                 i,
					colSemanticID:               semanticIDValue,
					"supplemental_semantic_ids": supplementalSemanticIDsValue,
					colName:                     val.Name,
					colValue:                    val.Value,
					colExternalSubjectRef:       externalSubjectReferenceID,
				}).
				Returning(tSpecificAssetID.Col(colID)).
				ToSQL()
			if err != nil {
				return err
			}
			var id int64
			if err = tx.QueryRow(sqlStr, args...).Scan(&id); err != nil {
				return err
			}

		}
	}
	return nil
}
