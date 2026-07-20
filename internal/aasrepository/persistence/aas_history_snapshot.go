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

	"github.com/FriedJannik/aas-go-sdk/jsonization"
	"github.com/FriedJannik/aas-go-sdk/types"
	"github.com/doug-martin/goqu/v9"
	persistenceutils "github.com/eclipse-basyx/basyx-go-components/internal/aasrepository/persistence/utils"
	"github.com/eclipse-basyx/basyx-go-components/internal/common"
	"github.com/eclipse-basyx/basyx-go-components/internal/common/history"
)

const (
	aasAssetInformationSnapshotField = "assetInformation"
	aasDefaultThumbnailSnapshotField = "defaultThumbnail"
	aasSubmodelsSnapshotField        = "submodels"
)

func (s *AssetAdministrationShellDatabase) appendMutatedAASHistoryTx(ctx context.Context, tx *sql.Tx, aasIdentifier string, previousSnapshot map[string]any, mutate history.SnapshotMutator) error {
	err := history.AppendMutatedVersionTx(ctx, tx, history.TableAAS, aasIdentifier, history.ChangeUpdated, previousSnapshot, func(snapshot map[string]any) error {
		return mutate(snapshot)
	})
	if err == nil || !common.IsErrNotFound(err) {
		return err
	}
	return s.appendCurrentAASHistoryTx(ctx, tx, aasIdentifier, previousSnapshot, history.ChangeUpdated)
}

func (s *AssetAdministrationShellDatabase) appendAddedSubmodelReferenceHistoryTx(ctx context.Context, tx *sql.Tx, aasIdentifier string, previousSnapshot map[string]any, reference types.IReference) error {
	jsonable, err := jsonization.ToJsonable(reference)
	if err != nil {
		return common.NewInternalServerError("AASREPO-HISTORY-SMREF-TOJSONABLE " + err.Error())
	}
	return s.appendMutatedAASHistoryTx(ctx, tx, aasIdentifier, previousSnapshot, func(snapshot map[string]any) error {
		return history.AppendSnapshotArrayItem(snapshot, aasSubmodelsSnapshotField, jsonable)
	})
}

func (s *AssetAdministrationShellDatabase) appendRemovedSubmodelReferenceHistoryTx(ctx context.Context, tx *sql.Tx, aasIdentifier string, previousSnapshot map[string]any, submodelIdentifier string) error {
	return s.appendMutatedAASHistoryTx(ctx, tx, aasIdentifier, previousSnapshot, func(snapshot map[string]any) error {
		return history.RemoveSnapshotArrayItem(snapshot, aasSubmodelsSnapshotField, func(reference map[string]any) bool {
			return snapshotReferenceContainsKeyValue(reference, submodelIdentifier)
		})
	})
}

func (s *AssetAdministrationShellDatabase) appendUploadedThumbnailHistoryTx(ctx context.Context, tx *sql.Tx, aasIdentifier string, previousSnapshot map[string]any) error {
	return s.appendMutatedAASHistoryTx(ctx, tx, aasIdentifier, previousSnapshot, func(snapshot map[string]any) error {
		thumbnail, err := loadThumbnailSnapshotTx(ctx, tx, aasIdentifier)
		if err != nil {
			return err
		}
		assetInformation, err := aasAssetInformationSnapshot(snapshot)
		if err != nil {
			return err
		}
		assetInformation[aasDefaultThumbnailSnapshotField] = thumbnail
		return nil
	})
}

func (s *AssetAdministrationShellDatabase) appendDeletedThumbnailHistoryTx(ctx context.Context, tx *sql.Tx, aasIdentifier string, previousSnapshot map[string]any) error {
	return s.appendMutatedAASHistoryTx(ctx, tx, aasIdentifier, previousSnapshot, func(snapshot map[string]any) error {
		assetInformation, err := aasAssetInformationSnapshot(snapshot)
		if err != nil {
			return err
		}
		delete(assetInformation, aasDefaultThumbnailSnapshotField)
		return nil
	})
}

func aasAssetInformationSnapshot(snapshot map[string]any) (map[string]any, error) {
	rawAssetInformation, exists := snapshot[aasAssetInformationSnapshotField]
	if !exists {
		return nil, common.NewInternalServerError("AASREPO-HISTORY-ASSETINFO-MISSING assetInformation missing from snapshot")
	}
	assetInformation, ok := rawAssetInformation.(map[string]any)
	if !ok {
		return nil, common.NewInternalServerError("AASREPO-HISTORY-ASSETINFO-INVALID assetInformation snapshot must be an object")
	}
	return assetInformation, nil
}

func loadThumbnailSnapshotTx(ctx context.Context, tx *sql.Tx, aasIdentifier string) (map[string]any, error) {
	aasDBID, err := persistenceutils.GetAssetAdministrationShellDatabaseID(tx, aasIdentifier)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, common.NewErrNotFound("AASREPO-HISTORY-THUMBNAIL-AASNOTFOUND Asset Administration Shell with ID '" + aasIdentifier + "' not found")
		}
		return nil, common.NewInternalServerError("AASREPO-HISTORY-THUMBNAIL-GETAASDBID " + err.Error())
	}

	query, args, err := goqu.From("thumbnail_file_element").
		Select("value", "content_type").
		Where(goqu.I("id").Eq(aasDBID)).
		ToSQL()
	if err != nil {
		return nil, common.NewInternalServerError("AASREPO-HISTORY-THUMBNAIL-BUILDSQL " + err.Error())
	}

	var path sql.NullString
	var contentType sql.NullString
	if err = tx.QueryRowContext(ctx, query, args...).Scan(&path, &contentType); err != nil {
		if err == sql.ErrNoRows {
			return nil, common.NewErrNotFound("AASREPO-HISTORY-THUMBNAIL-NOTFOUND Thumbnail for Asset Administration Shell with ID '" + aasIdentifier + "' not found")
		}
		return nil, common.NewInternalServerError("AASREPO-HISTORY-THUMBNAIL-EXECSQL " + err.Error())
	}

	resource := buildThumbnailResource(path, contentType)
	if resource == nil {
		return nil, common.NewErrNotFound("AASREPO-HISTORY-THUMBNAIL-EMPTYPATH Thumbnail path is empty")
	}
	jsonable, err := jsonization.ToJsonable(resource)
	if err != nil {
		return nil, common.NewInternalServerError("AASREPO-HISTORY-THUMBNAIL-TOJSONABLE " + err.Error())
	}
	return jsonable, nil
}

func snapshotReferenceContainsKeyValue(reference map[string]any, value string) bool {
	rawKeys, ok := reference["keys"].([]any)
	if !ok {
		return false
	}
	for _, rawKey := range rawKeys {
		key, ok := rawKey.(map[string]any)
		if ok && key["value"] == value {
			return true
		}
	}
	return false
}
