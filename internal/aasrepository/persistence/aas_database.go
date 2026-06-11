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
	"crypto/rsa"
	"database/sql"
	"encoding/json"
	"errors"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/FriedJannik/aas-go-sdk/jsonization"
	"github.com/FriedJannik/aas-go-sdk/stringification"
	"github.com/FriedJannik/aas-go-sdk/types"
	"github.com/FriedJannik/aas-go-sdk/verification"
	"github.com/doug-martin/goqu/v9"
	persistenceutils "github.com/eclipse-basyx/basyx-go-components/internal/aasrepository/persistence/utils"
	"github.com/eclipse-basyx/basyx-go-components/internal/common"
	"github.com/eclipse-basyx/basyx-go-components/internal/common/descriptors"
	"github.com/eclipse-basyx/basyx-go-components/internal/common/history"
	"github.com/eclipse-basyx/basyx-go-components/internal/common/jws"
	commonmodel "github.com/eclipse-basyx/basyx-go-components/internal/common/model"
	"github.com/eclipse-basyx/basyx-go-components/internal/common/model/grammar"
	auth "github.com/eclipse-basyx/basyx-go-components/internal/common/security"
	"github.com/lib/pq"
)

// AssetAdministrationShellDatabase is the implementation of the AssetAdministrationShellRepositoryDatabase interface using PostgreSQL as the underlying database.
type AssetAdministrationShellDatabase struct {
	db               *sql.DB
	verificationMode commonmodel.VerificationMode
	privateKey       *rsa.PrivateKey
	signingOptions   jws.SigningOptions
}

type aasDBQueryer interface {
	Query(query string, args ...any) (*sql.Rows, error)
	QueryContext(ctx context.Context, query string, args ...any) (*sql.Rows, error)
	QueryRowContext(ctx context.Context, query string, args ...any) *sql.Row
}

// SetJWSPrivateKey configures the key used by signed AAS responses.
func (s *AssetAdministrationShellDatabase) SetJWSPrivateKey(privateKey *rsa.PrivateKey) {
	s.privateKey = privateKey
}

// SetJWSCertificateChain configures the optional certificate chain embedded in
// signed Asset Administration Shell responses.
//
// The provided slice must already be formatted for the JWS "x5c" protected
// header: each entry is a base64 encoded DER certificate, ordered from signer
// certificate to issuer certificates. Passing nil or an empty slice disables the
// "x5c" header while still allowing signed responses when a private key is
// configured.
//
// Parameters:
//   - certificateChain: Base64 encoded DER certificate chain for the JWS "x5c"
//     protected header.
//
// Returns:
//   - None.
func (s *AssetAdministrationShellDatabase) SetJWSCertificateChain(certificateChain []string) {
	s.signingOptions.CertificateChain = certificateChain
}

// ExecuteInTransaction runs fn in a database transaction bound to this backend.
func (s *AssetAdministrationShellDatabase) ExecuteInTransaction(startErrorCode string, commitErrorCode string, fn func(tx *sql.Tx) error) error {
	return common.ExecuteInTransaction(s.db, startErrorCode, commitErrorCode, fn)
}

// NewAssetAdministrationShellDatabase creates a new instance of AssetAdministrationShellDatabase with the provided database connection.
func NewAssetAdministrationShellDatabase(dsn string, maxOpenConnections int, maxIdleConnections int, connMaxLifetimeMinutes int, strictVerification string) (*AssetAdministrationShellDatabase, error) {
	db, err := common.NewDatabaseConnection(dsn)
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

	return NewAssetAdministrationShellDatabaseFromDB(db, strictVerification)
}

// NewAssetAdministrationShellDatabaseFromDB creates a new repository backend from an existing DB pool.
func NewAssetAdministrationShellDatabaseFromDB(db *sql.DB, strictVerification string) (*AssetAdministrationShellDatabase, error) {
	if db == nil {
		return nil, common.NewErrBadRequest("AASREPO-NEWFROMDB-NILDB database handle must not be nil")
	}

	verificationMode, err := commonmodel.ParseVerificationMode(strictVerification)
	if err != nil {
		return nil, common.NewErrBadRequest("AASREPO-NEWFROMDB-INVALIDMODE " + err.Error())
	}

	return &AssetAdministrationShellDatabase{
		db:               db,
		verificationMode: verificationMode,
	}, nil
}

// verifyAssetAdministrationShell validates an AAS when strict verification is enabled.
func (s *AssetAdministrationShellDatabase) verifyAssetAdministrationShell(aas types.IAssetAdministrationShell, errorPrefix string) error {
	return commonmodel.ValidateWithMode(
		s.verificationMode,
		errorPrefix,
		func(collector func(*verification.VerificationError) bool) {
			verification.VerifyAssetAdministrationShell(aas, collector)
		},
		func(message string) error {
			return common.NewErrBadRequest(errorPrefix + " " + message)
		},
	)
}

// verifyAssetInformation validates an AssetInformation when strict verification is enabled.
func (s *AssetAdministrationShellDatabase) verifyAssetInformation(asset_information types.IAssetInformation, errorPrefix string) error {
	return commonmodel.ValidateWithMode(
		s.verificationMode,
		errorPrefix,
		func(collector func(*verification.VerificationError) bool) {
			verification.VerifyAssetInformation(asset_information, collector)
		},
		func(message string) error {
			return common.NewErrBadRequest(errorPrefix + " " + message)
		},
	)
}

// verifyReference validates a Reference when strict verification is enabled.
func (s *AssetAdministrationShellDatabase) verifyReference(reference types.IReference, errorPrefix string) error {
	return commonmodel.ValidateWithMode(
		s.verificationMode,
		errorPrefix,
		func(collector func(*verification.VerificationError) bool) {
			verification.VerifyReference(reference, collector)
		},
		func(message string) error {
			return common.NewErrBadRequest(errorPrefix + " " + message)
		},
	)
}

func aasToHistorySnapshot(aas types.IAssetAdministrationShell) (map[string]any, error) {
	jsonable, err := jsonization.ToJsonable(aas)
	if err != nil {
		return nil, common.NewInternalServerError("AASREPO-HISTORY-TOJSONABLE " + err.Error())
	}
	return jsonable, nil
}

func (s *AssetAdministrationShellDatabase) appendAASHistoryTx(ctx context.Context, tx *sql.Tx, aas types.IAssetAdministrationShell, changeType string, deleted bool) error {
	snapshot, err := aasToHistorySnapshot(aas)
	if err != nil {
		return err
	}
	return history.AppendVersionTx(ctx, tx, history.TableAAS, aas.ID(), changeType, snapshot, deleted)
}

func (s *AssetAdministrationShellDatabase) appendCurrentAASHistoryTx(ctx context.Context, tx *sql.Tx, aasIdentifier string, changeType string) error {
	aasDBID, err := persistenceutils.GetAssetAdministrationShellDatabaseID(tx, aasIdentifier)
	if err != nil {
		if err == sql.ErrNoRows {
			return common.NewErrNotFound("AASREPO-HISTORY-AASNOTFOUND Asset Administration Shell with ID '" + aasIdentifier + "' not found")
		}
		return common.NewInternalServerError("AASREPO-HISTORY-GETAASDBID " + err.Error())
	}

	aas, err := s.getAssetAdministrationShellMapByDBIDInTransaction(ctx, tx, aasDBID)
	if err != nil {
		return err
	}
	return s.appendAASHistoryTx(ctx, tx, aas, changeType, false)
}

// GetSignedAssetAdministrationShell returns a compact JWS for the requested
// Asset Administration Shell.
//
// The method loads the AAS through the normal repository read path, so
// visibility and ABAC rules from ctx are preserved. The model is converted to
// its JSON representation, canonicalized for stable payload bytes, and signed
// with RS256. The compact JWS contains the BaSyx/IDTA protected headers
// generated by the common JWS signer, including "typ", "sigT", "sid", and
// optionally "x5c" when SetJWSCertificateChain was configured.
//
// Parameters:
//   - ctx: Request context carrying configuration, security, and ABAC data.
//   - aasID: Identifier of the Asset Administration Shell to fetch and sign.
//
// Returns:
//   - string: Compact serialized JWS containing the canonical AAS JSON payload.
//   - error: Error when signing is not configured, the AAS cannot be loaded,
//     JSON conversion or canonicalization fails, or JWS signing fails.
func (s *AssetAdministrationShellDatabase) GetSignedAssetAdministrationShell(ctx context.Context, aasID string) (string, error) {
	if s.privateKey == nil {
		return "", errors.New("JWS signing not configured: private key not loaded")
	}
	aas, err := s.GetAssetAdministrationShellByID(ctx, aasID)
	if err != nil {
		return "", err
	}
	jsonAAS, err := jsonization.ToJsonable(aas)
	if err != nil {
		return "", err
	}
	payload, err := common.CanonicalJSON(jsonAAS)
	if err != nil {
		return "", err
	}
	return jws.SignPayloadWithOptions(s.privateKey, payload, s.signingOptions)
}

// GetAssetAdministrationShellByIDAndDate returns the AAS version valid at the requested instant.
func (s *AssetAdministrationShellDatabase) GetAssetAdministrationShellByIDAndDate(ctx context.Context, aasIdentifier string, at time.Time) (types.IAssetAdministrationShell, error) {
	snapshot, err := history.SnapshotByDate(ctx, s.db, history.TableAAS, aasIdentifier, at)
	if err != nil {
		return nil, err
	}
	aas, err := jsonization.AssetAdministrationShellFromJsonable(snapshot)
	if err != nil {
		return nil, common.NewInternalServerError("AASREPO-HISTORY-FROMJSON " + err.Error())
	}
	return aas, nil
}

// GetAssetAdministrationShellRecentChanges returns AAS history rows for recent-change APIs.
func (s *AssetAdministrationShellDatabase) GetAssetAdministrationShellRecentChanges(ctx context.Context, limit int32, cursor string, createdFrom time.Time, updatedFrom time.Time) ([]history.Row, string, error) {
	shouldEnforce, enforceErr := shouldEnforceFormula(ctx, "AASREPO-RECENT-SHOULDENFORCE")
	if enforceErr != nil {
		return nil, "", enforceErr
	}
	if !shouldEnforce {
		return history.RecentRows(ctx, s.db, history.TableAAS, limit, cursor, createdFrom, updatedFrom)
	}

	collector, err := buildAASCollector()
	if err != nil {
		return nil, "", err
	}
	visibilityDS := goqu.From(goqu.T("aas").As("aas")).
		Select(goqu.V(1)).
		Where(goqu.I("aas.aas_id").Eq(goqu.I("history.identifier")))

	return history.RecentRowsForVisibleIdentifiables(ctx, s.db, history.TableAAS, limit, cursor, createdFrom, updatedFrom, visibilityDS, collector)
}

