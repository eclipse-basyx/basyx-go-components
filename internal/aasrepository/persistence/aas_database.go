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

// Package persistence contains the implementation of the AssetAdministrationShellRepositoryDatabase interface using PostgreSQL as the underlying database.
package persistence

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"os"
	"strconv"
	"time"

	"github.com/FriedJannik/aas-go-sdk/jsonization"
	"github.com/FriedJannik/aas-go-sdk/stringification"
	"github.com/FriedJannik/aas-go-sdk/types"
	"github.com/FriedJannik/aas-go-sdk/verification"
	"github.com/doug-martin/goqu/v9"
	persistenceutils "github.com/eclipse-basyx/basyx-go-components/internal/aasrepository/persistence/utils"
	"github.com/eclipse-basyx/basyx-go-components/internal/common"
	"github.com/eclipse-basyx/basyx-go-components/internal/common/descriptors"
	"github.com/lib/pq"
)

// AssetAdministrationShellDatabase is the implementation of the AssetAdministrationShellRepositoryDatabase interface using PostgreSQL as the underlying database.
type AssetAdministrationShellDatabase struct {
	db                 *sql.DB
	strictVerification bool
}

// NewAssetAdministrationShellDatabase creates a new instance of AssetAdministrationShellDatabase with the provided database connection.
func NewAssetAdministrationShellDatabase(dsn string, maxOpenConnections int, maxIdleConnections int, connMaxLifetimeMinutes int, databaseSchema string, strictVerification bool) (*AssetAdministrationShellDatabase, error) {
	db, err := common.InitializeDatabase(dsn, databaseSchema)
	if err != nil {
		return nil, err
	}

	if maxOpenConnections > 0 {
		db.SetMaxOpenConns(int(maxOpenConnections))
	}
	if maxIdleConnections > 0 {
		db.SetMaxIdleConns(maxIdleConnections)
	}
	if connMaxLifetimeMinutes > 0 {
		db.SetConnMaxLifetime(time.Duration(connMaxLifetimeMinutes) * time.Minute)
	}

	return &AssetAdministrationShellDatabase{
		db:                 db,
		strictVerification: strictVerification,
	}, nil
}

// verifyAssetAdministrationShell validates an AAS when strict verification is enabled.
func (s *AssetAdministrationShellDatabase) verifyAssetAdministrationShell(aas types.IAssetAdministrationShell, errorPrefix string) error {
	if !s.strictVerification {
		return nil
	}

	verificationErrors := make([]verification.VerificationError, 0)

	verification.VerifyAssetAdministrationShell(aas, func(ve *verification.VerificationError) bool {
		verificationErrors = append(verificationErrors, *ve)
		return false
	})

	if len(verificationErrors) == 0 {
		return nil
	}

	stringOfAllErrors := ""
	for _, err := range verificationErrors {
		stringOfAllErrors += fmt.Sprintf("%s ", err.Error())
	}

	return common.NewErrBadRequest(errorPrefix + " " + stringOfAllErrors)
}

// verifyAssetInformation validates an AssetInformation when strict verification is enabled.
func (s *AssetAdministrationShellDatabase) verifyAssetInformation(asset_information types.IAssetInformation, errorPrefix string) error {
	if !s.strictVerification {
		return nil
	}

	verificationErrors := make([]verification.VerificationError, 0)

	verification.VerifyAssetInformation(asset_information, func(ve *verification.VerificationError) bool {
		verificationErrors = append(verificationErrors, *ve)
		return false
	})

	if len(verificationErrors) == 0 {
		return nil
	}

	stringOfAllErrors := ""
	for _, err := range verificationErrors {
		stringOfAllErrors += fmt.Sprintf("%s ", err.Error())
	}

	return common.NewErrBadRequest(errorPrefix + " " + stringOfAllErrors)
}

// verifyReference validates a Reference when strict verification is enabled.
func (s *AssetAdministrationShellDatabase) verifyReference(reference types.IReference, errorPrefix string) error {
	if !s.strictVerification {
		return nil
	}

	verificationErrors := make([]verification.VerificationError, 0)

	verification.VerifyReference(reference, func(ve *verification.VerificationError) bool {
		verificationErrors = append(verificationErrors, *ve)
		return false
	})

	if len(verificationErrors) == 0 {
		return nil
	}

	stringOfAllErrors := ""
	for _, err := range verificationErrors {
		stringOfAllErrors += fmt.Sprintf("%s ", err.Error())
	}

	return common.NewErrBadRequest(errorPrefix + " " + stringOfAllErrors)
}

// CreateAssetAdministrationShell persists a new AAS including all related payload and reference data.
func (s *AssetAdministrationShellDatabase) CreateAssetAdministrationShell(aas types.IAssetAdministrationShell) error {
	if err := s.verifyAssetAdministrationShell(aas, "AASREPO-NEWAAS-VERIFY"); err != nil {
		return err
	}

	tx, cleanup, err := common.StartTransaction(s.db)
	if err != nil {
		return common.NewInternalServerError("AASREPO-NEWAAS-STARTTX " + err.Error())
	}
	defer cleanup(&err)

	err = s.createAssetAdministrationShellInTransaction(tx, aas)
	if err != nil {
		return err
	}

	err = tx.Commit()
	if err != nil {
		return common.NewInternalServerError("AASREPO-NEWAAS-CREATE-COMMIT " + err.Error())
	}

	return nil
}

// createAssetAdministrationShellInTransaction creates an AAS and all dependent records within an existing transaction.
func (s *AssetAdministrationShellDatabase) createAssetAdministrationShellInTransaction(tx *sql.Tx, aas types.IAssetAdministrationShell) error {
	dialect := goqu.Dialect("postgres")

	ids, args, err := buildAssetAdministrationShellQuery(&dialect, aas)
	if err != nil {
		return common.NewInternalServerError("AASREPO-NEWAAS-CREATE-BUILDINSERTSQL " + err.Error())
	}

	var aasDBID int64
	if err := tx.QueryRow(ids, args...).Scan(&aasDBID); err != nil {
		if mappedErr := mapCreateAASInsertError(err); mappedErr != nil {
			return mappedErr
		}
		return common.NewInternalServerError("AASREPO-NEWAAS-CREATE-EXECINSERTSQL " + err.Error())
	}

	jsonizedPayload, err := jsonizeAssetAdministrationShellPayload(aas)
	if err != nil {
		return common.NewInternalServerError("AASREPO-NEWAAS-CREATE-JSON " + err.Error())
	}

	ids, args, err = buildAssetAdministrationShellPayloadQuery(
		&dialect,
		aasDBID,
		jsonizedPayload.description,
		jsonizedPayload.displayName,
		jsonizedPayload.administrativeInformation,
		jsonizedPayload.embeddedDataSpecification,
		jsonizedPayload.extensions,
		jsonizedPayload.derivedFrom,
	)
	if err != nil {
		return common.NewInternalServerError("AASREPO-NEWAAS-CREATE-BUILDPAYLOADSQL " + err.Error())
	}

	if _, err := tx.Exec(ids, args...); err != nil {
		return common.NewInternalServerError("AASREPO-NEWAAS-CREATE-EXECPAYLOADSQL " + err.Error())
	}

	// asset information
	ids, args, err = buildAssetInformationQuery(
		&dialect,
		aasDBID,
		aas.AssetInformation(),
	)
	if err != nil {
		return common.NewInternalServerError("AASREPO-NEWAAS-CREATE-BUILDASSETINFORMATIONSQL " + err.Error())
	}

	if _, err := tx.Exec(ids, args...); err != nil {
		return common.NewInternalServerError("AASREPO-NEWAAS-CREATE-EXECASSETINFORMATIONSQL " + err.Error())
	}

	// specific asset ids
	err = common.CreateSpecificAssetIDForAssetInformation(tx, aasDBID, aas.AssetInformation().SpecificAssetIDs())
	if err != nil {
		return common.NewInternalServerError("AASREPO-NEWAAS-CREATE-CREATESPECIFICASSETIDS " + err.Error())
	}

	// submodel references
	for _, submodelRef := range aas.Submodels() {
		ids, args, err = buildAssetAdministrationShellSubmodelReferenceQuery(&dialect, aasDBID, submodelRef)
		if err != nil {
			return common.NewInternalServerError("AASREPO-NEWAAS-CREATE-BUILDSUBMODELREFSQL " + err.Error())
		}

		var aasSubmodelReferenceDBID int64
		if err := tx.QueryRow(ids, args...).Scan(&aasSubmodelReferenceDBID); err != nil {
			return common.NewInternalServerError("AASREPO-NEWAAS-CREATE-EXECSUBMODELREFSQL " + err.Error())
		}

		ids, args, err = buildAssetAdministrationShellSubmodelReferenceKeysQuery(&dialect, aasSubmodelReferenceDBID, submodelRef)
		if err != nil {
			return common.NewInternalServerError("AASREPO-NEWAAS-CREATE-BUILDSUBMODELREFKEYSSQL " + err.Error())
		}

		if _, err := tx.Exec(ids, args...); err != nil {
			return common.NewInternalServerError("AASREPO-NEWAAS-CREATE-EXECSUBMODELREFKEYSSQL " + err.Error())
		}

		ids, args, err = buildAssetAdministrationShellSubmodelReferencePayloadQuery(&dialect, aasSubmodelReferenceDBID, submodelRef)
		if err != nil {
			return common.NewInternalServerError("AASREPO-NEWAAS-CREATE-BUILDSUBMODELREFPAYLOADSSQL " + err.Error())
		}

		if _, err := tx.Exec(ids, args...); err != nil {
			return common.NewInternalServerError("AASREPO-NEWAAS-CREATE-EXECSUBMODELREFPAYLOADSSQL " + err.Error())
		}
	}
	return nil
}

