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

package persistence

import (
	"context"
	"database/sql"
	"encoding/json"

	"github.com/FriedJannik/aas-go-sdk/types"
	"github.com/eclipse-basyx/basyx-go-components/internal/common"
	"github.com/eclipse-basyx/basyx-go-components/internal/common/postgresstaging"
	submodelelements "github.com/eclipse-basyx/basyx-go-components/internal/submodelrepository/persistence/submodelElements"
)

const (
	submodelMetadataStageDataset         = "submodel_metadata"
	submodelElementStageDataset          = "submodel_elements"
	submodelClassifiedUpdateStageDataset = "submodel_classified_updates"
)

type stagedSubmodelTarget struct {
	stage        *postgresstaging.Stage
	elementCount int
}

func (s *SubmodelDatabase) stageSubmodelTargetTx(
	ctx context.Context,
	tx *sql.Tx,
	submodel types.ISubmodel,
) (*stagedSubmodelTarget, error) {
	stage, err := postgresstaging.Open(ctx, tx)
	if err != nil {
		return nil, err
	}
	metadata, err := buildSubmodelReconciliationTargetMetadata(submodel)
	if err != nil {
		return nil, err
	}
	metadataJSON, err := json.Marshal(metadata)
	if err != nil {
		return nil, common.NewInternalServerError("SMREPO-STAGE-METADATAJSON " + err.Error())
	}
	writer, err := stage.NewWriter(submodelMetadataStageDataset)
	if err != nil {
		return nil, err
	}
	if err = writer.Add(ctx, postgresstaging.Row{
		MatchKey: submodel.ID(),
		Ordinal:  0,
		Data:     metadataJSON,
	}); err != nil {
		return nil, err
	}
	elementCount := 0
	err = submodelelements.StreamReconciliationElementRows(
		ctx,
		s.db,
		submodel.SubmodelElements(),
		func(row submodelelements.ReconciliationElementRow) error {
			rowJSON, marshalErr := json.Marshal(row)
			if marshalErr != nil {
				return common.NewInternalServerError("SMREPO-STAGE-ELEMENTJSON " + marshalErr.Error())
			}
			var parentKey *string
			if row.ParentPath != "" {
				value := row.ParentPath
				parentKey = &value
			}
			rowType := row.ModelType
			if addErr := writer.AddToDataset(ctx, submodelElementStageDataset, postgresstaging.Row{
				MatchKey:  row.Path,
				ParentKey: parentKey,
				RowType:   &rowType,
				Ordinal:   int64(elementCount),
				Data:      rowJSON,
			}); addErr != nil {
				return addErr
			}
			elementCount++
			return nil
		},
	)
	if err != nil {
		return nil, err
	}
	if err = writer.Flush(ctx); err != nil {
		return nil, err
	}
	return &stagedSubmodelTarget{stage: stage, elementCount: elementCount}, nil
}
