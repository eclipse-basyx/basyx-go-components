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
	"github.com/eclipse-basyx/basyx-go-components/internal/common/model"
	persistence_utils "github.com/eclipse-basyx/basyx-go-components/internal/submodelrepository/persistence/utils"
)

func createSubModelDescriptors(tx *sql.Tx, aasDescriptorID sql.NullInt64, submodelDescriptors []model.SubmodelDescriptor) error {
	if submodelDescriptors == nil {
		return nil
	}
	if len(submodelDescriptors) > 0 {
		d := goqu.Dialect(dialect)
		for i, val := range submodelDescriptors {
			var (
				semanticID sql.NullInt64
				err        error
			)

			var a sql.NullInt64
			semanticID, err = persistence_utils.CreateReference(tx, val.SemanticId, a, a)
			if err != nil {
				return err
			}

			descriptionPayload, err := buildLangStringTextPayload(val.Description)
			if err != nil {
				return common.NewInternalServerError("SMDESC-INSERT-DESCRIPTIONPAYLOAD")
			}
			displayNamePayload, err := buildLangStringNamePayload(val.DisplayName)
			if err != nil {
				return common.NewInternalServerError("SMDESC-INSERT-DISPLAYNAMEPAYLOAD")
			}
			administrationPayload, err := buildAdministrativeInfoPayload(val.Administration)
			if err != nil {
				return common.NewInternalServerError("SMDESC-INSERT-ADMINPAYLOAD")
			}

			sqlStr, args, err := d.
				Insert(tblDescriptor).
				Returning(tDescriptor.Col(colID)).
				ToSQL()
			if err != nil {
				return err
			}
			var submodelDescriptorID int64
			if err = tx.QueryRow(sqlStr, args...).Scan(&submodelDescriptorID); err != nil {
				return err
			}

			sqlStr, args, err = d.
				Insert(tblSubmodelDescriptor).
				Rows(goqu.Record{
					colDescriptorID:    submodelDescriptorID,
					colPosition:        i,
					colAASDescriptorID: aasDescriptorID,
					colIDShort:         val.IdShort,
					colAASID:           val.Id,
					colSemanticID:      semanticID,
				}).
				ToSQL()
			if err != nil {
				return err
			}
			if _, err = tx.Exec(sqlStr, args...); err != nil {
				return err
			}

			sqlStr, args, err = d.
				Insert(tblDescriptorPayload).
				Rows(goqu.Record{
					colDescriptorID:              submodelDescriptorID,
					colDescriptionPayload:        goqu.L("?::jsonb", string(descriptionPayload)),
					colDisplayNamePayload:        goqu.L("?::jsonb", string(displayNamePayload)),
					colAdministrativeInfoPayload: goqu.L("?::jsonb", string(administrationPayload)),
				}).
				ToSQL()
			if err != nil {
				return err
			}
			if _, err = tx.Exec(sqlStr, args...); err != nil {
				return err
			}

			if err = createsubModelDescriptorSupplementalSemantic(tx, submodelDescriptorID, val.SupplementalSemanticId); err != nil {
				return err
			}

			if err = createExtensions(tx, submodelDescriptorID, val.Extensions); err != nil {
				return err
			}

			if len(val.Endpoints) == 0 {
				return common.NewErrBadRequest("Submodel Descriptor needs at least 1 Endpoint.")
			}
			if err = CreateEndpoints(tx, submodelDescriptorID, val.Endpoints); err != nil {
				return err
			}
		}
	}
	return nil
}

func createsubModelDescriptorSupplementalSemantic(tx *sql.Tx, subModelDescriptorID int64, references []types.IReference) error {
	if len(references) == 0 {
		return nil
	}
	d := goqu.Dialect(dialect)
	rows := make([]goqu.Record, 0, len(references))
	for i := range references {
		var a sql.NullInt64
		referenceID, err := persistence_utils.CreateReference(tx, references[i], a, a)
		if err != nil {
			return err
		}
		rows = append(rows, goqu.Record{
			colDescriptorID: subModelDescriptorID,
			colReferenceID:  referenceID,
		})
	}
	sqlStr, args, err := d.Insert(tblSubmodelDescriptorSuppSemantic).Rows(rows).ToSQL()
	if err != nil {
		return err
	}
	_, err = tx.Exec(sqlStr, args...)
	return err
}