// mapCreateAASInsertError maps database uniqueness violations to domain-specific conflict errors.
func mapCreateAASInsertError(err error) error {
	if err == nil {
		return nil
	}

	pqErr, ok := err.(*pq.Error)
	if !ok {
		return nil
	}

	if pqErr.Code == "23505" {
		return common.NewErrConflict("AASREPO-NEWAAS-CONFLICT AAS with given id already exists")
	}

	return nil
}

// CreateSubmodelReferenceInAssetAdministrationShell adds a submodel reference to the specified AAS.
func (s *AssetAdministrationShellDatabase) CreateSubmodelReferenceInAssetAdministrationShell(aasIdentifier string, submodelRef types.IReference) error {
	if err := s.verifyReference(submodelRef, "AASREPO-NEWSMREFINAAS-VERIFY"); err != nil {
		return err
	}

	tx, cleanup, err := common.StartTransaction(s.db)
	if err != nil {
		return err
	}
	defer cleanup(&err)

	err = s.createSubmodelReferenceInAssetAdministrationShellInTransaction(tx, aasIdentifier, submodelRef)
	if err != nil {
		return err
	}

	err = tx.Commit()
	if err != nil {
		return common.NewInternalServerError("AASREPO-NEWSMREFINAAS-COMMIT " + err.Error())
	}
	return nil
}

// createSubmodelReferenceInAssetAdministrationShellInTransaction adds a submodel reference within an existing transaction.
func (s *AssetAdministrationShellDatabase) createSubmodelReferenceInAssetAdministrationShellInTransaction(tx *sql.Tx, aasIdentifier string, submodelRef types.IReference) error {
	// check if aas exists
	aasDBID, err := persistenceutils.GetAssetAdministrationShellDatabaseID(tx, aasIdentifier)
	if err != nil {
		if err == sql.ErrNoRows {
			return common.NewErrNotFound("AASREPO-NEWSMREFINAAS-AASNOTFOUND Asset Administration Shell with ID '" + aasIdentifier + "' not found")
		}
		return common.NewInternalServerError("AASREPO-CHECKSMREFINAAS-GETAASDBID " + err.Error())
	}

	dialect := goqu.Dialect("postgres")

	ids, args, err := buildAssetAdministrationShellSubmodelReferenceQuery(&dialect, aasDBID, submodelRef)
	if err != nil {
		return common.NewInternalServerError("AASREPO-NEWSMREFINAAS-CREATE-BUILDSUBMODELREFSQL " + err.Error())
	}

	var aasSubmodelReferenceDBID int64

	if err := tx.QueryRow(ids, args...).Scan(&aasSubmodelReferenceDBID); err != nil {
		return common.NewInternalServerError("AASREPO-NEWSMREFINAAS-CREATE-EXECSUBMODELREFSQL" + err.Error())
	}

	ids, args, err = buildAssetAdministrationShellSubmodelReferenceKeysQuery(&dialect, aasSubmodelReferenceDBID, submodelRef)
	if err != nil {
		return common.NewInternalServerError("AASREPO-NEWSMREFINAAS-CREATE-BUILDSUBMODELREFKEYSQL " + err.Error())
	}

	if _, err := tx.Exec(ids, args...); err != nil {
		return common.NewInternalServerError("AASREPO-NEWSMREFINAAS-CREATE-EXECSUBMODELREFKEYSSQL " + err.Error())
	}

	ids, args, err = buildAssetAdministrationShellSubmodelReferencePayloadQuery(&dialect, aasSubmodelReferenceDBID, submodelRef)
	if err != nil {
		return common.NewInternalServerError("AASREPO-NEWSMREFINAAS-CREATE-BUILDSUBMODELREFPAYLOADQL " + err.Error())
	}

	if _, err := tx.Exec(ids, args...); err != nil {
		return common.NewInternalServerError("AASREPO-NEWSMREFINAAS-CREATE-EXECSUBMODELREFPAYLOADSSQL " + err.Error())
	}

	return nil
}

// CheckIfSubmodelReferenceExistsInAssetAdministrationShell checks whether a submodel reference exists in the specified AAS.
func (s *AssetAdministrationShellDatabase) CheckIfSubmodelReferenceExistsInAssetAdministrationShell(aasIdentifier string, submodelIdentifier string) error {
	tx, cleanup, err := common.StartTransaction(s.db)
	if err != nil {
		return common.NewInternalServerError("AASREPO-CHECKSMREFINAAS-STARTTX " + err.Error())
	}
	defer cleanup(&err)

	err = s.checkIfSubmodelReferenceExistsInAssetAdministrationShellInTransaction(tx, aasIdentifier, submodelIdentifier)
	if err != nil {
		return err
	}

	err = tx.Commit()
	if err != nil {
		return common.NewInternalServerError("AASREPO-CHECKSMREFINAAS-COMMIT " + err.Error())
	}

	return nil
}