func buildAASCollector() (*grammar.ResolvedFieldPathCollector, error) {
	collector, err := grammar.NewResolvedFieldPathCollectorForRoot(grammar.CollectorRootAAS)
	if err != nil {
		return nil, common.NewInternalServerError("AASREPO-ABAC-COLLECTOR " + err.Error())
	}

	return collector, nil
}

func buildCoreAssetAdministrationShellSelectExpressions(
	ctx context.Context,
	collector *grammar.ResolvedFieldPathCollector,
	includeDatabaseID bool,
) ([]interface{}, error) {
	columns := make([]auth.FilterColumnSpec, 0, len(unmaskedCoreAssetAdministrationShellSelectExpressions(includeDatabaseID)))
	if includeDatabaseID {
		columns = append(columns, auth.Column(goqu.I("aas.id")))
	}
	columns = append(columns,
		auth.Column(goqu.I("aas.aas_id")),
		auth.MaskedColumn(goqu.I("aas.id_short"), "$aas#idShort"),
		auth.Column(goqu.I("aas.category")),
		auth.Column(goqu.I("ap.displayname_payload")),
		auth.Column(goqu.I("ap.description_payload")),
		auth.Column(goqu.I("ap.administrative_information_payload")),
		auth.Column(goqu.I("ap.embedded_data_specification_payload")),
		auth.Column(goqu.I("ap.extensions_payload")),
		auth.Column(goqu.I("ap.derived_from_payload")),
		auth.Column(goqu.I("asset_information.asset_kind")),
		auth.MaskedColumn(goqu.I("asset_information.global_asset_id"), "$aas#assetInformation.globalAssetId"),
		auth.MaskedColumn(goqu.I("asset_information.asset_type"), "$aas#assetInformation.assetType"),
		auth.Column(goqu.I("tfe.value")),
		auth.Column(goqu.I("tfe.content_type")),
	)

	expressions, err := auth.GetColumnSelectStatement(ctx, columns, collector)
	if err != nil {
		return nil, err
	}

	selectExpressions := make([]interface{}, 0, len(expressions))
	for _, expression := range expressions {
		selectExpressions = append(selectExpressions, expression)
	}
	return selectExpressions, nil
}

func shouldEnforceFormula(ctx context.Context, step string) (bool, error) {
	shouldEnforce, err := auth.ShouldEnforceFormula(ctx)
	if err != nil {
		return false, common.NewInternalServerError(step + " " + err.Error())
	}
	return shouldEnforce, nil
}

func (s *AssetAdministrationShellDatabase) checkAASVisibilityInTx(ctx context.Context, tx *sql.Tx, aasIdentifier string) (bool, bool, error) {
	_, err := persistenceutils.GetAssetAdministrationShellDatabaseID(tx, aasIdentifier)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return false, false, nil
		}
		return false, false, common.NewInternalServerError("AASREPO-ABACCHKAAS-GETAASDBID " + err.Error())
	}

	shouldEnforce, enforceErr := shouldEnforceFormula(ctx, "AASREPO-ABACCHKAAS-SHOULDENFORCE")
	if enforceErr != nil {
		return false, false, enforceErr
	}
	if !shouldEnforce {
		return true, true, nil
	}

	dialect := goqu.Dialect("postgres")
	ds := buildGetAssetAdministrationShellDBIDByIdentifierDataset(&dialect, aasIdentifier)

	collector, collectorErr := buildAASCollector()
	if collectorErr != nil {
		return false, false, collectorErr
	}

	ds, addFormulaErr := auth.AddFormulaQueryFromContext(ctx, ds, collector)
	if addFormulaErr != nil {
		return false, false, common.NewInternalServerError("AASREPO-ABACCHKAAS-ADDFORMULA " + addFormulaErr.Error())
	}

	sqlQuery, args, toSQLErr := ds.ToSQL()
	if toSQLErr != nil {
		return false, false, common.NewInternalServerError("AASREPO-ABACCHKAAS-BUILDSQL " + toSQLErr.Error())
	}

	var aasDBID int64
	scanErr := tx.QueryRowContext(ctx, sqlQuery, args...).Scan(&aasDBID)
	if scanErr == nil {
		return true, true, nil
	}
	if errors.Is(scanErr, sql.ErrNoRows) {
		return true, false, nil
	}

	return false, false, common.NewInternalServerError("AASREPO-ABACCHKAAS-EXECSQL " + scanErr.Error())
}

// CreateAssetAdministrationShell persists a new AAS and performs an ABAC re-check before commit when enabled.
func (s *AssetAdministrationShellDatabase) CreateAssetAdministrationShell(ctx context.Context, aas types.IAssetAdministrationShell) error {
	return common.ExecuteInTransaction(
		s.db,
		"AASREPO-NEWAAS-STARTTX",
		"AASREPO-NEWAAS-CREATE-COMMIT",
		func(tx *sql.Tx) error {
			return s.CreateAssetAdministrationShellInTransaction(ctx, tx, aas)
		},
	)
}

