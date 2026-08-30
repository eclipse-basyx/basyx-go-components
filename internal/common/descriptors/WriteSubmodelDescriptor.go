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

		for i, val := range submodelDescriptors {
			position := i
			if useAppendPosition {
				position = startPosition + i
			}
			if _, err := insertSubmodelDescriptorAtPositionTx(tx, aasDescriptorID, val, position); err != nil {
				return err
			}
		}
	}
	return nil
}

func insertSubmodelDescriptorAtPositionTx(
	tx *sql.Tx,
	aasDescriptorID sql.NullInt64,
	value model.SubmodelDescriptor,
	position int,
) (int64, error) {
	if len(value.Endpoints) == 0 {
		return 0, common.NewErrBadRequest("Submodel Descriptor needs at least 1 Endpoint.")
	}

	descriptionPayload, err := buildLangStringTextPayload(value.Description)
	if err != nil {
		return 0, common.NewInternalServerError("SMDESC-INSERT-DESCRIPTIONPAYLOAD")
	}
	displayNamePayload, err := buildLangStringNamePayload(value.DisplayName)
	if err != nil {
		return 0, common.NewInternalServerError("SMDESC-INSERT-DISPLAYNAMEPAYLOAD")
	}
	administrationPayload, err := buildAdministrativeInfoPayload(value.Administration)
	if err != nil {
		return 0, common.NewInternalServerError("SMDESC-INSERT-ADMINPAYLOAD")
	}
	extensionsPayload, err := buildExtensionsPayload(value.Extensions)
	if err != nil {
		return 0, common.NewInternalServerError("SMDESC-INSERT-EXTENSIONPAYLOAD")
	}

	d := goqu.Dialect(common.Dialect)
	sqlStr, args, err := d.
		Insert(common.TblDescriptor).
		Returning(common.TDescriptor.Col(common.ColID)).
		ToSQL()
	if err != nil {
		return 0, err
	}
	var descriptorID int64
	if err = tx.QueryRow(sqlStr, args...).Scan(&descriptorID); err != nil {
		return 0, err
	}

	sqlStr, args, err = d.
		Insert(common.TblSubmodelDescriptor).
		Rows(goqu.Record{
			common.ColDescriptorID:    descriptorID,
			common.ColPosition:        position,
			common.ColAASDescriptorID: aasDescriptorID,
			common.ColIDShort:         value.IdShort,
			common.ColAASID:           value.Id,
		}).
		ToSQL()
	if err != nil {
		return 0, err
	}
	if _, err = tx.Exec(sqlStr, args...); err != nil {
		return 0, err
	}

	if err = common.CreateContextReference(
		tx,
		descriptorID,
		value.SemanticId,
		"submodel_descriptor_semantic_id_reference",
		"submodel_descriptor_semantic_id_reference_key",
	); err != nil {
		return 0, err
	}

	sqlStr, args, err = d.
		Insert(common.TblDescriptorPayload).
		Rows(goqu.Record{
			common.ColDescriptorID:              descriptorID,
			common.ColDescriptionPayload:        goqu.L("?::jsonb", string(descriptionPayload)),
			common.ColDisplayNamePayload:        goqu.L("?::jsonb", string(displayNamePayload)),
			common.ColAdministrativeInfoPayload: goqu.L("?::jsonb", string(administrationPayload)),
			common.ColExtensionsPayload:         goqu.L("?::jsonb", string(extensionsPayload)),
		}).
		ToSQL()
	if err != nil {
		return 0, err
	}
	if _, err = tx.Exec(sqlStr, args...); err != nil {
		return 0, err
	}
	if err = createsubModelDescriptorSupplementalSemantic(tx, descriptorID, value.SupplementalSemanticId); err != nil {
		return 0, err
	}
	if err = CreateEndpoints(tx, descriptorID, value.Endpoints); err != nil {
		return 0, err
	}
	return descriptorID, nil
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
	return common.CreateContextReferences1ToMany(
		tx,
		subModelDescriptorID,
		references,
		common.TblSubmodelDescriptorSuppSemantic,
		common.ColDescriptorID,
	)
}