// checkIfSubmodelReferenceExistsInAssetAdministrationShellInTransaction performs the existence check within an existing transaction.
func (s *AssetAdministrationShellDatabase) checkIfSubmodelReferenceExistsInAssetAdministrationShellInTransaction(tx *sql.Tx, aasIdentifier string, submodelIdentifier string) error {
	aasDBID, err := persistenceutils.GetAssetAdministrationShellDatabaseID(tx, aasIdentifier)
	if err != nil {
		if err == sql.ErrNoRows {
			return common.NewErrNotFound("AASREPO-CHECKSMREFINAAS-AASNOTFOUND Asset Administration Shell with ID '" + aasIdentifier + "' not found")
		}
		return common.NewInternalServerError("AASREPO-CHECKSMREFINAAS-GETAASDBID " + err.Error())
	}

	dialect := goqu.Dialect("postgres")
	sqlQuery, args, err := buildCheckAssetAdministrationShellSubmodelReferenceExistsQuery(&dialect, aasDBID, submodelIdentifier)
	if err != nil {
		return common.NewInternalServerError("AASREPO-CHECKSMREFINAAS-BUILDEXISTSSQL " + err.Error())
	}

	var submodelReferenceExists int
	if err := tx.QueryRow(sqlQuery, args...).Scan(&submodelReferenceExists); err != nil {
		if err == sql.ErrNoRows {
			return common.NewErrNotFound("AASREPO-CHECKSMREFINAAS-SMREFNOTFOUND Submodel reference to Submodel with ID '" + submodelIdentifier + "' not found in Asset Administration Shell with ID '" + aasIdentifier + "'")
		}
		return common.NewInternalServerError("AASREPO-CHECKSMREFINAAS-EXECEXISTSSQL " + err.Error())
	}

	return nil
}

// GetAssetAdministrationShells returns a paginated list of AAS representations and the next cursor.
func (s *AssetAdministrationShellDatabase) GetAssetAdministrationShells(ctx context.Context, limit int32, cursor string, idShort string, assetIDs []string) ([]map[string]any, string, error) {
	dialect := goqu.Dialect("postgres")

	if limit < 0 {
		return nil, "", common.NewErrBadRequest("AASREPO-GETAASLIST-BADLIMIT Limit " + string(limit) + " too small")
	}

	sqlQuery, args, err := buildGetAssetAdministrationShellsQuery(&dialect, limit, cursor, idShort, assetIDs)
	if err != nil {
		return nil, "", common.NewInternalServerError("AASREPO-GETAASLIST-BUILDSQL " + err.Error())
	}

	rows, err := s.db.Query(sqlQuery, args...)
	if err != nil {
		return nil, "", common.NewInternalServerError("AASREPO-GETAASLIST-EXECSQL " + err.Error())
	}
	defer func() {
		_ = rows.Close()
	}()

	aasIDs := make([]int64, 0, limit+1)
	for rows.Next() {
		var aasID int64
		if scanErr := rows.Scan(&aasID); scanErr != nil {
			return nil, "", common.NewInternalServerError("AASREPO-GETAASLIST-SCANROW " + scanErr.Error())
		}
		aasIDs = append(aasIDs, aasID)
	}
	if rowsErr := rows.Err(); rowsErr != nil {
		return nil, "", common.NewInternalServerError("AASREPO-GETAASLIST-ITERROWS " + rowsErr.Error())
	}

	nextCursor := ""
	if limit > 0 && len(aasIDs) > int(limit) {
		nextID := aasIDs[len(aasIDs)-1]
		aasIDs = aasIDs[:len(aasIDs)-1]

		cursorSQL, cursorArgs, cursorBuildErr := buildGetAssetAdministrationShellCursorByDBIDQuery(&dialect, nextID)
		if cursorBuildErr != nil {
			return nil, "", common.NewInternalServerError("AASREPO-GETAASLIST-BUILDCURSORSQL " + cursorBuildErr.Error())
		}
		if queryErr := s.db.QueryRow(cursorSQL, cursorArgs...).Scan(&nextCursor); queryErr != nil {
			return nil, "", common.NewInternalServerError("AASREPO-GETAASLIST-GETCURSOR " + queryErr.Error())
		}
	}

	result := make([]map[string]any, 0, len(aasIDs))
	if len(aasIDs) > 0 {
		result, err = s.getAssetAdministrationShellMapsByDBIDs(ctx, aasIDs)
		if err != nil {
			return nil, "", err
		}
	}

	return result, nextCursor, nil
}

// GetAssetAdministrationShellByID returns the JSON-like representation of an AAS by identifier.
func (s *AssetAdministrationShellDatabase) GetAssetAdministrationShellByID(ctx context.Context, aasIdentifier string) (map[string]any, error) {
	dialect := goqu.Dialect("postgres")
	sqlQuery, args, err := buildGetAssetAdministrationShellDBIDByIdentifierQuery(&dialect, aasIdentifier)
	if err != nil {
		return nil, common.NewInternalServerError("AASREPO-GETAASBYID-BUILDSQL " + err.Error())
	}

	var aasDBID int64
	if queryErr := s.db.QueryRow(sqlQuery, args...).Scan(&aasDBID); queryErr != nil {
		if queryErr == sql.ErrNoRows {
			return nil, common.NewErrNotFound("AASREPO-GETAASBYID-AASNOTFOUND Asset Administration Shell with ID '" + aasIdentifier + "' not found")
		}
		return nil, common.NewInternalServerError("AASREPO-GETAASBYID-EXECSQL " + queryErr.Error())
	}

	return s.getAssetAdministrationShellMapByDBID(ctx, aasDBID)
}

// PutAssetAdministrationShellByID upserts an AAS by identifier and returns whether an existing entry was updated.
func (s *AssetAdministrationShellDatabase) PutAssetAdministrationShellByID(aasIdentifier string, aas types.IAssetAdministrationShell) (bool, error) {
	if aasIdentifier != aas.ID() {
		return false, common.NewErrBadRequest("AASREPO-PUTAAS-IDMISMATCH Asset Administration Shell ID in path and body do not match")
	}

	if err := s.verifyAssetAdministrationShell(aas, "AASREPO-PUTAAS-VERIFY"); err != nil {
		return false, err
	}

	tx, cleanup, err := common.StartTransaction(s.db)
	if err != nil {
		return false, common.NewInternalServerError("AASREPO-PUTAAS-STARTTX " + err.Error())
	}
	defer cleanup(&err)

	dialect := goqu.Dialect("postgres")
	selectSQL, selectArgs, buildErr := buildGetAssetAdministrationShellDBIDByIdentifierQuery(&dialect, aasIdentifier)
	if buildErr != nil {
		return false, common.NewInternalServerError("AASREPO-PUTAAS-BUILDSELECT " + buildErr.Error())
	}

	var existingID int64
	isUpdate := true
	if scanErr := tx.QueryRow(selectSQL, selectArgs...).Scan(&existingID); scanErr != nil {
		if scanErr != sql.ErrNoRows {
			return false, common.NewInternalServerError("AASREPO-PUTAAS-EXECSELECT " + scanErr.Error())
		}
		isUpdate = false
	}

	if isUpdate {
		deleteSQL, deleteArgs, deleteBuildErr := buildDeleteAssetAdministrationShellByDBIDQuery(&dialect, existingID)
		if deleteBuildErr != nil {
			return false, common.NewInternalServerError("AASREPO-PUTAAS-BUILDDELETE " + deleteBuildErr.Error())
		}
		if _, deleteErr := tx.Exec(deleteSQL, deleteArgs...); deleteErr != nil {
			return false, common.NewInternalServerError("AASREPO-PUTAAS-EXECDELETE " + deleteErr.Error())
		}
	}

	err = s.createAssetAdministrationShellInTransaction(tx, aas)
	if err != nil {
		return false, err
	}

	err = tx.Commit()
	if err != nil {
		return false, common.NewInternalServerError("AASREPO-PUTAAS-COMMIT " + err.Error())
	}

	return isUpdate, nil
}

