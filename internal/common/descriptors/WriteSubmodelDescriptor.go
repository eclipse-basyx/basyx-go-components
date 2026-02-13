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
		startPosition := 0
		useAppendPosition := aasDescriptorID.Valid && len(submodelDescriptors) == 1
		if useAppendPosition {
			nextPosition, err := getNextSubmodelDescriptorPosition(tx, aasDescriptorID.Int64)
			if err != nil {
				return err
			}
			startPosition = nextPosition
		}

		d := goqu.Dialect(dialect)
		for i, val := range submodelDescriptors {
			var err error
			position := i
			if useAppendPosition {
				position = startPosition + i
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
			extensionsPayload, err := buildExtensionsPayload(val.Extensions)
			if err != nil {
				return common.NewInternalServerError("SMDESC-INSERT-EXTENSIONPAYLOAD")
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
					colPosition:        position,
					colAASDescriptorID: aasDescriptorID,
					colIDShort:         val.IdShort,
					colAASID:           val.Id,
				}).
				ToSQL()
			if err != nil {
				return err
			}
			if _, err = tx.Exec(sqlStr, args...); err != nil {
				return err
			}

			if err = createContextReference(
				tx,
				submodelDescriptorID,
				val.SemanticId,
				"submodel_descriptor_semantic_id_reference",
				"submodel_descriptor_semantic_id_reference_key",
			); err != nil {
				return err
			}

			sqlStr, args, err = d.
				Insert(tblDescriptorPayload).
				Rows(goqu.Record{
					colDescriptorID:              submodelDescriptorID,
					colDescriptionPayload:        goqu.L("?::jsonb", string(descriptionPayload)),
					colDisplayNamePayload:        goqu.L("?::jsonb", string(displayNamePayload)),
					colAdministrativeInfoPayload: goqu.L("?::jsonb", string(administrationPayload)),
					colExtensionsPayload:         goqu.L("?::jsonb", string(extensionsPayload)),
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

func getNextSubmodelDescriptorPosition(tx *sql.Tx, aasDescriptorID int64) (int, error) {
	var nextPos int
	err := tx.QueryRow(
		`SELECT COALESCE(MAX(position), -1) + 1 FROM submodel_descriptor WHERE aas_descriptor_id = $1`,
		aasDescriptorID,
	).Scan(&nextPos)
	if err != nil {
		return 0, err
	}
	return nextPos, nil
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