// CreateAssetAdministrationShellInTransaction persists a new AAS within an existing transaction.
func (s *AssetAdministrationShellDatabase) CreateAssetAdministrationShellInTransaction(ctx context.Context, tx *sql.Tx, aas types.IAssetAdministrationShell) error {
	if tx == nil {
		return common.NewInternalServerError("AASREPO-NEWAAS-NILTX transaction must not be nil")
	}

	if err := s.verifyAssetAdministrationShell(aas, "AASREPO-NEWAAS-VERIFY"); err != nil {
		return err
	}

	if err := s.createAssetAdministrationShellInTransaction(tx, aas); err != nil {
		return err
	}

	shouldEnforce, enforceErr := shouldEnforceFormula(ctx, "AASREPO-NEWAAS-SHOULDENFORCE")
	if enforceErr != nil {
		return enforceErr
	}
	if shouldEnforce {
		exists, visible, visErr := s.checkAASVisibilityInTx(ctx, tx, aas.ID())
		if visErr != nil {
			return visErr
		}
		if !exists {
			return common.NewInternalServerError("AASREPO-NEWAAS-ABACCHECKMISSING created AAS not found before commit")
		}
		if !visible {
			return common.NewErrDenied("AASREPO-NEWAAS-ABACDENIED created AAS is not accessible under ABAC constraints")
		}
	}

	return s.appendAASHistoryTx(ctx, tx, aas, history.ChangeCreated, false)
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

	if err := upsertDefaultThumbnailForAssetInformation(tx, &dialect, aasDBID, aas.AssetInformation(), "AASREPO-NEWAAS-CREATE"); err != nil {
		return err
	}

	// specific asset ids
	err = common.CreateSpecificAssetIDForAssetInformation(tx, aasDBID, aas.AssetInformation().SpecificAssetIDs())
	if err != nil {
		return common.NewInternalServerError("AASREPO-NEWAAS-CREATE-CREATESPECIFICASSETIDS " + err.Error())
	}

	// submodel references
	for position, submodelRef := range aas.Submodels() {
		ids, args, err = buildAssetAdministrationShellSubmodelReferenceQuery(&dialect, aasDBID, position, submodelRef)
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

// CreateSubmodelReferenceInAssetAdministrationShell adds a submodel reference with ABAC checks.
func (s *AssetAdministrationShellDatabase) CreateSubmodelReferenceInAssetAdministrationShell(ctx context.Context, aasIdentifier string, submodelRef types.IReference) error {
	return s.createSubmodelReferenceInAssetAdministrationShellWithTransaction(ctx, nil, aasIdentifier, submodelRef)
}

// CreateSubmodelReferenceInAssetAdministrationShellInTransaction adds a submodel reference within an existing transaction.
func (s *AssetAdministrationShellDatabase) CreateSubmodelReferenceInAssetAdministrationShellInTransaction(ctx context.Context, tx *sql.Tx, aasIdentifier string, submodelRef types.IReference) error {
	if tx == nil {
		return common.NewInternalServerError("AASREPO-NEWSMREFINAAS-NILTX transaction must not be nil")
	}

	return s.createSubmodelReferenceInAssetAdministrationShellWithTransaction(ctx, tx, aasIdentifier, submodelRef)
}

func (s *AssetAdministrationShellDatabase) createSubmodelReferenceInAssetAdministrationShellWithTransaction(ctx context.Context, tx *sql.Tx, aasIdentifier string, submodelRef types.IReference) (err error) {
	if err := s.verifyReference(submodelRef, "AASREPO-NEWSMREFINAAS-VERIFY"); err != nil {
		return err
	}

	if tx == nil {
		return s.ExecuteInTransaction(
			"AASREPO-NEWSMREFINAAS-STARTTX",
			"AASREPO-NEWSMREFINAAS-COMMIT",
			func(tx *sql.Tx) error {
				return s.createSubmodelReferenceInAssetAdministrationShellWithinTransaction(ctx, tx, aasIdentifier, submodelRef)
			},
		)
	}

	return s.createSubmodelReferenceInAssetAdministrationShellWithinTransaction(ctx, tx, aasIdentifier, submodelRef)
}

func (s *AssetAdministrationShellDatabase) createSubmodelReferenceInAssetAdministrationShellWithinTransaction(ctx context.Context, tx *sql.Tx, aasIdentifier string, submodelRef types.IReference) error {
	if tx == nil {
		return common.NewInternalServerError("AASREPO-NEWSMREFINAAS-NILTX transaction must not be nil")
	}

	shouldEnforce, enforceErr := shouldEnforceFormula(ctx, "AASREPO-NEWSMREFINAAS-SHOULDENFORCE")
	if enforceErr != nil {
		return enforceErr
	}
	if shouldEnforce {
		exists, visible, visErr := s.checkAASVisibilityInTx(ctx, tx, aasIdentifier)
		if visErr != nil {
			return visErr
		}
		if !exists {
			return common.NewErrNotFound("AASREPO-NEWSMREFINAAS-AASNOTFOUND Asset Administration Shell with ID '" + aasIdentifier + "' not found")
		}
		if !visible {
			return common.NewErrDenied("AASREPO-NEWSMREFINAAS-ABACDENIED writing to this AAS is not allowed")
		}
	}

	if err := s.createSubmodelReferenceInAssetAdministrationShellInTransaction(tx, aasIdentifier, submodelRef); err != nil {
		return err
	}

	if shouldEnforce {
		exists, visible, visErr := s.checkAASVisibilityInTx(ctx, tx, aasIdentifier)
		if visErr != nil {
			return visErr
		}
		if !exists {
			return common.NewInternalServerError("AASREPO-NEWSMREFINAAS-ABACCHECKMISSING AAS not found before commit")
		}
		if !visible {
			return common.NewErrDenied("AASREPO-NEWSMREFINAAS-ABACDENIED written AAS is not accessible under ABAC constraints")
		}
	}
	return s.appendAddedSubmodelReferenceHistoryTx(ctx, tx, aasIdentifier, submodelRef)
}

func (s *AssetAdministrationShellDatabase) getNextSubmodelReferencePositionInTransaction(tx *sql.Tx, aasDBID int64) (int, error) {
	dialect := goqu.Dialect("postgres")
	sqlQuery, args, buildErr := buildGetNextAssetAdministrationShellSubmodelReferencePositionQuery(&dialect, aasDBID)
	if buildErr != nil {
		return 0, common.NewInternalServerError("AASREPO-NEWSMREFINAAS-BUILDNEXTPOSSQL " + buildErr.Error())
	}

	var nextPosition int
	if queryErr := tx.QueryRow(sqlQuery, args...).Scan(&nextPosition); queryErr != nil {
		return 0, common.NewInternalServerError("AASREPO-NEWSMREFINAAS-EXECNEXTPOSSQL " + queryErr.Error())
	}

	return nextPosition, nil
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

	keys := submodelRef.Keys()
	if len(keys) > 0 {
		submodelIdentifier := keys[0].Value()
		if submodelIdentifier != "" {
			checkErr := s.checkIfSubmodelReferenceExistsInAssetAdministrationShellInTransaction(tx, aasIdentifier, submodelIdentifier)
			if checkErr == nil {
				return common.NewErrConflict("AASREPO-NEWSMREFINAAS-CONFLICT Submodel reference to Submodel with ID '" + submodelIdentifier + "' already exists in Asset Administration Shell with ID '" + aasIdentifier + "'")
			}
			if !common.IsErrNotFound(checkErr) {
				return checkErr
			}
		}
	}

	dialect := goqu.Dialect("postgres")

	nextPosition, nextPositionErr := s.getNextSubmodelReferencePositionInTransaction(tx, aasDBID)
	if nextPositionErr != nil {
		return nextPositionErr
	}

	ids, args, err := buildAssetAdministrationShellSubmodelReferenceQuery(&dialect, aasDBID, nextPosition, submodelRef)
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
	return s.ExecuteInTransaction(
		"AASREPO-CHECKSMREFINAAS-STARTTX",
		"AASREPO-CHECKSMREFINAAS-COMMIT",
		func(tx *sql.Tx) error {
			return s.checkIfSubmodelReferenceExistsInAssetAdministrationShellInTransaction(tx, aasIdentifier, submodelIdentifier)
		},
	)
}

// CheckIfSubmodelReferenceExistsInAssetAdministrationShellInTransaction checks for a submodel reference within an existing transaction.
func (s *AssetAdministrationShellDatabase) CheckIfSubmodelReferenceExistsInAssetAdministrationShellInTransaction(tx *sql.Tx, aasIdentifier string, submodelIdentifier string) error {
	if tx == nil {
		return common.NewInternalServerError("AASREPO-CHECKSMREFINAAS-NILTX transaction must not be nil")
	}
	return s.checkIfSubmodelReferenceExistsInAssetAdministrationShellInTransaction(tx, aasIdentifier, submodelIdentifier)
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

// GetAssetAdministrationShells returns a paginated list of AAS objects and the next cursor.
func (s *AssetAdministrationShellDatabase) GetAssetAdministrationShells(ctx context.Context, limit int32, cursor string, idShort string, assetIDs []string) ([]types.IAssetAdministrationShell, string, error) {
	dialect := goqu.Dialect("postgres")

	if limit < 0 {
		return nil, "", common.NewErrBadRequest("AASREPO-GETAASLIST-BADLIMIT Limit " + strconv.FormatInt(int64(limit), 10) + " too small")
	}
	if cursor != "" {
		cursorExists, cursorErr := s.assetAdministrationShellCursorExists(ctx, &dialect, cursor)
		if cursorErr != nil {
			return nil, "", cursorErr
		}
		if !cursorExists {
			return []types.IAssetAdministrationShell{}, "", nil
		}
	}

	selectDS, err := buildGetAssetAdministrationShellsDataset(&dialect, limit, cursor, idShort, assetIDs)
	if err != nil {
		return nil, "", common.NewInternalServerError("AASREPO-GETAASLIST-BUILDSQL " + err.Error())
	}

	collector, collectorErr := buildAASCollector()
	if collectorErr != nil {
		return nil, "", collectorErr
	}

	shouldEnforce, enforceErr := shouldEnforceFormula(ctx, "AASREPO-GETAASLIST-SHOULDENFORCE")
	if enforceErr != nil {
		return nil, "", enforceErr
	}
	if shouldEnforce {
		selectDS, err = auth.AddFormulaQueryFromContext(ctx, selectDS, collector)
		if err != nil {
			return nil, "", common.NewInternalServerError("AASREPO-GETAASLIST-ABACFORMULA " + err.Error())
		}
	}
	sqlQuery, args, toSQLErr := selectDS.ToSQL()
	if toSQLErr != nil {
		return nil, "", common.NewInternalServerError("AASREPO-GETAASLIST-BUILDSQL " + toSQLErr.Error())
	}

	rows, err := s.db.QueryContext(ctx, sqlQuery, args...)
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

	result := make([]types.IAssetAdministrationShell, 0, len(aasIDs))
	if len(aasIDs) > 0 {
		result, err = s.getAssetAdministrationShellMapsByDBIDs(ctx, aasIDs)
		if err != nil {
			return nil, "", err
		}
	}

	return result, nextCursor, nil
}

func (s *AssetAdministrationShellDatabase) assetAdministrationShellCursorExists(ctx context.Context, dialect *goqu.DialectWrapper, cursor string) (bool, error) {
	query, args, buildErr := dialect.From("aas").Select(goqu.V(1)).Where(goqu.I("aas_id").Eq(cursor)).Limit(1).ToSQL()
	if buildErr != nil {
		return false, common.NewInternalServerError("AASREPO-CHECKAASCURSOR-BUILDSQL " + buildErr.Error())
	}

	var one int
	if queryErr := s.db.QueryRowContext(ctx, query, args...).Scan(&one); queryErr != nil {
		if errors.Is(queryErr, sql.ErrNoRows) {
			return false, nil
		}
		return false, common.NewInternalServerError("AASREPO-CHECKAASCURSOR-EXECSQL " + queryErr.Error())
	}
	return true, nil
}

// GetAssetAdministrationShellByID returns an AAS by identifier.
func (s *AssetAdministrationShellDatabase) GetAssetAdministrationShellByID(ctx context.Context, aasIdentifier string) (types.IAssetAdministrationShell, error) {
	dialect := goqu.Dialect("postgres")
	selectDS := buildGetAssetAdministrationShellDBIDByIdentifierDataset(&dialect, aasIdentifier)

	collector, collectorErr := buildAASCollector()
	if collectorErr != nil {
		return nil, collectorErr
	}

	shouldEnforce, enforceErr := shouldEnforceFormula(ctx, "AASREPO-GETAASBYID-SHOULDENFORCE")
	if enforceErr != nil {
		return nil, enforceErr
	}
	if shouldEnforce {
		var err error
		selectDS, err = auth.AddFormulaQueryFromContext(ctx, selectDS, collector)
		if err != nil {
			return nil, common.NewInternalServerError("AASREPO-GETAASBYID-ABACFORMULA " + err.Error())
		}
	}

	sqlQuery, args, toSQLErr := selectDS.ToSQL()
	if toSQLErr != nil {
		return nil, common.NewInternalServerError("AASREPO-GETAASBYID-BUILDSQL " + toSQLErr.Error())
	}

	var aasDBID int64
	if queryErr := s.db.QueryRowContext(ctx, sqlQuery, args...).Scan(&aasDBID); queryErr != nil {
		if queryErr == sql.ErrNoRows {
			return nil, common.NewErrNotFound("AASREPO-GETAASBYID-AASNOTFOUND Asset Administration Shell with ID '" + aasIdentifier + "' not found")
		}
		return nil, common.NewInternalServerError("AASREPO-GETAASBYID-EXECSQL " + queryErr.Error())
	}

	return s.getAssetAdministrationShellMapByDBID(ctx, aasDBID)
}

// PutAssetAdministrationShellByID upserts an AAS and performs ABAC write checks when enabled.
func (s *AssetAdministrationShellDatabase) PutAssetAdministrationShellByID(ctx context.Context, aasIdentifier string, aas types.IAssetAdministrationShell) (bool, error) {
	if aasIdentifier != aas.ID() {
		return false, common.NewErrBadRequest("AASREPO-PUTAAS-IDMISMATCH Asset Administration Shell ID in path and body do not match")
	}

	if err := s.verifyAssetAdministrationShell(aas, "AASREPO-PUTAAS-VERIFY"); err != nil {
		return false, err
	}

	isUpdate := false
	err := common.ExecuteInTransaction(
		s.db,
		"AASREPO-PUTAAS-STARTTX",
		"AASREPO-PUTAAS-COMMIT",
		func(tx *sql.Tx) error {
			var txErr error
			isUpdate, txErr = s.putAssetAdministrationShellByIDInTransactionValidated(ctx, tx, aasIdentifier, aas)
			return txErr
		},
	)
	if err != nil {
		return false, err
	}

	return isUpdate, nil
}

// PutAssetAdministrationShellByIDInTransaction upserts an AAS using an existing transaction.
func (s *AssetAdministrationShellDatabase) PutAssetAdministrationShellByIDInTransaction(ctx context.Context, tx *sql.Tx, aasIdentifier string, aas types.IAssetAdministrationShell) (bool, error) {
	if tx == nil {
		return false, common.NewInternalServerError("AASREPO-PUTAAS-NILTX transaction must not be nil")
	}
	if aasIdentifier != aas.ID() {
		return false, common.NewErrBadRequest("AASREPO-PUTAAS-IDMISMATCH Asset Administration Shell ID in path and body do not match")
	}
	if err := s.verifyAssetAdministrationShell(aas, "AASREPO-PUTAAS-VERIFY"); err != nil {
		return false, err
	}

	return s.putAssetAdministrationShellByIDInTransactionValidated(ctx, tx, aasIdentifier, aas)
}

func (s *AssetAdministrationShellDatabase) putAssetAdministrationShellByIDInTransactionValidated(ctx context.Context, tx *sql.Tx, aasIdentifier string, aas types.IAssetAdministrationShell) (bool, error) {
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

	shouldEnforce, enforceErr := shouldEnforceFormula(ctx, "AASREPO-PUTAAS-SHOULDENFORCE")
	if enforceErr != nil {
		return false, enforceErr
	}
	if shouldEnforce {
		ctx = auth.SelectPutFormulaByExistence(ctx, isUpdate)
	}

	if shouldEnforce && isUpdate {
		exists, visible, visErr := s.checkAASVisibilityInTx(ctx, tx, aasIdentifier)
		if visErr != nil {
			return false, visErr
		}
		if !exists {
			return false, common.NewErrNotFound("AASREPO-PUTAAS-AASNOTFOUND Asset Administration Shell with ID '" + aasIdentifier + "' not found")
		}
		if !visible {
			return false, common.NewErrDenied("AASREPO-PUTAAS-ABACDENIED existing AAS is not accessible under ABAC constraints")
		}
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

	if err := s.createAssetAdministrationShellInTransaction(tx, aas); err != nil {
		return false, err
	}

	if shouldEnforce {
		exists, visible, visErr := s.checkAASVisibilityInTx(ctx, tx, aasIdentifier)
		if visErr != nil {
			return false, visErr
		}
		if !exists {
			return false, common.NewInternalServerError("AASREPO-PUTAAS-ABACCHECKMISSING written AAS not found before commit")
		}
		if !visible {
			return false, common.NewErrDenied("AASREPO-PUTAAS-ABACDENIED written AAS is not accessible under ABAC constraints")
		}
	}

	changeType := history.ChangeCreated
	if isUpdate {
		changeType = history.ChangeUpdated
	}
	if err := s.appendAASHistoryTx(ctx, tx, aas, changeType, false); err != nil {
		return false, err
	}

	return isUpdate, nil
}

// DeleteAssetAdministrationShellByID removes an AAS and checks ABAC visibility before deletion.
func (s *AssetAdministrationShellDatabase) DeleteAssetAdministrationShellByID(ctx context.Context, aasIdentifier string) error {
	return common.ExecuteInTransaction(
		s.db,
		"AASREPO-DELAAS-STARTTX",
		"AASREPO-DELAAS-COMMIT",
		func(tx *sql.Tx) error {
			return s.DeleteAssetAdministrationShellByIDInTransaction(ctx, tx, aasIdentifier)
		},
	)
}

// DeleteAssetAdministrationShellByIDInTransaction removes an AAS using an existing transaction.
func (s *AssetAdministrationShellDatabase) DeleteAssetAdministrationShellByIDInTransaction(ctx context.Context, tx *sql.Tx, aasIdentifier string) error {
	if tx == nil {
		return common.NewInternalServerError("AASREPO-DELAAS-NILTX transaction must not be nil")
	}

	shouldEnforce, enforceErr := shouldEnforceFormula(ctx, "AASREPO-DELAAS-SHOULDENFORCE")
	if enforceErr != nil {
		return enforceErr
	}
	if shouldEnforce {
		exists, visible, visErr := s.checkAASVisibilityInTx(ctx, tx, aasIdentifier)
		if visErr != nil {
			return visErr
		}
		if !exists {
			return common.NewErrNotFound("AASREPO-DELAAS-AASNOTFOUND Asset Administration Shell with ID '" + aasIdentifier + "' not found")
		}
		if !visible {
			return common.NewErrDenied("AASREPO-DELAAS-ABACDENIED deleting this AAS is not allowed")
		}
	}

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

	return history.AppendVersionTx(ctx, tx, history.TableAAS, aasIdentifier, history.ChangeDeleted, map[string]any{"id": aasIdentifier}, true)
}

// GetAssetAdministrationShellReferences returns paginated model references while preserving ABAC filters from ctx.
func (s *AssetAdministrationShellDatabase) GetAssetAdministrationShellReferences(ctx context.Context, limit int32, cursor string, idShort string, assetIDs []string) ([]types.IReference, string, error) {
	aasList, nextCursor, err := s.GetAssetAdministrationShells(ctx, limit, cursor, idShort, assetIDs)
	if err != nil {
		return nil, "", err
	}

	references := make([]types.IReference, 0, len(aasList))
	for _, aas := range aasList {
		if aas == nil {
			return nil, "", common.NewInternalServerError("AASREPO-GETAASREFS-NILAAS loaded AAS is nil")
		}

		aasID := aas.ID()
		key := types.NewKey(types.KeyTypesAssetAdministrationShell, aasID)
		references = append(references, types.NewReference(types.ReferenceTypesModelReference, []types.IKey{key}))
	}

	return references, nextCursor, nil
}

// GetAssetAdministrationShellReferenceByID returns the model reference for an AAS identifier while preserving ABAC filters from ctx.
func (s *AssetAdministrationShellDatabase) GetAssetAdministrationShellReferenceByID(ctx context.Context, aasIdentifier string) (types.IReference, error) {
	_, err := s.GetAssetAdministrationShellByID(ctx, aasIdentifier)
	if err != nil {
		return nil, err
	}

	key := types.NewKey(types.KeyTypesAssetAdministrationShell, aasIdentifier)
	return types.NewReference(types.ReferenceTypesModelReference, []types.IKey{key}), nil
}

// GetAssetInformationByAASID returns the assetInformation section while preserving ABAC filters from ctx.
func (s *AssetAdministrationShellDatabase) GetAssetInformationByAASID(ctx context.Context, aasIdentifier string) (map[string]any, error) {
	aas, err := s.GetAssetAdministrationShellByID(ctx, aasIdentifier)
	if err != nil {
		return nil, err
	}

	if aas == nil {
		return nil, common.NewInternalServerError("AASREPO-GETASSETINFO-NILAAS loaded AAS is nil")
	}

	assetInformation := aas.AssetInformation()
	if assetInformation == nil {
		return nil, common.NewErrNotFound("AASREPO-GETASSETINFO-NOTFOUND Asset Information for Asset Administration Shell with ID '" + aasIdentifier + "' not found")
	}

	jsonAssetInformation, jsonErr := jsonization.ToJsonable(assetInformation)
	if jsonErr != nil {
		return nil, common.NewInternalServerError("AASREPO-GETASSETINFO-TOJSONABLE " + jsonErr.Error())
	}

	return jsonAssetInformation, nil
}

// PutAssetInformationByAASID updates the assetInformation section and applies ABAC write checks.
// nolint:revive // cyclomatic complexity (31) is acceptable due to the multiple steps and checks involved in this operation.
func (s *AssetAdministrationShellDatabase) PutAssetInformationByAASID(ctx context.Context, aasIdentifier string, assetInformation types.IAssetInformation) error {
	tx, cleanup, err := common.StartTransaction(s.db)
	if err != nil {
		return common.NewInternalServerError("AASREPO-PUTASSETINFO-STARTTX " + err.Error())
	}
	defer cleanup(&err)

	if err = s.PutAssetInformationByAASIDInTransaction(ctx, tx, aasIdentifier, assetInformation); err != nil {
		return err
	}

	err = tx.Commit()
	if err != nil {
		return common.NewInternalServerError("AASREPO-PUTASSETINFO-COMMIT " + err.Error())
	}

	return nil
}

// PutAssetInformationByAASIDInTransaction updates AAS asset information in an existing transaction.
func (s *AssetAdministrationShellDatabase) PutAssetInformationByAASIDInTransaction(
	ctx context.Context,
	tx *sql.Tx,
	aasIdentifier string,
	assetInformation types.IAssetInformation,
) error {
	if tx == nil {
		return common.NewInternalServerError("AASREPO-PUTASSETINFO-NILTX transaction must not be nil")
	}

	if err := s.verifyAssetInformation(assetInformation, "AASREPO-PUTASSETINFORMATION-VERIFY"); err != nil {
		return err
	}

	shouldEnforce, enforceErr := shouldEnforceFormula(ctx, "AASREPO-PUTASSETINFO-SHOULDENFORCE")
	if enforceErr != nil {
		return enforceErr
	}
	if err := s.ensureAASWritableForAssetInformationUpdate(ctx, tx, aasIdentifier, shouldEnforce); err != nil {
		return err
	}

	aasDBID, err := getAASDatabaseIDForAssetInformationUpdate(tx, aasIdentifier)
	if err != nil {
		return err
	}

	dialect := goqu.Dialect("postgres")
	currentState, err := loadCurrentAssetInformationState(tx, &dialect, aasDBID, aasIdentifier)
	if err != nil {
		return err
	}

	if err := updateAssetInformationRecord(tx, &dialect, aasDBID, aasIdentifier, assetInformation, currentState); err != nil {
		return err
	}

	if err := replaceSpecificAssetIDsForAssetInformation(tx, &dialect, aasDBID, assetInformation); err != nil {
		return err
	}

	if err := s.ensureAASVisibleAfterAssetInformationUpdate(ctx, tx, aasIdentifier, shouldEnforce); err != nil {
		return err
	}

	return s.appendAssetInformationHistoryTx(ctx, tx, aasIdentifier, assetInformation)
}

type currentAssetInformationState struct {
	assetKind     sql.NullInt64
	globalAssetID sql.NullString
	assetType     sql.NullString
}

func (s *AssetAdministrationShellDatabase) ensureAASWritableForAssetInformationUpdate(
	ctx context.Context,
	tx *sql.Tx,
	aasIdentifier string,
	shouldEnforce bool,
) error {
	if !shouldEnforce {
		return nil
	}

	exists, visible, visErr := s.checkAASVisibilityInTx(ctx, tx, aasIdentifier)
	if visErr != nil {
		return visErr
	}
	if !exists {
		return common.NewErrNotFound("AASREPO-PUTASSETINFO-AASNOTFOUND Asset Administration Shell with ID '" + aasIdentifier + "' not found")
	}
	if !visible {
		return common.NewErrDenied("AASREPO-PUTASSETINFO-ABACDENIED updating this AAS is not allowed")
	}
	return nil
}

func getAASDatabaseIDForAssetInformationUpdate(tx *sql.Tx, aasIdentifier string) (int64, error) {
	aasDBID, err := persistenceutils.GetAssetAdministrationShellDatabaseID(tx, aasIdentifier)
	if err != nil {
		if err == sql.ErrNoRows {
			return 0, common.NewErrNotFound("AASREPO-PUTASSETINFO-AASNOTFOUND Asset Administration Shell with ID '" + aasIdentifier + "' not found")
		}
		return 0, common.NewInternalServerError("AASREPO-PUTASSETINFO-GETAASDBID " + err.Error())
	}
	return aasDBID, nil
}

func loadCurrentAssetInformationState(
	tx *sql.Tx,
	dialect *goqu.DialectWrapper,
	aasDBID int64,
	aasIdentifier string,
) (currentAssetInformationState, error) {
	currentSQL, currentArgs, currentBuildErr := buildGetAssetInformationCurrentStateQuery(dialect, aasDBID)
	if currentBuildErr != nil {
		return currentAssetInformationState{}, common.NewInternalServerError("AASREPO-PUTASSETINFO-BUILDCURRENTSQL " + currentBuildErr.Error())
	}

	var currentState currentAssetInformationState
	if currentErr := tx.QueryRow(currentSQL, currentArgs...).Scan(
		&currentState.assetKind,
		&currentState.globalAssetID,
		&currentState.assetType,
	); currentErr != nil {
		if currentErr == sql.ErrNoRows {
			return currentAssetInformationState{}, common.NewErrNotFound("AASREPO-PUTASSETINFO-ASSETINFONOTFOUND Asset Information for Asset Administration Shell with ID '" + aasIdentifier + "' not found")
		}
		return currentAssetInformationState{}, common.NewInternalServerError("AASREPO-PUTASSETINFO-EXECCURRENTSQL " + currentErr.Error())
	}
	return currentState, nil
}

func updateAssetInformationRecord(
	tx *sql.Tx,
	dialect *goqu.DialectWrapper,
	aasDBID int64,
	aasIdentifier string,
	assetInformation types.IAssetInformation,
	currentState currentAssetInformationState,
) error {
	updatedAssetKind := int64(assetInformation.AssetKind())
	if updatedAssetKind == 0 && currentState.assetKind.Valid {
		updatedAssetKind = currentState.assetKind.Int64
	}

	updatedGlobalAssetID := assetInformation.GlobalAssetID()
	if updatedGlobalAssetID == nil && currentState.globalAssetID.Valid {
		updatedGlobalAssetID = &currentState.globalAssetID.String
	}

	updatedAssetType := assetInformation.AssetType()
	if updatedAssetType == nil && currentState.assetType.Valid {
		updatedAssetType = &currentState.assetType.String
	}

	updateSQL, updateArgs, buildErr := buildUpdateAssetInformationQuery(dialect, aasDBID, goqu.Record{
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

	return upsertDefaultThumbnailForAssetInformation(tx, dialect, aasDBID, assetInformation, "AASREPO-PUTASSETINFO")
}

func upsertDefaultThumbnailForAssetInformation(
	tx *sql.Tx,
	dialect *goqu.DialectWrapper,
	aasDBID int64,
	assetInformation types.IAssetInformation,
	errorPrefix string,
) error {
	thumbnail, thumbnailPath := defaultThumbnailWithPath(assetInformation)
	if thumbnail == nil {
		return nil
	}

	upsertSQL, upsertArgs, buildErr := buildUpsertDefaultThumbnailQuery(dialect, aasDBID, thumbnail, thumbnailPath)
	if buildErr != nil {
		return common.NewInternalServerError(errorPrefix + "-BUILDTHUMBNAILSQL " + buildErr.Error())
	}

	if _, execErr := tx.Exec(upsertSQL, upsertArgs...); execErr != nil {
		return common.NewInternalServerError(errorPrefix + "-EXECTHUMBNAILSQL " + execErr.Error())
	}

	return nil
}

func defaultThumbnailWithPath(assetInformation types.IAssetInformation) (types.IResource, string) {
	if assetInformation == nil || assetInformation.DefaultThumbnail() == nil {
		return nil, ""
	}

	thumbnail := assetInformation.DefaultThumbnail()
	thumbnailPath := strings.TrimSpace(thumbnail.Path())
	if thumbnailPath == "" {
		return nil, ""
	}

	if !strings.HasPrefix(thumbnailPath, "http://") && !strings.HasPrefix(thumbnailPath, "https://") {
		return nil, ""
	}

	return thumbnail, thumbnailPath
}

func buildUpsertDefaultThumbnailQuery(
	dialect *goqu.DialectWrapper,
	aasDBID int64,
	thumbnail types.IResource,
	thumbnailPath string,
) (string, []any, error) {
	record := goqu.Record{
		"id":           aasDBID,
		"content_type": thumbnail.ContentType(),
		"file_name":    nil,
		"value":        thumbnailPath,
	}

	return dialect.Insert("thumbnail_file_element").
		Rows(record).
		OnConflict(goqu.DoUpdate("id", goqu.Record{
			"content_type": goqu.COALESCE(goqu.I("excluded.content_type"), goqu.I("thumbnail_file_element.content_type")),
			"file_name":    nil,
			"value":        thumbnailPath,
		})).
		ToSQL()
}

func replaceSpecificAssetIDsForAssetInformation(
	tx *sql.Tx,
	dialect *goqu.DialectWrapper,
	aasDBID int64,
	assetInformation types.IAssetInformation,
) error {
	if assetInformation.SpecificAssetIDs() == nil {
		return nil
	}

	deleteSpecificSQL, deleteSpecificArgs, deleteSpecificBuildErr := buildDeleteSpecificAssetIDsByAssetInformationIDQuery(dialect, aasDBID)
	if deleteSpecificBuildErr != nil {
		return common.NewInternalServerError("AASREPO-PUTASSETINFO-BUILDDELETESPECIFIC " + deleteSpecificBuildErr.Error())
	}
	if _, deleteErr := tx.Exec(deleteSpecificSQL, deleteSpecificArgs...); deleteErr != nil {
		return common.NewInternalServerError("AASREPO-PUTASSETINFO-EXECDELETESPECIFIC " + deleteErr.Error())
	}
	if err := common.CreateSpecificAssetIDForAssetInformation(tx, aasDBID, assetInformation.SpecificAssetIDs()); err != nil {
		return common.NewInternalServerError("AASREPO-PUTASSETINFO-CREATESPECIFICASSETIDS " + err.Error())
	}
	return nil
}

func (s *AssetAdministrationShellDatabase) ensureAASVisibleAfterAssetInformationUpdate(
	ctx context.Context,
	tx *sql.Tx,
	aasIdentifier string,
	shouldEnforce bool,
) error {
	if !shouldEnforce {
		return nil
	}

	exists, visible, visErr := s.checkAASVisibilityInTx(ctx, tx, aasIdentifier)
	if visErr != nil {
		return visErr
	}
	if !exists {
		return common.NewInternalServerError("AASREPO-PUTASSETINFO-ABACCHECKMISSING AAS not found before commit")
	}
	if !visible {
		return common.NewErrDenied("AASREPO-PUTASSETINFO-ABACDENIED updated AAS is not accessible under ABAC constraints")
	}
	return nil
}

// GetThumbnailByAASID downloads the thumbnail while preserving ABAC visibility from ctx.
func (s *AssetAdministrationShellDatabase) GetThumbnailByAASID(ctx context.Context, aasIdentifier string) ([]byte, string, string, string, error) {
	if _, err := s.GetAssetAdministrationShellByID(ctx, aasIdentifier); err != nil {
		return nil, "", "", "", err
	}

	thumbnailHandler, err := NewPostgreSQLThumbnailFileHandler(s.db)
	if err != nil {
		return nil, "", "", "", common.NewInternalServerError("AASREPO-GETTHUMBNAIL-NEWHANDLER " + err.Error())
	}

	return thumbnailHandler.DownloadThumbnailByAASID(aasIdentifier)
}

// PutThumbnailByAASID uploads or replaces the thumbnail and checks ABAC visibility.
func (s *AssetAdministrationShellDatabase) PutThumbnailByAASID(ctx context.Context, aasIdentifier string, fileName string, file *os.File) error {
	tx, cleanup, err := common.StartTransaction(s.db)
	if err != nil {
		return common.NewInternalServerError("AASREPO-PUTTHUMBNAIL-STARTTX " + err.Error())
	}
	defer cleanup(&err)

	shouldEnforce, enforceErr := shouldEnforceFormula(ctx, "AASREPO-PUTTHUMBNAIL-SHOULDENFORCE")
	if enforceErr != nil {
		return enforceErr
	}
	if shouldEnforce {
		aasDBID, dbIDErr := persistenceutils.GetAssetAdministrationShellDatabaseID(tx, aasIdentifier)
		if dbIDErr != nil {
			if dbIDErr == sql.ErrNoRows {
				return common.NewErrNotFound("AASREPO-PUTTHUMBNAIL-AASNOTFOUND Asset Administration Shell with ID '" + aasIdentifier + "' not found")
			}
			return common.NewInternalServerError("AASREPO-PUTTHUMBNAIL-GETAASDBID " + dbIDErr.Error())
		}

		dialect := goqu.Dialect("postgres")
		fileQuery, fileArgs, fileBuildErr := dialect.
			From("thumbnail_file_data").
			Select("file_oid").
			Where(
				goqu.C("id").Eq(aasDBID),
				goqu.C("file_oid").IsNotNull(),
			).
			Limit(1).
			ToSQL()
		if fileBuildErr != nil {
			return common.NewInternalServerError("AASREPO-PUTTHUMBNAIL-BUILDEXISTSQL " + fileBuildErr.Error())
		}

		thumbnailExists := true
		var fileOID int64
		if scanErr := tx.QueryRow(fileQuery, fileArgs...).Scan(&fileOID); scanErr != nil {
			if scanErr != sql.ErrNoRows {
				return common.NewInternalServerError("AASREPO-PUTTHUMBNAIL-EXECEXISTSQL " + scanErr.Error())
			}
			thumbnailExists = false
		}

		ctx = auth.SelectPutFormulaByExistence(ctx, thumbnailExists)
		exists, visible, visErr := s.checkAASVisibilityInTx(ctx, tx, aasIdentifier)
		if visErr != nil {
			return visErr
		}
		if !exists {
			return common.NewErrNotFound("AASREPO-PUTTHUMBNAIL-AASNOTFOUND Asset Administration Shell with ID '" + aasIdentifier + "' not found")
		}
		if !visible {
			return common.NewErrDenied("AASREPO-PUTTHUMBNAIL-ABACDENIED updating this AAS is not allowed")
		}
	}

	thumbnailHandler, err := NewPostgreSQLThumbnailFileHandler(s.db)
	if err != nil {
		return common.NewInternalServerError("AASREPO-PUTTHUMBNAIL-NEWHANDLER " + err.Error())
	}

	if uploadErr := thumbnailHandler.uploadThumbnailByAASIDInTransaction(tx, aasIdentifier, fileName, file); uploadErr != nil {
		return uploadErr
	}

	if err = s.appendUploadedThumbnailHistoryTx(ctx, tx, aasIdentifier); err != nil {
		return err
	}

	err = tx.Commit()
	if err != nil {
		return common.NewInternalServerError("AASREPO-PUTTHUMBNAIL-COMMIT " + err.Error())
	}

	return nil
}

// DeleteThumbnailByAASID removes the thumbnail and checks ABAC visibility.
func (s *AssetAdministrationShellDatabase) DeleteThumbnailByAASID(ctx context.Context, aasIdentifier string) error {
	tx, cleanup, err := common.StartTransaction(s.db)
	if err != nil {
		return common.NewInternalServerError("AASREPO-DELTHUMBNAIL-STARTTX " + err.Error())
	}
	defer cleanup(&err)

	shouldEnforce, enforceErr := shouldEnforceFormula(ctx, "AASREPO-DELTHUMBNAIL-SHOULDENFORCE")
	if enforceErr != nil {
		return enforceErr
	}
	if shouldEnforce {
		exists, visible, visErr := s.checkAASVisibilityInTx(ctx, tx, aasIdentifier)
		if visErr != nil {
			return visErr
		}
		if !exists {
			return common.NewErrNotFound("AASREPO-DELTHUMBNAIL-AASNOTFOUND Asset Administration Shell with ID '" + aasIdentifier + "' not found")
		}
		if !visible {
			return common.NewErrDenied("AASREPO-DELTHUMBNAIL-ABACDENIED deleting this thumbnail is not allowed")
		}
	}

	thumbnailHandler, err := NewPostgreSQLThumbnailFileHandler(s.db)
	if err != nil {
		return common.NewInternalServerError("AASREPO-DELTHUMBNAIL-NEWHANDLER " + err.Error())
	}

	if deleteErr := thumbnailHandler.deleteThumbnailByAASIDInTransaction(tx, aasIdentifier); deleteErr != nil {
		return deleteErr
	}

	if err = s.appendDeletedThumbnailHistoryTx(ctx, tx, aasIdentifier); err != nil {
		return err
	}

	err = tx.Commit()
	if err != nil {
		return common.NewInternalServerError("AASREPO-DELTHUMBNAIL-COMMIT " + err.Error())
	}

	return nil
}

// GetAllSubmodelReferencesByAASID returns paginated submodel references while preserving ABAC visibility from ctx.
func (s *AssetAdministrationShellDatabase) GetAllSubmodelReferencesByAASID(ctx context.Context, aasIdentifier string, limit int32, cursor string) ([]types.IReference, string, error) {
	tx, cleanup, err := common.StartTransaction(s.db)
	if err != nil {
		return nil, "", common.NewInternalServerError("AASREPO-GETSMREFS-STARTTX " + err.Error())
	}
	defer cleanup(&err)

	if limit < 0 {
		return nil, "", common.NewErrBadRequest("AASREPO-GETSMREFS-BADLIMIT Limit " + strconv.FormatInt(int64(limit), 10) + " too small")
	}

	cursorID := int64(0)
	if cursor != "" {
		parsedCursor, parseErr := strconv.ParseInt(cursor, 10, 64)
		if parseErr != nil {
			return nil, "", common.NewErrBadRequest("AASREPO-GETSMREFS-BADCURSOR Invalid cursor")
		}
		cursorID = parsedCursor
	}

	dialect := goqu.Dialect("postgres")
	selectDS := buildGetAssetAdministrationShellDBIDByIdentifierDataset(&dialect, aasIdentifier)

	collector, collectorErr := buildAASCollector()
	if collectorErr != nil {
		return nil, "", collectorErr
	}

	shouldEnforce, enforceErr := shouldEnforceFormula(ctx, "AASREPO-GETSMREFS-SHOULDENFORCE")
	if enforceErr != nil {
		return nil, "", enforceErr
	}
	if shouldEnforce {
		selectDS, err = auth.AddFormulaQueryFromContext(ctx, selectDS, collector)
		if err != nil {
			return nil, "", common.NewInternalServerError("AASREPO-GETSMREFS-ABACFORMULA " + err.Error())
		}
	}

	aasDBIDSQL, aasDBIDArgs, aasDBIDBuildErr := selectDS.ToSQL()
	if aasDBIDBuildErr != nil {
		return nil, "", common.NewInternalServerError("AASREPO-GETSMREFS-BUILDAASSQL " + aasDBIDBuildErr.Error())
	}

	var aasDBID int64
	if queryErr := tx.QueryRowContext(ctx, aasDBIDSQL, aasDBIDArgs...).Scan(&aasDBID); queryErr != nil {
		if queryErr == sql.ErrNoRows {
			return nil, "", common.NewErrNotFound("AASREPO-GETSMREFS-AASNOTFOUND Asset Administration Shell with ID '" + aasIdentifier + "' not found")
		}
		return nil, "", common.NewInternalServerError("AASREPO-GETSMREFS-EXECAASSQL " + queryErr.Error())
	}
	if cursorID > 0 {
		cursorExists, cursorErr := submodelReferenceCursorExists(ctx, tx, &dialect, aasDBID, cursorID)
		if cursorErr != nil {
			return nil, "", cursorErr
		}
		if !cursorExists {
			return []types.IReference{}, "", nil
		}
	}

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

// ListAASIdentifiersBySubmodelID returns all AAS identifiers that reference the given submodel ID.
func (s *AssetAdministrationShellDatabase) ListAASIdentifiersBySubmodelID(ctx context.Context, submodelIdentifier string) ([]string, error) {
	dialect := goqu.Dialect("postgres")
	sqlQuery, args, buildErr := buildListAASIdentifiersBySubmodelIdentifierQuery(&dialect, submodelIdentifier)
	if buildErr != nil {
		return nil, common.NewInternalServerError("AASREPO-LISTAASBYSM-BUILDSQL " + buildErr.Error())
	}

	rows, queryErr := s.db.QueryContext(ctx, sqlQuery, args...)
	if queryErr != nil {
		return nil, common.NewInternalServerError("AASREPO-LISTAASBYSM-EXECSQL " + queryErr.Error())
	}
	defer func() { _ = rows.Close() }()

	aasIDs := make([]string, 0, 16)
	for rows.Next() {
		var aasID string
		if scanErr := rows.Scan(&aasID); scanErr != nil {
			return nil, common.NewInternalServerError("AASREPO-LISTAASBYSM-SCANROW " + scanErr.Error())
		}
		aasIDs = append(aasIDs, aasID)
	}
	if rowsErr := rows.Err(); rowsErr != nil {
		return nil, common.NewInternalServerError("AASREPO-LISTAASBYSM-ITERROWS " + rowsErr.Error())
	}

	return aasIDs, nil
}

// ListAASIdentifiersBySubmodelIDInTransaction returns all AAS identifiers that reference the given submodel ID using the provided transaction.
func (s *AssetAdministrationShellDatabase) ListAASIdentifiersBySubmodelIDInTransaction(ctx context.Context, tx *sql.Tx, submodelIdentifier string) ([]string, error) {
	if tx == nil {
		return nil, common.NewInternalServerError("AASREPO-LISTAASBYSM-NILTX transaction must not be nil")
	}

	dialect := goqu.Dialect("postgres")
	sqlQuery, args, buildErr := buildListAASIdentifiersBySubmodelIdentifierQuery(&dialect, submodelIdentifier)
	if buildErr != nil {
		return nil, common.NewInternalServerError("AASREPO-LISTAASBYSM-BUILDSQL " + buildErr.Error())
	}

	rows, queryErr := tx.QueryContext(ctx, sqlQuery, args...)
	if queryErr != nil {
		return nil, common.NewInternalServerError("AASREPO-LISTAASBYSM-EXECSQL " + queryErr.Error())
	}
	defer func() { _ = rows.Close() }()

	aasIDs := make([]string, 0, 16)
	for rows.Next() {
		var aasID string
		if scanErr := rows.Scan(&aasID); scanErr != nil {
			return nil, common.NewInternalServerError("AASREPO-LISTAASBYSM-SCANROW " + scanErr.Error())
		}
		aasIDs = append(aasIDs, aasID)
	}
	if rowsErr := rows.Err(); rowsErr != nil {
		return nil, common.NewInternalServerError("AASREPO-LISTAASBYSM-ITERROWS " + rowsErr.Error())
	}

	return aasIDs, nil
}

func submodelReferenceCursorExists(ctx context.Context, tx *sql.Tx, dialect *goqu.DialectWrapper, aasDBID int64, cursorID int64) (bool, error) {
	query, args, buildErr := dialect.
		From(goqu.T("aas_submodel_reference").As("r")).
		Select(goqu.V(1)).
		Where(
			goqu.I("r.id").Eq(cursorID),
			goqu.I("r.aas_id").Eq(aasDBID),
		).
		Limit(1).
		ToSQL()
	if buildErr != nil {
		return false, common.NewInternalServerError("AASREPO-CHECKSMREFCURSOR-BUILDSQL " + buildErr.Error())
	}

	var one int
	if queryErr := tx.QueryRowContext(ctx, query, args...).Scan(&one); queryErr != nil {
		if errors.Is(queryErr, sql.ErrNoRows) {
			return false, nil
		}
		return false, common.NewInternalServerError("AASREPO-CHECKSMREFCURSOR-EXECSQL " + queryErr.Error())
	}
	return true, nil
}

// DeleteSubmodelReferenceInAssetAdministrationShell removes a submodel reference and checks ABAC visibility.
func (s *AssetAdministrationShellDatabase) DeleteSubmodelReferenceInAssetAdministrationShell(ctx context.Context, aasIdentifier string, submodelIdentifier string) error {
	return s.deleteSubmodelReferenceInAssetAdministrationShellWithTransaction(ctx, nil, aasIdentifier, submodelIdentifier)
}

// DeleteSubmodelReferenceInAssetAdministrationShellInTransaction removes a submodel reference within an existing transaction.
func (s *AssetAdministrationShellDatabase) DeleteSubmodelReferenceInAssetAdministrationShellInTransaction(ctx context.Context, tx *sql.Tx, aasIdentifier string, submodelIdentifier string) error {
	if tx == nil {
		return common.NewInternalServerError("AASREPO-DELSMREF-NILTX transaction must not be nil")
	}

	return s.deleteSubmodelReferenceInAssetAdministrationShellWithTransaction(ctx, tx, aasIdentifier, submodelIdentifier)
}

func (s *AssetAdministrationShellDatabase) deleteSubmodelReferenceInAssetAdministrationShellWithTransaction(ctx context.Context, tx *sql.Tx, aasIdentifier string, submodelIdentifier string) error {
	if tx == nil {
		return s.ExecuteInTransaction(
			"AASREPO-DELSMREF-STARTTX",
			"AASREPO-DELSMREF-COMMIT",
			func(tx *sql.Tx) error {
				return s.deleteSubmodelReferenceInAssetAdministrationShellWithTransaction(ctx, tx, aasIdentifier, submodelIdentifier)
			},
		)
	}

	shouldEnforce, enforceErr := shouldEnforceFormula(ctx, "AASREPO-DELSMREF-SHOULDENFORCE")
	if enforceErr != nil {
		return enforceErr
	}
	if shouldEnforce {
		exists, visible, visErr := s.checkAASVisibilityInTx(ctx, tx, aasIdentifier)
		if visErr != nil {
			return visErr
		}
		if !exists {
			return common.NewErrNotFound("AASREPO-DELSMREF-AASNOTFOUND Asset Administration Shell with ID '" + aasIdentifier + "' not found")
		}
		if !visible {
			return common.NewErrDenied("AASREPO-DELSMREF-ABACDENIED deleting this submodel reference is not allowed")
		}
	}

	if err := s.deleteSubmodelReferenceInAssetAdministrationShellInTransaction(tx, aasIdentifier, submodelIdentifier); err != nil {
		return err
	}

	return s.appendRemovedSubmodelReferenceHistoryTx(ctx, tx, aasIdentifier, submodelIdentifier)
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

// nolint:revive // cyclomatic complexity of 32
// getAssetAdministrationShellMapByDBID loads an AAS and maps it to a typed model.
func (s *AssetAdministrationShellDatabase) getAssetAdministrationShellMapByDBID(ctx context.Context, aasDBID int64) (types.IAssetAdministrationShell, error) {
	return s.getAssetAdministrationShellMapByDBIDWithQueryer(ctx, s.db, aasDBID)
}

func (s *AssetAdministrationShellDatabase) getAssetAdministrationShellMapByDBIDInTransaction(ctx context.Context, tx *sql.Tx, aasDBID int64) (types.IAssetAdministrationShell, error) {
	if tx == nil {
		return nil, common.NewInternalServerError("AASREPO-MAPAAS-NILTX transaction must not be nil")
	}
	return s.getAssetAdministrationShellMapByDBIDWithQueryer(ctx, tx, aasDBID)
}

func (s *AssetAdministrationShellDatabase) getAssetAdministrationShellMapByDBIDWithQueryer(ctx context.Context, db aasDBQueryer, aasDBID int64) (types.IAssetAdministrationShell, error) {
	dialect := goqu.Dialect("postgres")
	collector, collectorErr := buildAASCollector()
	if collectorErr != nil {
		return nil, collectorErr
	}
	selectExpressions, selectErr := buildCoreAssetAdministrationShellSelectExpressions(ctx, collector, false)
	if selectErr != nil {
		return nil, common.NewInternalServerError("AASREPO-MAPAAS-BUILDMASKS " + selectErr.Error())
	}

	querySQL, queryArgs, buildErr := buildGetAssetAdministrationShellMapByDBIDQueryWithSelect(&dialect, aasDBID, selectExpressions)
	if buildErr != nil {
		return nil, common.NewInternalServerError("AASREPO-MAPAAS-BUILDSQL " + buildErr.Error())
	}

	var row coreAssetAdministrationShellRow

	if queryErr := db.QueryRowContext(ctx, querySQL, queryArgs...).Scan(
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
	); queryErr != nil {
		if queryErr == sql.ErrNoRows {
			return nil, common.NewErrNotFound("AASREPO-MAPAAS-AASNOTFOUND Asset Administration Shell not found")
		}
		return nil, common.NewInternalServerError("AASREPO-MAPAAS-EXECSQL " + queryErr.Error())
	}

	specificAssetIDs, specificErr := s.readSpecificAssetIDsByAssetInformationID(ctx, db, aasDBID)
	if specificErr != nil {
		return nil, common.NewInternalServerError("AASREPO-MAPAAS-READSPECIFICASSETIDS " + specificErr.Error())
	}

	submodelsByAASID, submodelErr := s.readSubmodelReferencePayloadsByAASDBIDs(ctx, db, []int64{aasDBID})
	if submodelErr != nil {
		return nil, submodelErr
	}

	return buildAssetAdministrationShellFromCoreRow(row, submodelsByAASID[aasDBID], specificAssetIDs, "AASREPO-MAPAAS")
}

// nolint:revive // cyclomatic complexity of 32
func (s *AssetAdministrationShellDatabase) getAssetAdministrationShellMapsByDBIDs(ctx context.Context, aasDBIDs []int64) ([]types.IAssetAdministrationShell, error) {
	if len(aasDBIDs) == 0 {
		return []types.IAssetAdministrationShell{}, nil
	}

	dialect := goqu.Dialect("postgres")
	collector, collectorErr := buildAASCollector()
	if collectorErr != nil {
		return nil, collectorErr
	}
	selectExpressions, selectErr := buildCoreAssetAdministrationShellSelectExpressions(ctx, collector, true)
	if selectErr != nil {
		return nil, common.NewInternalServerError("AASREPO-MAPAASBATCH-BUILDMASKS " + selectErr.Error())
	}

	querySQL, queryArgs, buildErr := buildGetAssetAdministrationShellMapsByDBIDsQueryWithSelect(&dialect, aasDBIDs, selectExpressions)
	if buildErr != nil {
		return nil, common.NewInternalServerError("AASREPO-MAPAASBATCH-BUILDSQL " + buildErr.Error())
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

	submodelsByAASID, submodelErr := s.readSubmodelReferencePayloadsByAASDBIDs(ctx, s.db, aasDBIDs)
	if submodelErr != nil {
		return nil, submodelErr
	}

	specificAssetIDsByAASID, specificErr := s.readSpecificAssetIDsByAssetInformationIDs(ctx, s.db, aasDBIDs)
	if specificErr != nil {
		return nil, common.NewInternalServerError("AASREPO-MAPAASBATCH-READSPECIFICASSETIDS " + specificErr.Error())
	}

	result := make([]types.IAssetAdministrationShell, 0, len(aasDBIDs))
	for _, aasDBID := range aasDBIDs {
		row, ok := coreRows[aasDBID]
		if !ok {
			continue
		}

		aas, mapErr := buildAssetAdministrationShellFromCoreRow(
			row,
			submodelsByAASID[aasDBID],
			specificAssetIDsByAASID[aasDBID],
			"AASREPO-MAPAASBATCH",
		)
		if mapErr != nil {
			return nil, mapErr
		}
		result = append(result, aas)
	}

	return result, nil
}

func (s *AssetAdministrationShellDatabase) readSubmodelReferencePayloadsByAASDBIDs(ctx context.Context, db aasDBQueryer, aasDBIDs []int64) (map[int64][]types.IReference, error) {
	out := make(map[int64][]types.IReference, len(aasDBIDs))
	if len(aasDBIDs) == 0 {
		return out, nil
	}

	dialect := goqu.Dialect("postgres")
	submodelDS := buildGetSubmodelReferencePayloadsByAASIDsDataset(&dialect, aasDBIDs)
	collector, collectorErr := buildAASCollector()
	if collectorErr != nil {
		return nil, collectorErr
	}
	submodelDS, filterErr := auth.AddFilterQueryFromContext(ctx, submodelDS, "$aas#submodels[]", collector)
	if filterErr != nil {
		return nil, common.NewInternalServerError("AASREPO-READSMREFBATCH-ABACFILTERS " + filterErr.Error())
	}
	submodelDS, filterErr = auth.AddFilterQueryFromContext(ctx, submodelDS, "$aas#submodels[].keys[]", collector)
	if filterErr != nil {
		return nil, common.NewInternalServerError("AASREPO-READSMREFBATCH-ABACKEYFILTERS " + filterErr.Error())
	}

	submodelSQL, submodelArgs, submodelBuildErr := submodelDS.ToSQL()
	if submodelBuildErr != nil {
		return nil, common.NewInternalServerError("AASREPO-READSMREFBATCH-BUILDSQL " + submodelBuildErr.Error())
	}

	rows, submodelQueryErr := db.QueryContext(ctx, submodelSQL, submodelArgs...)
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

		reference, parseErr := parseReferencePayload(payload, "AASREPO-READSMREFBATCH-PARSEREFERENCE")
		if parseErr != nil {
			return nil, parseErr
		}
		if reference == nil {
			continue
		}

		out[aasDBID] = append(out[aasDBID], reference)
	}
	if rowsErr := rows.Err(); rowsErr != nil {
		return nil, common.NewInternalServerError("AASREPO-READSMREFBATCH-ITERROWS " + rowsErr.Error())
	}

	return out, nil
}

func buildAssetAdministrationShellFromCoreRow(row coreAssetAdministrationShellRow, submodels []types.IReference, specificAssetIDs []types.ISpecificAssetID, errorPrefix string) (types.IAssetAdministrationShell, error) {
	aas := types.NewAssetAdministrationShell(row.aasID, buildAssetInformationFromCoreRow(row, specificAssetIDs))

	if row.idShort.Valid && row.idShort.String != "" {
		idShort := row.idShort.String
		aas.SetIDShort(&idShort)
	}
	if row.category.Valid && row.category.String != "" {
		category := row.category.String
		aas.SetCategory(&category)
	}

	displayName, displayErr := parseLangStringNameTypesPayload(row.displayNamePayload, errorPrefix+"-PARSEDISPLAYNAME")
	if displayErr != nil {
		return nil, displayErr
	}
	if len(displayName) > 0 {
		aas.SetDisplayName(displayName)
	}

	description, descriptionErr := parseLangStringTextTypesPayload(row.descriptionPayload, errorPrefix+"-PARSEDESCRIPTION")
	if descriptionErr != nil {
		return nil, descriptionErr
	}
	if len(description) > 0 {
		aas.SetDescription(description)
	}

	administration, administrationErr := parseAdministrativeInformationPayload(row.administrationPayload, errorPrefix+"-PARSEADMINISTRATION")
	if administrationErr != nil {
		return nil, administrationErr
	}
	if administration != nil {
		aas.SetAdministration(administration)
	}

	embeddedDataSpecifications, edsErr := parseEmbeddedDataSpecificationsPayload(row.edsPayload, errorPrefix+"-PARSEEDS")
	if edsErr != nil {
		return nil, edsErr
	}
	if len(embeddedDataSpecifications) > 0 {
		aas.SetEmbeddedDataSpecifications(embeddedDataSpecifications)
	}

	extensions, extensionsErr := parseExtensionsPayload(row.extensionsPayload, errorPrefix+"-PARSEEXTENSIONS")
	if extensionsErr != nil {
		return nil, extensionsErr
	}
	if len(extensions) > 0 {
		aas.SetExtensions(extensions)
	}

	derivedFrom, derivedFromErr := parseReferencePayload(row.derivedFromPayload, errorPrefix+"-PARSEDERIVEDFROM")
	if derivedFromErr != nil {
		return nil, derivedFromErr
	}
	if derivedFrom != nil {
		aas.SetDerivedFrom(derivedFrom)
	}

	if len(submodels) > 0 {
		aas.SetSubmodels(submodels)
	}

	return aas, nil
}

func buildAssetInformationFromCoreRow(row coreAssetAdministrationShellRow, specificAssetIDs []types.ISpecificAssetID) types.IAssetInformation {
	assetInformation := types.NewAssetInformation(resolveAssetKind(row.assetKind))

	if row.globalAssetID.Valid && row.globalAssetID.String != "" {
		globalAssetID := row.globalAssetID.String
		assetInformation.SetGlobalAssetID(&globalAssetID)
	}
	if row.assetType.Valid && row.assetType.String != "" {
		assetType := row.assetType.String
		assetInformation.SetAssetType(&assetType)
	}
	if thumbnail := buildThumbnailResource(row.thumbnailPath, row.thumbnailContentType); thumbnail != nil {
		assetInformation.SetDefaultThumbnail(thumbnail)
	}
	if len(specificAssetIDs) > 0 {
		assetInformation.SetSpecificAssetIDs(specificAssetIDs)
	}

	return assetInformation
}

func resolveAssetKind(assetKind sql.NullInt64) types.AssetKind {
	if !assetKind.Valid {
		return types.AssetKindType
	}

	parsed := types.AssetKind(assetKind.Int64)
	if _, ok := stringification.AssetKindToString(parsed); !ok {
		return types.AssetKindType
	}

	return parsed
}

func buildThumbnailResource(path sql.NullString, contentType sql.NullString) types.IResource {
	if !path.Valid || path.String == "" {
		return nil
	}

	thumbnail := types.NewResource(path.String)
	if contentType.Valid && contentType.String != "" {
		contentTypeValue := contentType.String
		thumbnail.SetContentType(&contentTypeValue)
	}

	return thumbnail
}

func parseJSONPayloadAsArrayOfMaps(payload []byte) ([]map[string]any, error) {
	if len(payload) == 0 {
		return nil, nil
	}

	var values []map[string]any
	if err := json.Unmarshal(payload, &values); err != nil {
		return nil, err
	}
	if len(values) == 0 {
		return nil, nil
	}

	return values, nil
}

func parseJSONPayloadAsMap(payload []byte) (map[string]any, error) {
	if len(payload) == 0 {
		return nil, nil
	}

	var value map[string]any
	if err := json.Unmarshal(payload, &value); err != nil {
		return nil, err
	}
	if len(value) == 0 {
		return nil, nil
	}

	return value, nil
}

func parseLangStringTextTypesPayload(payload []byte, errorPrefix string) ([]types.ILangStringTextType, error) {
	jsonable, err := parseJSONPayloadAsArrayOfMaps(payload)
	if err != nil {
		return nil, common.NewInternalServerError(errorPrefix + " " + err.Error())
	}

	values := make([]types.ILangStringTextType, 0, len(jsonable))
	for _, item := range jsonable {
		parsed, parseErr := jsonization.LangStringTextTypeFromJsonable(item)
		if parseErr != nil {
			return nil, common.NewInternalServerError(errorPrefix + " " + parseErr.Error())
		}
		values = append(values, parsed)
	}

	return values, nil
}

func parseLangStringNameTypesPayload(payload []byte, errorPrefix string) ([]types.ILangStringNameType, error) {
	jsonable, err := parseJSONPayloadAsArrayOfMaps(payload)
	if err != nil {
		return nil, common.NewInternalServerError(errorPrefix + " " + err.Error())
	}

	values := make([]types.ILangStringNameType, 0, len(jsonable))
	for _, item := range jsonable {
		parsed, parseErr := jsonization.LangStringNameTypeFromJsonable(item)
		if parseErr != nil {
			return nil, common.NewInternalServerError(errorPrefix + " " + parseErr.Error())
		}
		values = append(values, parsed)
	}

	return values, nil
}

func parseAdministrativeInformationPayload(payload []byte, errorPrefix string) (types.IAdministrativeInformation, error) {
	jsonable, err := parseJSONPayloadAsMap(payload)
	if err != nil {
		return nil, common.NewInternalServerError(errorPrefix + " " + err.Error())
	}
	if jsonable == nil {
		return nil, nil
	}

	parsed, parseErr := jsonization.AdministrativeInformationFromJsonable(jsonable)
	if parseErr != nil {
		return nil, common.NewInternalServerError(errorPrefix + " " + parseErr.Error())
	}

	return parsed, nil
}

func parseEmbeddedDataSpecificationsPayload(payload []byte, errorPrefix string) ([]types.IEmbeddedDataSpecification, error) {
	jsonable, err := parseJSONPayloadAsArrayOfMaps(payload)
	if err != nil {
		return nil, common.NewInternalServerError(errorPrefix + " " + err.Error())
	}

	values := make([]types.IEmbeddedDataSpecification, 0, len(jsonable))
	for _, item := range jsonable {
		parsed, parseErr := jsonization.EmbeddedDataSpecificationFromJsonable(item)
		if parseErr != nil {
			return nil, common.NewInternalServerError(errorPrefix + " " + parseErr.Error())
		}
		values = append(values, parsed)
	}

	return values, nil
}

func parseExtensionsPayload(payload []byte, errorPrefix string) ([]types.IExtension, error) {
	jsonable, err := parseJSONPayloadAsArrayOfMaps(payload)
	if err != nil {
		return nil, common.NewInternalServerError(errorPrefix + " " + err.Error())
	}

	values := make([]types.IExtension, 0, len(jsonable))
	for _, item := range jsonable {
		parsed, parseErr := jsonization.ExtensionFromJsonable(item)
		if parseErr != nil {
			return nil, common.NewInternalServerError(errorPrefix + " " + parseErr.Error())
		}
		values = append(values, parsed)
	}

	return values, nil
}

func parseReferencePayload(payload []byte, errorPrefix string) (types.IReference, error) {
	jsonable, err := parseJSONPayloadAsMap(payload)
	if err != nil {
		return nil, common.NewInternalServerError(errorPrefix + " " + err.Error())
	}
	if jsonable == nil {
		return nil, nil
	}

	parsed, parseErr := jsonization.ReferenceFromJsonable(jsonable)
	if parseErr != nil {
		return nil, common.NewInternalServerError(errorPrefix + " " + parseErr.Error())
	}

	return parsed, nil
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
func (s *AssetAdministrationShellDatabase) readSpecificAssetIDsByAssetInformationID(ctx context.Context, db aasDBQueryer, assetInformationID int64) ([]types.ISpecificAssetID, error) {
	dialect := goqu.Dialect("postgres")
	queryDS := buildReadSpecificAssetIDsByAssetInformationIDDataset(&dialect, assetInformationID)
	collector, collectorErr := buildAASCollector()
	if collectorErr != nil {
		return nil, collectorErr
	}
	queryDS, filterErr := auth.AddFilterQueryFromContext(ctx, queryDS, "$aas#assetInformation.specificAssetIds[]", collector)
	if filterErr != nil {
		return nil, common.NewInternalServerError("AASREPO-READSPECIFIC-ABACFILTERS " + filterErr.Error())
	}

	querySQL, queryArgs, buildErr := queryDS.ToSQL()
	if buildErr != nil {
		return nil, common.NewInternalServerError("AASREPO-READSPECIFIC-BUILDSQL " + buildErr.Error())
	}

	rows, queryErr := db.QueryContext(ctx, querySQL, queryArgs...)
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

	externalSubjectByID, extErr := descriptors.ReadSpecificAssetExternalSubjectReferencesBySpecificAssetIDs(ctx, db, ids)
	if extErr != nil {
		return nil, extErr
	}

	supplementalByID, suppErr := descriptors.ReadSpecificAssetSupplementalSemanticReferencesBySpecificAssetIDs(ctx, db, ids)
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
func (s *AssetAdministrationShellDatabase) readSpecificAssetIDsByAssetInformationIDs(ctx context.Context, db aasDBQueryer, assetInformationIDs []int64) (map[int64][]types.ISpecificAssetID, error) {
	out := make(map[int64][]types.ISpecificAssetID, len(assetInformationIDs))
	if len(assetInformationIDs) == 0 {
		return out, nil
	}

	dialect := goqu.Dialect("postgres")
	queryDS := buildReadSpecificAssetIDsByAssetInformationIDsDataset(&dialect, assetInformationIDs)
	collector, collectorErr := buildAASCollector()
	if collectorErr != nil {
		return nil, collectorErr
	}
	queryDS, filterErr := auth.AddFilterQueryFromContext(ctx, queryDS, "$aas#assetInformation.specificAssetIds[]", collector)
	if filterErr != nil {
		return nil, common.NewInternalServerError("AASREPO-READSPECIFICBATCH-ABACFILTERS " + filterErr.Error())
	}

	querySQL, queryArgs, buildErr := queryDS.ToSQL()
	if buildErr != nil {
		return nil, common.NewInternalServerError("AASREPO-READSPECIFICBATCH-BUILDSQL " + buildErr.Error())
	}

	rows, queryErr := db.QueryContext(ctx, querySQL, queryArgs...)
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

	externalSubjectByID, extErr := descriptors.ReadSpecificAssetExternalSubjectReferencesBySpecificAssetIDs(ctx, db, ids)
	if extErr != nil {
		return nil, extErr
	}

	supplementalByID, suppErr := descriptors.ReadSpecificAssetSupplementalSemanticReferencesBySpecificAssetIDs(ctx, db, ids)
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