// DeleteAssetAdministrationShellByID removes an AAS by identifier.
func (s *AssetAdministrationShellDatabase) DeleteAssetAdministrationShellByID(aasIdentifier string) error {
	tx, cleanup, err := common.StartTransaction(s.db)
	if err != nil {
		return common.NewInternalServerError("AASREPO-DELAAS-STARTTX " + err.Error())
	}
	defer cleanup(&err)

	dialect := goqu.Dialect("postgres")
	sqlQuery, args, buildErr := buildDeleteAssetAdministrationShellByIdentifierQuery(&dialect, aasIdentifier)
	if buildErr != nil {
		return common.NewInternalServerError("AASREPO-DELAAS-BUILDSQL " + buildErr.Error())
	}

	result, execErr := tx.Exec(sqlQuery, args...)
	if execErr != nil {
		return common.NewInternalServerError("AASREPO-DELAAS-EXECSQL " + execErr.Error())
	}

	rowsAffected, rowsErr := result.RowsAffected()
	if rowsErr != nil {
		return common.NewInternalServerError("AASREPO-DELAAS-GETROWCOUNT " + rowsErr.Error())
	}

	if rowsAffected == 0 {
		return common.NewErrNotFound("AASREPO-DELAAS-AASNOTFOUND Asset Administration Shell with ID '" + aasIdentifier + "' not found")
	}

	err = tx.Commit()
	if err != nil {
		return common.NewInternalServerError("AASREPO-DELAAS-COMMIT " + err.Error())
	}

	return nil
}

// GetAssetAdministrationShellReferences returns paginated model references for AAS entries.
func (s *AssetAdministrationShellDatabase) GetAssetAdministrationShellReferences(limit int32, cursor string, idShort string, assetIDs []string) ([]types.IReference, string, error) {
	aasMaps, nextCursor, err := s.GetAssetAdministrationShells(context.Background(), limit, cursor, idShort, assetIDs)
	if err != nil {
		return nil, "", err
	}

	references := make([]types.IReference, 0, len(aasMaps))
	for _, aasMap := range aasMaps {
		aasID, _ := aasMap["id"].(string)
		key := types.NewKey(types.KeyTypesAssetAdministrationShell, aasID)
		references = append(references, types.NewReference(types.ReferenceTypesModelReference, []types.IKey{key}))
	}

	return references, nextCursor, nil
}

// GetAssetAdministrationShellReferenceByID returns the model reference for an AAS identifier.
func (s *AssetAdministrationShellDatabase) GetAssetAdministrationShellReferenceByID(aasIdentifier string) (types.IReference, error) {
	_, err := s.GetAssetAdministrationShellByID(context.Background(), aasIdentifier)
	if err != nil {
		return nil, err
	}

	key := types.NewKey(types.KeyTypesAssetAdministrationShell, aasIdentifier)
	return types.NewReference(types.ReferenceTypesModelReference, []types.IKey{key}), nil
}

// GetAssetInformationByAASID returns the assetInformation section of an AAS by identifier.
func (s *AssetAdministrationShellDatabase) GetAssetInformationByAASID(aasIdentifier string) (map[string]any, error) {
	aasMap, err := s.GetAssetAdministrationShellByID(context.Background(), aasIdentifier)
	if err != nil {
		return nil, err
	}

	assetInformation, ok := aasMap["assetInformation"].(map[string]any)
	if !ok {
		return nil, common.NewErrNotFound("AASREPO-GETASSETINFO-NOTFOUND Asset Information for Asset Administration Shell with ID '" + aasIdentifier + "' not found")
	}

	return assetInformation, nil
}

// PutAssetInformationByAASID updates the assetInformation section of an existing AAS.
func (s *AssetAdministrationShellDatabase) PutAssetInformationByAASID(aasIdentifier string, assetInformation types.IAssetInformation) error {
	if err := s.verifyAssetInformation(assetInformation, "AASREPO-PUTASSETINFORMATION-VERIFY"); err != nil {
		return err
	}

	tx, cleanup, err := common.StartTransaction(s.db)
	if err != nil {
		return common.NewInternalServerError("AASREPO-PUTASSETINFO-STARTTX " + err.Error())
	}
	defer cleanup(&err)

	aasDBID, err := persistenceutils.GetAssetAdministrationShellDatabaseID(tx, aasIdentifier)
	if err != nil {
		if err == sql.ErrNoRows {
			return common.NewErrNotFound("AASREPO-PUTASSETINFO-AASNOTFOUND Asset Administration Shell with ID '" + aasIdentifier + "' not found")
		}
		return common.NewInternalServerError("AASREPO-PUTASSETINFO-GETAASDBID " + err.Error())
	}

	dialect := goqu.Dialect("postgres")
	currentSQL, currentArgs, currentBuildErr := buildGetAssetInformationCurrentStateQuery(&dialect, aasDBID)
	if currentBuildErr != nil {
		return common.NewInternalServerError("AASREPO-PUTASSETINFO-BUILDCURRENTSQL " + currentBuildErr.Error())
	}

	var currentAssetKind sql.NullInt64
	var currentGlobalAssetID sql.NullString
	var currentAssetType sql.NullString
	if currentErr := tx.QueryRow(currentSQL, currentArgs...).Scan(&currentAssetKind, &currentGlobalAssetID, &currentAssetType); currentErr != nil {
		if currentErr == sql.ErrNoRows {
			return common.NewErrNotFound("AASREPO-PUTASSETINFO-ASSETINFONOTFOUND Asset Information for Asset Administration Shell with ID '" + aasIdentifier + "' not found")
		}
		return common.NewInternalServerError("AASREPO-PUTASSETINFO-EXECCURRENTSQL " + currentErr.Error())
	}

	updatedAssetKind := int64(assetInformation.AssetKind())
	if updatedAssetKind == 0 && currentAssetKind.Valid {
		updatedAssetKind = currentAssetKind.Int64
	}

	updatedGlobalAssetID := assetInformation.GlobalAssetID()
	if updatedGlobalAssetID == nil && currentGlobalAssetID.Valid {
		updatedGlobalAssetID = &currentGlobalAssetID.String
	}

	updatedAssetType := assetInformation.AssetType()
	if updatedAssetType == nil && currentAssetType.Valid {
		updatedAssetType = &currentAssetType.String
	}

	updateSQL, updateArgs, buildErr := buildUpdateAssetInformationQuery(&dialect, aasDBID, goqu.Record{
		"asset_kind":      updatedAssetKind,
		"global_asset_id": updatedGlobalAssetID,
		"asset_type":      updatedAssetType,
	})
	if buildErr != nil {
		return common.NewInternalServerError("AASREPO-PUTASSETINFO-BUILDUPDATESQL " + buildErr.Error())
	}

	result, execErr := tx.Exec(updateSQL, updateArgs...)
	if execErr != nil {
		return common.NewInternalServerError("AASREPO-PUTASSETINFO-EXECUPDATESQL " + execErr.Error())
	}

	rowsAffected, rowsErr := result.RowsAffected()
	if rowsErr != nil {
		return common.NewInternalServerError("AASREPO-PUTASSETINFO-GETROWCOUNT " + rowsErr.Error())
	}
	if rowsAffected == 0 {
		return common.NewErrNotFound("AASREPO-PUTASSETINFO-ASSETINFONOTFOUND Asset Information for Asset Administration Shell with ID '" + aasIdentifier + "' not found")
	}

	if assetInformation.SpecificAssetIDs() != nil {
		deleteSpecificSQL, deleteSpecificArgs, deleteSpecificBuildErr := buildDeleteSpecificAssetIDsByAssetInformationIDQuery(&dialect, aasDBID)
		if deleteSpecificBuildErr != nil {
			return common.NewInternalServerError("AASREPO-PUTASSETINFO-BUILDDELETESPECIFIC " + deleteSpecificBuildErr.Error())
		}
		if _, deleteErr := tx.Exec(deleteSpecificSQL, deleteSpecificArgs...); deleteErr != nil {
			return common.NewInternalServerError("AASREPO-PUTASSETINFO-EXECDELETESPECIFIC " + deleteErr.Error())
		}

		if err = common.CreateSpecificAssetIDForAssetInformation(tx, aasDBID, assetInformation.SpecificAssetIDs()); err != nil {
			return common.NewInternalServerError("AASREPO-PUTASSETINFO-CREATESPECIFICASSETIDS " + err.Error())
		}
	}

	err = tx.Commit()
	if err != nil {
		return common.NewInternalServerError("AASREPO-PUTASSETINFO-COMMIT " + err.Error())
	}

	return nil
}

// GetThumbnailByAASID downloads the thumbnail for an AAS including metadata.
func (s *AssetAdministrationShellDatabase) GetThumbnailByAASID(aasIdentifier string) ([]byte, string, string, string, error) {
	thumbnailHandler, err := NewPostgreSQLThumbnailFileHandler(s.db)
	if err != nil {
		return nil, "", "", "", common.NewInternalServerError("AASREPO-GETTHUMBNAIL-NEWHANDLER " + err.Error())
	}

	return thumbnailHandler.DownloadThumbnailByAASID(aasIdentifier)
}

// PutThumbnailByAASID uploads or replaces the thumbnail for an AAS.
func (s *AssetAdministrationShellDatabase) PutThumbnailByAASID(aasIdentifier string, fileName string, file *os.File) error {
	thumbnailHandler, err := NewPostgreSQLThumbnailFileHandler(s.db)
	if err != nil {
		return common.NewInternalServerError("AASREPO-PUTTHUMBNAIL-NEWHANDLER " + err.Error())
	}

	return thumbnailHandler.UploadThumbnailByAASID(aasIdentifier, fileName, file)
}

// DeleteThumbnailByAASID removes the thumbnail associated with an AAS.
func (s *AssetAdministrationShellDatabase) DeleteThumbnailByAASID(aasIdentifier string) error {
	thumbnailHandler, err := NewPostgreSQLThumbnailFileHandler(s.db)
	if err != nil {
		return common.NewInternalServerError("AASREPO-DELTHUMBNAIL-NEWHANDLER " + err.Error())
	}

	return thumbnailHandler.DeleteThumbnailByAASID(aasIdentifier)
}

// GetAllSubmodelReferencesByAASID returns paginated submodel references for the specified AAS.
func (s *AssetAdministrationShellDatabase) GetAllSubmodelReferencesByAASID(aasIdentifier string, limit int32, cursor string) ([]types.IReference, string, error) {
	tx, cleanup, err := common.StartTransaction(s.db)
	if err != nil {
		return nil, "", common.NewInternalServerError("AASREPO-GETSMREFS-STARTTX " + err.Error())
	}
	defer cleanup(&err)

	if limit < 0 {
		return nil, "", common.NewErrBadRequest("AASREPO-GETSMREFS-BADLIMIT Limit " + string(limit) + " too small")
	}

	cursorID := int64(0)
	if cursor != "" {
		parsedCursor, parseErr := strconv.ParseInt(cursor, 10, 64)
		if parseErr != nil {
			return nil, "", common.NewErrBadRequest("AASREPO-GETSMREFS-BADCURSOR Invalid cursor")
		}
		cursorID = parsedCursor
	}

	aasDBID, err := persistenceutils.GetAssetAdministrationShellDatabaseID(tx, aasIdentifier)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, "", common.NewErrNotFound("AASREPO-GETSMREFS-AASNOTFOUND Asset Administration Shell with ID '" + aasIdentifier + "' not found")
		}
		return nil, "", common.NewInternalServerError("AASREPO-GETSMREFS-GETAASDBID " + err.Error())
	}

	dialect := goqu.Dialect("postgres")
	sqlQuery, args, buildErr := buildGetAllSubmodelReferencesByAASIDQuery(&dialect, aasDBID, limit, cursorID)
	if buildErr != nil {
		return nil, "", common.NewInternalServerError("AASREPO-GETSMREFS-BUILDSQL " + buildErr.Error())
	}

	rows, queryErr := tx.Query(sqlQuery, args...)
	if queryErr != nil {
		return nil, "", common.NewInternalServerError("AASREPO-GETSMREFS-EXECSQL " + queryErr.Error())
	}
	defer func() {
		_ = rows.Close()
	}()

	referenceIDs := make([]int64, 0, limit+1)
	references := make([]types.IReference, 0, limit+1)
	for rows.Next() {
		var referenceID int64
		var payload []byte
		if scanErr := rows.Scan(&referenceID, &payload); scanErr != nil {
			return nil, "", common.NewInternalServerError("AASREPO-GETSMREFS-SCANROW " + scanErr.Error())
		}

		var jsonable any
		if unmarshalErr := json.Unmarshal(payload, &jsonable); unmarshalErr != nil {
			return nil, "", common.NewInternalServerError("AASREPO-GETSMREFS-UNMARSHALPAYLOAD " + unmarshalErr.Error())
		}

		reference, refErr := jsonization.ReferenceFromJsonable(jsonable)
		if refErr != nil {
			return nil, "", common.NewInternalServerError("AASREPO-GETSMREFS-PARSEREFERENCE " + refErr.Error())
		}
		referenceIDs = append(referenceIDs, referenceID)
		references = append(references, reference)
	}

	if rowsErr := rows.Err(); rowsErr != nil {
		return nil, "", common.NewInternalServerError("AASREPO-GETSMREFS-ITERROWS " + rowsErr.Error())
	}

	nextCursor := ""
	if limit > 0 && len(referenceIDs) > int(limit) {
		nextCursor = strconv.FormatInt(referenceIDs[len(referenceIDs)-1], 10)
		references = references[:len(references)-1]
	}

	err = tx.Commit()
	if err != nil {
		return nil, "", common.NewInternalServerError("AASREPO-GETSMREFS-COMMIT " + err.Error())
	}

	return references, nextCursor, nil
}

// DeleteSubmodelReferenceInAssetAdministrationShell removes a submodel reference from the specified AAS.
func (s *AssetAdministrationShellDatabase) DeleteSubmodelReferenceInAssetAdministrationShell(aasIdentifier string, submodelIdentifier string) error {
	tx, cleanup, err := common.StartTransaction(s.db)
	if err != nil {
		return common.NewInternalServerError("AASREPO-DELSMREF-STARTTX " + err.Error())
	}
	defer cleanup(&err)

	err = s.deleteSubmodelReferenceInAssetAdministrationShellInTransaction(tx, aasIdentifier, submodelIdentifier)
	if err != nil {
		return err
	}

	err = tx.Commit()
	if err != nil {
		return common.NewInternalServerError("AASREPO-DELSMREF-COMMIT " + err.Error())
	}

	return nil
}

// deleteSubmodelReferenceInAssetAdministrationShellInTransaction removes a submodel reference within an existing transaction.
func (s *AssetAdministrationShellDatabase) deleteSubmodelReferenceInAssetAdministrationShellInTransaction(tx *sql.Tx, aasIdentifier string, submodelIdentifier string) error {
	aasDBID, err := persistenceutils.GetAssetAdministrationShellDatabaseID(tx, aasIdentifier)
	if err != nil {
		if err == sql.ErrNoRows {
			return common.NewErrNotFound("AASREPO-DELSMREF-AASNOTFOUND Asset Administration Shell with ID '" + aasIdentifier + "' not found")
		}
		return common.NewInternalServerError("AASREPO-DELSMREF-GETAASDBID " + err.Error())
	}

	dialect := goqu.Dialect("postgres")
	findSQL, findArgs, findBuildErr := buildFindSubmodelReferenceIDByAASIDAndSubmodelIdentifierQuery(&dialect, aasDBID, submodelIdentifier)
	if findBuildErr != nil {
		return common.NewInternalServerError("AASREPO-DELSMREF-BUILDFINDSQL " + findBuildErr.Error())
	}

	var referenceID int64
	if scanErr := tx.QueryRow(findSQL, findArgs...).Scan(&referenceID); scanErr != nil {
		if scanErr == sql.ErrNoRows {
			return common.NewErrNotFound("AASREPO-DELSMREF-SMREFNOTFOUND Submodel reference to Submodel with ID '" + submodelIdentifier + "' not found in Asset Administration Shell with ID '" + aasIdentifier + "'")
		}
		return common.NewInternalServerError("AASREPO-DELSMREF-EXECFINDSQL " + scanErr.Error())
	}

	deleteSQL, deleteArgs, deleteBuildErr := buildDeleteSubmodelReferenceByIDQuery(&dialect, referenceID)
	if deleteBuildErr != nil {
		return common.NewInternalServerError("AASREPO-DELSMREF-BUILDDELETESQL " + deleteBuildErr.Error())
	}

	if _, deleteErr := tx.Exec(deleteSQL, deleteArgs...); deleteErr != nil {
		return common.NewInternalServerError("AASREPO-DELSMREF-EXECDELETESQL " + deleteErr.Error())
	}

	return nil
}

// nolint:revive // cyclomatic complexity of 32
// getAssetAdministrationShellMapByDBID loads an AAS and maps it to the API JSON representation.
func (s *AssetAdministrationShellDatabase) getAssetAdministrationShellMapByDBID(ctx context.Context, aasDBID int64) (map[string]any, error) {
	dialect := goqu.Dialect("postgres")
	querySQL, queryArgs, buildErr := buildGetAssetAdministrationShellMapByDBIDQuery(&dialect, aasDBID)
	if buildErr != nil {
		return nil, common.NewInternalServerError("AASREPO-MAPAAS-BUILDSQL " + buildErr.Error())
	}

	var aasID string
	var idShort sql.NullString
	var category sql.NullString
	var displayNamePayload []byte
	var descriptionPayload []byte
	var administrationPayload []byte
	var edsPayload []byte
	var extensionsPayload []byte
	var derivedFromPayload []byte
	var assetKind sql.NullInt64
	var globalAssetID sql.NullString
	var assetType sql.NullString
	var thumbnailPath sql.NullString
	var thumbnailContentType sql.NullString

	if queryErr := s.db.QueryRow(querySQL, queryArgs...).Scan(
		&aasID,
		&idShort,
		&category,
		&displayNamePayload,
		&descriptionPayload,
		&administrationPayload,
		&edsPayload,
		&extensionsPayload,
		&derivedFromPayload,
		&assetKind,
		&globalAssetID,
		&assetType,
		&thumbnailPath,
		&thumbnailContentType,
	); queryErr != nil {
		if queryErr == sql.ErrNoRows {
			return nil, common.NewErrNotFound("AASREPO-MAPAAS-AASNOTFOUND Asset Administration Shell not found")
		}
		return nil, common.NewInternalServerError("AASREPO-MAPAAS-EXECSQL " + queryErr.Error())
	}

	result := map[string]any{
		"id":        aasID,
		"modelType": "AssetAdministrationShell",
	}
	if idShort.Valid && idShort.String != "" {
		result["idShort"] = idShort.String
	}
	if category.Valid && category.String != "" {
		result["category"] = category.String
	}

	if assignErr := assignJSONPayload(result, "displayName", displayNamePayload); assignErr != nil {
		return nil, assignErr
	}
	if assignErr := assignJSONPayload(result, "description", descriptionPayload); assignErr != nil {
		return nil, assignErr
	}
	if assignErr := assignJSONPayload(result, "administration", administrationPayload); assignErr != nil {
		return nil, assignErr
	}
	if assignErr := assignJSONPayload(result, "embeddedDataSpecifications", edsPayload); assignErr != nil {
		return nil, assignErr
	}
	if assignErr := assignJSONPayload(result, "extensions", extensionsPayload); assignErr != nil {
		return nil, assignErr
	}
	if assignErr := assignJSONPayload(result, "derivedFrom", derivedFromPayload); assignErr != nil {
		return nil, assignErr
	}

	assetInfo := map[string]any{}
	if assetKind.Valid {
		assetKindString, ok := stringification.AssetKindToString(types.AssetKind(assetKind.Int64))
		if ok {
			assetInfo["assetKind"] = assetKindString
		}
	}
	if globalAssetID.Valid && globalAssetID.String != "" {
		assetInfo["globalAssetId"] = globalAssetID.String
	}
	if assetType.Valid && assetType.String != "" {
		assetInfo["assetType"] = assetType.String
	}
	if thumbnailMap := buildThumbnailMap(thumbnailPath, thumbnailContentType); len(thumbnailMap) > 0 {
		assetInfo["defaultThumbnail"] = thumbnailMap
	}

	specificAssetIDs, specificErr := s.readSpecificAssetIDsByAssetInformationID(ctx, aasDBID)
	if specificErr != nil {
		return nil, common.NewInternalServerError("AASREPO-MAPAAS-READSPECIFICASSETIDS " + specificErr.Error())
	}
	if len(specificAssetIDs) > 0 {
		jsonSpecific := make([]map[string]any, 0, len(specificAssetIDs))
		for _, specificAssetID := range specificAssetIDs {
			jsonableSpecific, jsonErr := jsonization.ToJsonable(specificAssetID)
			if jsonErr != nil {
				return nil, common.NewInternalServerError("AASREPO-MAPAAS-JSONIZESPECIFICASSETID " + jsonErr.Error())
			}
			jsonSpecific = append(jsonSpecific, jsonableSpecific)
		}
		assetInfo["specificAssetIds"] = jsonSpecific
	}

	if len(assetInfo) > 0 {
		result["assetInformation"] = assetInfo
	}

	submodelSQL, submodelArgs, submodelBuildErr := buildGetSubmodelReferencePayloadsByAASIDQuery(&dialect, aasDBID)
	if submodelBuildErr != nil {
		return nil, common.NewInternalServerError("AASREPO-MAPAAS-BUILDSMREFSQL " + submodelBuildErr.Error())
	}

	rows, submodelQueryErr := s.db.Query(submodelSQL, submodelArgs...)
	if submodelQueryErr != nil {
		return nil, common.NewInternalServerError("AASREPO-MAPAAS-EXECSMREFSQL " + submodelQueryErr.Error())
	}
	defer func() {
		_ = rows.Close()
	}()

	submodels := make([]map[string]any, 0)
	for rows.Next() {
		var payload []byte
		if scanErr := rows.Scan(&payload); scanErr != nil {
			return nil, common.NewInternalServerError("AASREPO-MAPAAS-SCANSMREFROW " + scanErr.Error())
		}
		var submodelReference map[string]any
		if unmarshalErr := json.Unmarshal(payload, &submodelReference); unmarshalErr != nil {
			return nil, common.NewInternalServerError("AASREPO-MAPAAS-UNMARSHALSMREF " + unmarshalErr.Error())
		}
		submodels = append(submodels, submodelReference)
	}
	if rowsErr := rows.Err(); rowsErr != nil {
		return nil, common.NewInternalServerError("AASREPO-MAPAAS-ITERSMREFROWS " + rowsErr.Error())
	}
	if len(submodels) > 0 {
		result["submodels"] = submodels
	}

	return result, nil
}

// nolint:revive // cyclomatic complexity of 32
func (s *AssetAdministrationShellDatabase) getAssetAdministrationShellMapsByDBIDs(ctx context.Context, aasDBIDs []int64) ([]map[string]any, error) {
	if len(aasDBIDs) == 0 {
		return []map[string]any{}, nil
	}

	dialect := goqu.Dialect("postgres")
	querySQL, queryArgs, buildErr := buildGetAssetAdministrationShellMapsByDBIDsQuery(&dialect, aasDBIDs)
	if buildErr != nil {
		return nil, common.NewInternalServerError("AASREPO-MAPAASBATCH-BUILDSQL " + buildErr.Error())
	}

	type coreAssetAdministrationShellRow struct {
		aasID                 string
		idShort               sql.NullString
		category              sql.NullString
		displayNamePayload    []byte
		descriptionPayload    []byte
		administrationPayload []byte
		edsPayload            []byte
		extensionsPayload     []byte
		derivedFromPayload    []byte
		assetKind             sql.NullInt64
		globalAssetID         sql.NullString
		assetType             sql.NullString
		thumbnailPath         sql.NullString
		thumbnailContentType  sql.NullString
	}

	rows, queryErr := s.db.QueryContext(ctx, querySQL, queryArgs...)
	if queryErr != nil {
		return nil, common.NewInternalServerError("AASREPO-MAPAASBATCH-EXECSQL " + queryErr.Error())
	}
	defer func() {
		_ = rows.Close()
	}()

	coreRows := make(map[int64]coreAssetAdministrationShellRow, len(aasDBIDs))
	for rows.Next() {
		var aasDBID int64
		var row coreAssetAdministrationShellRow
		if scanErr := rows.Scan(
			&aasDBID,
			&row.aasID,
			&row.idShort,
			&row.category,
			&row.displayNamePayload,
			&row.descriptionPayload,
			&row.administrationPayload,
			&row.edsPayload,
			&row.extensionsPayload,
			&row.derivedFromPayload,
			&row.assetKind,
			&row.globalAssetID,
			&row.assetType,
			&row.thumbnailPath,
			&row.thumbnailContentType,
		); scanErr != nil {
			return nil, common.NewInternalServerError("AASREPO-MAPAASBATCH-SCANROW " + scanErr.Error())
		}
		coreRows[aasDBID] = row
	}
	if rowsErr := rows.Err(); rowsErr != nil {
		return nil, common.NewInternalServerError("AASREPO-MAPAASBATCH-ITERROWS " + rowsErr.Error())
	}

	submodelsByAASID, submodelErr := s.readSubmodelReferencePayloadsByAASDBIDs(ctx, aasDBIDs)
	if submodelErr != nil {
		return nil, submodelErr
	}

	specificAssetIDsByAASID, specificErr := s.readSpecificAssetIDsByAssetInformationIDs(ctx, aasDBIDs)
	if specificErr != nil {
		return nil, common.NewInternalServerError("AASREPO-MAPAASBATCH-READSPECIFICASSETIDS " + specificErr.Error())
	}

	result := make([]map[string]any, 0, len(aasDBIDs))
	for _, aasDBID := range aasDBIDs {
		row, ok := coreRows[aasDBID]
		if !ok {
			continue
		}

		aasMap := map[string]any{
			"id":        row.aasID,
			"modelType": "AssetAdministrationShell",
		}

		if row.idShort.Valid && row.idShort.String != "" {
			aasMap["idShort"] = row.idShort.String
		}
		if row.category.Valid && row.category.String != "" {
			aasMap["category"] = row.category.String
		}

		if assignErr := assignJSONPayload(aasMap, "displayName", row.displayNamePayload); assignErr != nil {
			return nil, assignErr
		}
		if assignErr := assignJSONPayload(aasMap, "description", row.descriptionPayload); assignErr != nil {
			return nil, assignErr
		}
		if assignErr := assignJSONPayload(aasMap, "administration", row.administrationPayload); assignErr != nil {
			return nil, assignErr
		}
		if assignErr := assignJSONPayload(aasMap, "embeddedDataSpecifications", row.edsPayload); assignErr != nil {
			return nil, assignErr
		}
		if assignErr := assignJSONPayload(aasMap, "extensions", row.extensionsPayload); assignErr != nil {
			return nil, assignErr
		}
		if assignErr := assignJSONPayload(aasMap, "derivedFrom", row.derivedFromPayload); assignErr != nil {
			return nil, assignErr
		}

		assetInfo := map[string]any{}
		if row.assetKind.Valid {
			assetKindString, ok := stringification.AssetKindToString(types.AssetKind(row.assetKind.Int64))
			if ok {
				assetInfo["assetKind"] = assetKindString
			}
		}
		if row.globalAssetID.Valid && row.globalAssetID.String != "" {
			assetInfo["globalAssetId"] = row.globalAssetID.String
		}
		if row.assetType.Valid && row.assetType.String != "" {
			assetInfo["assetType"] = row.assetType.String
		}
		if thumbnailMap := buildThumbnailMap(row.thumbnailPath, row.thumbnailContentType); len(thumbnailMap) > 0 {
			assetInfo["defaultThumbnail"] = thumbnailMap
		}

		specificAssetIDs := specificAssetIDsByAASID[aasDBID]
		if len(specificAssetIDs) > 0 {
			jsonSpecific := make([]map[string]any, 0, len(specificAssetIDs))
			for _, specificAssetID := range specificAssetIDs {
				jsonableSpecific, jsonErr := jsonization.ToJsonable(specificAssetID)
				if jsonErr != nil {
					return nil, common.NewInternalServerError("AASREPO-MAPAASBATCH-JSONIZESPECIFICASSETID " + jsonErr.Error())
				}
				jsonSpecific = append(jsonSpecific, jsonableSpecific)
			}
			assetInfo["specificAssetIds"] = jsonSpecific
		}

		if len(assetInfo) > 0 {
			aasMap["assetInformation"] = assetInfo
		}

		submodels := submodelsByAASID[aasDBID]
		if len(submodels) > 0 {
			aasMap["submodels"] = submodels
		}

		result = append(result, aasMap)
	}

	return result, nil
}

func (s *AssetAdministrationShellDatabase) readSubmodelReferencePayloadsByAASDBIDs(ctx context.Context, aasDBIDs []int64) (map[int64][]map[string]any, error) {
	out := make(map[int64][]map[string]any, len(aasDBIDs))
	if len(aasDBIDs) == 0 {
		return out, nil
	}

	dialect := goqu.Dialect("postgres")
	submodelSQL, submodelArgs, submodelBuildErr := buildGetSubmodelReferencePayloadsByAASIDsQuery(&dialect, aasDBIDs)
	if submodelBuildErr != nil {
		return nil, common.NewInternalServerError("AASREPO-READSMREFBATCH-BUILDSQL " + submodelBuildErr.Error())
	}

	rows, submodelQueryErr := s.db.QueryContext(ctx, submodelSQL, submodelArgs...)
	if submodelQueryErr != nil {
		return nil, common.NewInternalServerError("AASREPO-READSMREFBATCH-EXECSQL " + submodelQueryErr.Error())
	}
	defer func() {
		_ = rows.Close()
	}()

	for rows.Next() {
		var aasDBID int64
		var payload []byte
		if scanErr := rows.Scan(&aasDBID, &payload); scanErr != nil {
			return nil, common.NewInternalServerError("AASREPO-READSMREFBATCH-SCANROW " + scanErr.Error())
		}

		var submodelReference map[string]any
		if unmarshalErr := json.Unmarshal(payload, &submodelReference); unmarshalErr != nil {
			return nil, common.NewInternalServerError("AASREPO-READSMREFBATCH-UNMARSHAL " + unmarshalErr.Error())
		}

		out[aasDBID] = append(out[aasDBID], submodelReference)
	}
	if rowsErr := rows.Err(); rowsErr != nil {
		return nil, common.NewInternalServerError("AASREPO-READSMREFBATCH-ITERROWS " + rowsErr.Error())
	}

	return out, nil
}

// assignJSONPayload unmarshals a JSON payload and assigns it to the target map key when present.
func assignJSONPayload(target map[string]any, key string, payload []byte) error {
	if len(payload) == 0 {
		return nil
	}

	var jsonValue any
	if err := json.Unmarshal(payload, &jsonValue); err != nil {
		return common.NewInternalServerError("AASREPO-MAPAAS-UNMARSHALPAYLOAD " + err.Error())
	}

	target[key] = jsonValue
	return nil
}

func buildThumbnailMap(path sql.NullString, contentType sql.NullString) map[string]any {
	if !path.Valid || path.String == "" {
		return nil
	}

	thumbnail := map[string]any{"path": path.String}
	if contentType.Valid && contentType.String != "" {
		thumbnail["contentType"] = contentType.String
	}

	return thumbnail
}

// parseSpecificAssetIDSemanticIDPayload parses an optional SpecificAssetID
// semanticId payload and reports whether parsing produced a semanticId.
func parseSpecificAssetIDSemanticIDPayload(payload []byte) (types.IReference, bool, error) {
	if len(payload) == 0 {
		return nil, false, nil
	}

	var jsonable any
	if err := json.Unmarshal(payload, &jsonable); err != nil {
		return nil, false, err
	}

	if jsonable == nil {
		return nil, false, nil
	}

	if jsonableMap, ok := jsonable.(map[string]any); ok && len(jsonableMap) == 0 {
		return nil, false, nil
	}

	if jsonableSlice, ok := jsonable.([]any); ok && len(jsonableSlice) == 0 {
		return nil, false, nil
	}

	parsedReference, err := jsonization.ReferenceFromJsonable(jsonable)
	if err != nil {
		return nil, false, err
	}

	return parsedReference, true, nil
}

// readSpecificAssetIDsByAssetInformationID reads and enriches specificAssetIds for an assetInformation record.
func (s *AssetAdministrationShellDatabase) readSpecificAssetIDsByAssetInformationID(ctx context.Context, assetInformationID int64) ([]types.ISpecificAssetID, error) {
	dialect := goqu.Dialect("postgres")
	querySQL, queryArgs, buildErr := buildReadSpecificAssetIDsByAssetInformationIDQuery(&dialect, assetInformationID)
	if buildErr != nil {
		return nil, common.NewInternalServerError("AASREPO-READSPECIFIC-BUILDSQL " + buildErr.Error())
	}

	rows, queryErr := s.db.QueryContext(ctx, querySQL, queryArgs...)
	if queryErr != nil {
		return nil, common.NewInternalServerError("AASREPO-READSPECIFIC-EXECSQL " + queryErr.Error())
	}
	defer func() {
		_ = rows.Close()
	}()

	type specificAssetRow struct {
		id                int64
		name              string
		value             string
		semanticIDPayload []byte
	}

	rowData := make([]specificAssetRow, 0)
	ids := make([]int64, 0)
	for rows.Next() {
		var row specificAssetRow
		if scanErr := rows.Scan(&row.id, &row.name, &row.value, &row.semanticIDPayload); scanErr != nil {
			return nil, common.NewInternalServerError("AASREPO-READSPECIFIC-SCANROW " + scanErr.Error())
		}
		rowData = append(rowData, row)
		ids = append(ids, row.id)
	}
	if rowsErr := rows.Err(); rowsErr != nil {
		return nil, common.NewInternalServerError("AASREPO-READSPECIFIC-ITERROWS " + rowsErr.Error())
	}

	if len(rowData) == 0 {
		return []types.ISpecificAssetID{}, nil
	}

	externalSubjectByID, extErr := descriptors.ReadSpecificAssetExternalSubjectReferencesBySpecificAssetIDs(ctx, s.db, ids)
	if extErr != nil {
		return nil, extErr
	}

	supplementalByID, suppErr := descriptors.ReadSpecificAssetSupplementalSemanticReferencesBySpecificAssetIDs(ctx, s.db, ids)
	if suppErr != nil {
		return nil, suppErr
	}

	result := make([]types.ISpecificAssetID, 0, len(rowData))
	for _, row := range rowData {
		specificAssetID := types.NewSpecificAssetID(row.name, row.value)

		semanticID, hasSemanticID, parseErr := parseSpecificAssetIDSemanticIDPayload(row.semanticIDPayload)
		if parseErr != nil {
			return nil, common.NewInternalServerError("AASREPO-READSPECIFIC-PARSESEMANTIC " + parseErr.Error())
		}
		if hasSemanticID {
			specificAssetID.SetSemanticID(semanticID)
		}

		specificAssetID.SetExternalSubjectID(externalSubjectByID[row.id])
		specificAssetID.SetSupplementalSemanticIDs(supplementalByID[row.id])
		result = append(result, specificAssetID)
	}

	return result, nil
}

// readSpecificAssetIDsByAssetInformationIDs reads and enriches specificAssetIds in batch for multiple assetInformation records.
func (s *AssetAdministrationShellDatabase) readSpecificAssetIDsByAssetInformationIDs(ctx context.Context, assetInformationIDs []int64) (map[int64][]types.ISpecificAssetID, error) {
	out := make(map[int64][]types.ISpecificAssetID, len(assetInformationIDs))
	if len(assetInformationIDs) == 0 {
		return out, nil
	}

	dialect := goqu.Dialect("postgres")
	querySQL, queryArgs, buildErr := buildReadSpecificAssetIDsByAssetInformationIDsQuery(&dialect, assetInformationIDs)
	if buildErr != nil {
		return nil, common.NewInternalServerError("AASREPO-READSPECIFICBATCH-BUILDSQL " + buildErr.Error())
	}

	rows, queryErr := s.db.QueryContext(ctx, querySQL, queryArgs...)
	if queryErr != nil {
		return nil, common.NewInternalServerError("AASREPO-READSPECIFICBATCH-EXECSQL " + queryErr.Error())
	}
	defer func() {
		_ = rows.Close()
	}()

	type specificAssetRow struct {
		assetInformationID int64
		id                 int64
		name               string
		value              string
		semanticIDPayload  []byte
	}

	rowData := make([]specificAssetRow, 0)
	ids := make([]int64, 0)
	for rows.Next() {
		var row specificAssetRow
		if scanErr := rows.Scan(&row.assetInformationID, &row.id, &row.name, &row.value, &row.semanticIDPayload); scanErr != nil {
			return nil, common.NewInternalServerError("AASREPO-READSPECIFICBATCH-SCANROW " + scanErr.Error())
		}
		rowData = append(rowData, row)
		ids = append(ids, row.id)
	}
	if rowsErr := rows.Err(); rowsErr != nil {
		return nil, common.NewInternalServerError("AASREPO-READSPECIFICBATCH-ITERROWS " + rowsErr.Error())
	}

	if len(rowData) == 0 {
		return out, nil
	}

	externalSubjectByID, extErr := descriptors.ReadSpecificAssetExternalSubjectReferencesBySpecificAssetIDs(ctx, s.db, ids)
	if extErr != nil {
		return nil, extErr
	}

	supplementalByID, suppErr := descriptors.ReadSpecificAssetSupplementalSemanticReferencesBySpecificAssetIDs(ctx, s.db, ids)
	if suppErr != nil {
		return nil, suppErr
	}

	for _, row := range rowData {
		specificAssetID := types.NewSpecificAssetID(row.name, row.value)

		semanticID, hasSemanticID, parseErr := parseSpecificAssetIDSemanticIDPayload(row.semanticIDPayload)
		if parseErr != nil {
			return nil, common.NewInternalServerError("AASREPO-READSPECIFICBATCH-PARSESEMANTIC " + parseErr.Error())
		}
		if hasSemanticID {
			specificAssetID.SetSemanticID(semanticID)
		}

		specificAssetID.SetExternalSubjectID(externalSubjectByID[row.id])
		specificAssetID.SetSupplementalSemanticIDs(supplementalByID[row.id])
		out[row.assetInformationID] = append(out[row.assetInformationID], specificAssetID)
	}

	return out, nil
}
