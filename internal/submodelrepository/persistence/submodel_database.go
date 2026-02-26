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
// Author: Jannik Fried (Fraunhofer IESE), Aaron Zielstorff (Fraunhofer IESE)

// Package persistence contains the implementation of the SubmodelRepositoryDatabase interface using PostgreSQL as the underlying database.
package persistence

import (
	"context"
	"crypto/rsa"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"time"

	"github.com/FriedJannik/aas-go-sdk/jsonization"
	"github.com/FriedJannik/aas-go-sdk/types"
	"github.com/FriedJannik/aas-go-sdk/verification"
	"github.com/doug-martin/goqu/v9"
	"github.com/eclipse-basyx/basyx-go-components/internal/common"
	gen "github.com/eclipse-basyx/basyx-go-components/internal/common/model"
	"github.com/eclipse-basyx/basyx-go-components/internal/common/model/grammar"
	auth "github.com/eclipse-basyx/basyx-go-components/internal/common/security"
	"github.com/eclipse-basyx/basyx-go-components/internal/submodelrepository/config"
	submodelelements "github.com/eclipse-basyx/basyx-go-components/internal/submodelrepository/persistence/submodelElements"
	persistenceutils "github.com/eclipse-basyx/basyx-go-components/internal/submodelrepository/persistence/utils"
	"golang.org/x/sync/errgroup"
	jose "gopkg.in/go-jose/go-jose.v2"

	_ "github.com/lib/pq"
)

// SubmodelDatabase is the implementation of the SubmodelRepositoryDatabase interface using PostgreSQL as the underlying database.
type SubmodelDatabase struct {
	db                 *sql.DB
	privateKey         *rsa.PrivateKey
	strictVerification bool
}

// NewSubmodelDatabase creates a new instance of SubmodelDatabase with the provided database connection.
func NewSubmodelDatabase(dsn string, maxOpenConnections int, maxIdleConnections int, connMaxLifetimeMinutes int, databaseSchema string, privateKey *rsa.PrivateKey, strictVerification bool) (*SubmodelDatabase, error) {
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

	return &SubmodelDatabase{
		db:                 db,
		privateKey:         privateKey,
		strictVerification: strictVerification,
	}, nil
}

// GetSignedSubmodel retrieves and signs a submodel (or its value-only representation) as JWS compact serialization.
func (s *SubmodelDatabase) GetSignedSubmodel(submodelID string, valueOnly bool) (string, error) {
	if s.privateKey == nil {
		return "", errors.New("JWS signing not configured: private key not loaded")
	}

	submodel, err := s.GetSubmodelByID(submodelID)
	if err != nil {
		return "", err
	}

	var payload []byte
	if valueOnly {
		valueOnlySubmodel, conversionErr := gen.SubmodelToValueOnly(submodel)
		if conversionErr != nil {
			return "", conversionErr
		}
		payload, err = json.Marshal(valueOnlySubmodel)
		if err != nil {
			return "", err
		}
	} else {
		jsonSubmodel, convertErr := jsonization.ToJsonable(submodel)
		if convertErr != nil {
			return "", convertErr
		}
		payload, err = json.Marshal(jsonSubmodel)
		if err != nil {
			return "", err
		}
	}

	signer, err := jose.NewSigner(jose.SigningKey{Algorithm: jose.RS256, Key: s.privateKey}, nil)
	if err != nil {
		return "", err
	}

	jws, err := signer.Sign(payload)
	if err != nil {
		return "", err
	}

	return jws.CompactSerialize()
}

// GetSubmodelByID retrieves a submodel by its identifier from the database.
func (s *SubmodelDatabase) GetSubmodelByID(submodelIdentifier string) (types.ISubmodel, error) {
	return s.GetSubmodelByIDWithContext(context.Background(), submodelIdentifier)
}

// GetSubmodelByIDWithContext retrieves a submodel by identifier and applies optional ABAC formula filters from ctx.
func (s *SubmodelDatabase) GetSubmodelByIDWithContext(ctx context.Context, submodelIdentifier string) (types.ISubmodel, error) {
	eg := errgroup.Group{}
	var submodels []types.ISubmodel
	eg.Go(func() error {
		var err error
		submodels, _, err = s.GetSubmodelsWithContext(ctx, 0, "", submodelIdentifier)
		if err != nil {
			return err
		}
		if len(submodels) == 0 {
			return common.NewErrNotFound(submodelIdentifier)
		}
		if len(submodels) > 1 {
			return fmt.Errorf("multiple submodels found with identifier '%s'", submodelIdentifier)
		}
		return nil
	})
	submodelElements := make([]types.ISubmodelElement, 0)
	eg.Go(func() error {
		unlimited := -1
		// Exact /submodels/{id} reads should use the ABAC formula only as a gate for
		// returning the submodel, not to prune the returned SME tree.
		smes, _, err := s.GetSubmodelElements(submodelIdentifier, &unlimited, "", false)
		if err != nil {
			return err
		}
		submodelElements = smes
		return nil
	})

	err := eg.Wait()
	if err != nil {
		return nil, err
	}
	if len(submodels) == 0 {
		return nil, common.NewErrNotFound(submodelIdentifier)
	}
	if submodels[0] == nil {
		return nil, common.NewInternalServerError("SMREPO-GETSMBYID-NILSUBMODEL Loaded submodel is nil")
	}

	submodels[0].SetSubmodelElements(submodelElements)

	return submodels[0], nil
}

// GetSubmodels retrieves submodels with optional filtering by identifier and keyset pagination.
func (s *SubmodelDatabase) GetSubmodels(limit int32, cursor string, submodelIdentifier string) ([]types.ISubmodel, string, error) {
	return s.GetSubmodelsWithContext(context.Background(), limit, cursor, submodelIdentifier)
}

// GetSubmodelsWithContext retrieves submodels and applies optional ABAC formula filters from ctx.
func (s *SubmodelDatabase) GetSubmodelsWithContext(ctx context.Context, limit int32, cursor string, submodelIdentifier string) ([]types.ISubmodel, string, error) {
	return s.getSubmodelsWithOptionalSemanticIDFilter(ctx, limit, cursor, submodelIdentifier, "")
}

// GetSubmodelReferences retrieves references for submodels with optional filtering and keyset pagination.
func (s *SubmodelDatabase) GetSubmodelReferences(limit int32, cursor string, submodelIdentifier string, semanticID string) ([]types.IReference, string, error) {
	return s.GetSubmodelReferencesWithContext(context.Background(), limit, cursor, submodelIdentifier, semanticID)
}

// GetSubmodelReferencesWithContext retrieves references and applies optional ABAC formula filters from ctx.
func (s *SubmodelDatabase) GetSubmodelReferencesWithContext(ctx context.Context, limit int32, cursor string, submodelIdentifier string, semanticID string) ([]types.IReference, string, error) {
	submodels, nextCursor, err := s.getSubmodelsWithOptionalSemanticIDFilter(ctx, limit, cursor, submodelIdentifier, semanticID)
	if err != nil {
		return nil, "", err
	}

	references := make([]types.IReference, 0, len(submodels))
	for _, submodel := range submodels {
		if submodel == nil {
			return nil, "", common.NewInternalServerError("SMREPO-GETSMREF-NILSUBMODEL loaded submodel is nil")
		}

		reference, referenceErr := buildSubmodelModelReference(submodel.ID())
		if referenceErr != nil {
			return nil, "", referenceErr
		}

		references = append(references, reference)
	}

	return references, nextCursor, nil
}

// GetSubmodelReference retrieves the model reference for a single submodel.
func (s *SubmodelDatabase) GetSubmodelReference(submodelIdentifier string) (types.IReference, error) {
	if submodelIdentifier == "" {
		return nil, common.NewErrBadRequest("SMREPO-GETSMREFONE-EMPTYIDENTIFIER submodel identifier is required")
	}

	submodels, _, err := s.GetSubmodels(1, "", submodelIdentifier)
	if err != nil {
		return nil, err
	}

	if len(submodels) == 0 {
		return nil, common.NewErrNotFound(submodelIdentifier)
	}

	if submodels[0] == nil {
		return nil, common.NewInternalServerError("SMREPO-GETSMREFONE-NILSUBMODEL loaded submodel is nil")
	}

	return buildSubmodelModelReference(submodels[0].ID())
}

func buildSubmodelModelReference(submodelIdentifier string) (types.IReference, error) {
	if submodelIdentifier == "" {
		return nil, common.NewErrBadRequest("SMREPO-BUILDSMREF-INVALIDIDENTIFIER submodel identifier is required")
	}

	key := types.NewKey(types.KeyTypesSubmodel, submodelIdentifier)

	reference := types.NewReference(types.ReferenceTypesModelReference, []types.IKey{key})

	return reference, nil
}

// QuerySubmodels returns submodels that match the provided query and supports cursor-based pagination.
func (s *SubmodelDatabase) QuerySubmodels(limit int32, cursor string, queryWrapper *grammar.QueryWrapper, includeChildren bool) ([]types.ISubmodel, string, error) {
	return s.QuerySubmodelsWithContext(context.Background(), limit, cursor, queryWrapper, includeChildren)
}

// QuerySubmodelsWithContext returns submodels and applies optional ABAC formula filters from ctx.
func (s *SubmodelDatabase) QuerySubmodelsWithContext(ctx context.Context, limit int32, cursor string, queryWrapper *grammar.QueryWrapper, _ bool) ([]types.ISubmodel, string, error) {
	if queryWrapper == nil || queryWrapper.Query.Condition == nil {
		return nil, "", common.NewErrBadRequest("SMREPO-QUERYSMS-INVALIDQUERY query condition is required")
	}

	collector, collectorErr := grammar.NewResolvedFieldPathCollectorForRoot(grammar.CollectorRootSM)
	if collectorErr != nil {
		return nil, "", common.NewErrBadRequest("SMREPO-QUERYSMS-BADCOLLECTOR " + collectorErr.Error())
	}

	whereExpr, _, evalErr := queryWrapper.Query.Condition.EvaluateToExpression(collector)
	if evalErr != nil {
		return nil, "", common.NewErrBadRequest("SMREPO-QUERYSMS-BADEXPR " + evalErr.Error())
	}

	pageLimit := int(limit)
	if pageLimit <= 0 {
		pageLimit = int(config.DefaultPageLimit)
	}
	usePagination := pageLimit > 0

	dialect := goqu.Dialect("postgres")
	query := dialect.
		From(goqu.T("submodel")).
		LeftJoin(
			goqu.T("submodel_semantic_id_reference").As("semantic_id_reference"),
			goqu.On(goqu.I("semantic_id_reference.id").Eq(goqu.I("submodel.id"))),
		).
		LeftJoin(
			goqu.T("submodel_semantic_id_reference_key").As("semantic_id_reference_key"),
			goqu.On(goqu.I("semantic_id_reference_key.reference_id").Eq(goqu.I("semantic_id_reference.id"))),
		).
		LeftJoin(
			goqu.T("submodel_element").As("submodel_element"),
			goqu.On(goqu.I("submodel_element.submodel_id").Eq(goqu.I("submodel.id"))),
		).
		LeftJoin(
			goqu.T("property_element").As("property_element"),
			goqu.On(goqu.I("property_element.id").Eq(goqu.I("submodel_element.id"))),
		).
		LeftJoin(
			goqu.T("multilanguage_property").As("multilanguage_property"),
			goqu.On(goqu.I("multilanguage_property.id").Eq(goqu.I("submodel_element.id"))),
		).
		LeftJoin(
			goqu.T("multilanguage_property_value").As("multilanguage_property_value"),
			goqu.On(goqu.I("multilanguage_property_value.mlp_id").Eq(goqu.I("multilanguage_property.id"))),
		).
		LeftJoin(
			goqu.T("submodel_element_semantic_id_reference").As("sme_semantic_id_reference"),
			goqu.On(goqu.I("sme_semantic_id_reference.id").Eq(goqu.I("submodel_element.id"))),
		).
		LeftJoin(
			goqu.T("submodel_element_semantic_id_reference_key").As("sme_semantic_id_reference_key"),
			goqu.On(goqu.I("sme_semantic_id_reference_key.reference_id").Eq(goqu.I("sme_semantic_id_reference.id"))),
		).
		SelectDistinct(goqu.I("submodel.submodel_identifier")).
		Where(whereExpr).
		Order(goqu.I("submodel.submodel_identifier").Asc())

	if cursor != "" {
		query = query.Where(goqu.I("submodel.submodel_identifier").Gt(cursor))
	}

	if usePagination {
		//nolint:gosec // pageLimit is validated to be > 0 and bounded by int32 input/default
		query = query.Limit(uint(pageLimit + 1))
	}

	query, addFormulaErr := auth.AddFormulaQueryFromContext(ctx, query, collector)
	if addFormulaErr != nil {
		return nil, "", common.NewInternalServerError("SMREPO-QUERYSMS-ABACFORMULA " + addFormulaErr.Error())
	}
	query = query.Prepared(true)

	sqlQuery, args, toSQLErr := query.ToSQL()
	if toSQLErr != nil {
		return nil, "", common.NewInternalServerError("SMREPO-QUERYSMS-BUILDSQL " + toSQLErr.Error())
	}

	rows, queryErr := s.db.Query(sqlQuery, args...)
	if queryErr != nil {
		return nil, "", common.NewInternalServerError("SMREPO-QUERYSMS-EXECQ " + queryErr.Error())
	}
	defer func() { _ = rows.Close() }()

	identifierCapacity := 64
	if usePagination {
		identifierCapacity = pageLimit + 1
	}
	identifiers := make([]string, 0, identifierCapacity)
	for rows.Next() {
		var identifier string
		if scanErr := rows.Scan(&identifier); scanErr != nil {
			return nil, "", common.NewInternalServerError("SMREPO-QUERYSMS-SCANID " + scanErr.Error())
		}
		identifiers = append(identifiers, identifier)
	}

	if rowsErr := rows.Err(); rowsErr != nil {
		return nil, "", common.NewInternalServerError("SMREPO-QUERYSMS-ROWERR " + rowsErr.Error())
	}

	nextCursor := ""
	if usePagination && len(identifiers) > pageLimit {
		nextCursor = identifiers[pageLimit]
		identifiers = identifiers[:pageLimit]
	}

	result := make([]types.ISubmodel, 0, len(identifiers))
	for _, identifier := range identifiers {
		sm, getErr := s.GetSubmodelByID(identifier)
		if getErr != nil {
			return nil, "", getErr
		}
		result = append(result, sm)
	}

	return result, nextCursor, nil
}

// CreateSubmodel creates a new submodel in the database with the provided submodel data.
func (s *SubmodelDatabase) CreateSubmodel(submodel types.ISubmodel) error {
	return s.CreateSubmodelWithContext(context.Background(), submodel)
}

// CreateSubmodelWithContext creates a new submodel and performs an ABAC re-check before commit when ABAC is enabled.
func (s *SubmodelDatabase) CreateSubmodelWithContext(ctx context.Context, submodel types.ISubmodel) (err error) {
	if err := s.verifySubmodel(submodel, "SMREPO-NEWSM-VERIFY"); err != nil {
		return err
	}

	tx, cu, err := common.StartTransaction(s.db)
	if err != nil {
		return common.NewInternalServerError("SMREPO-NEWSM-STARTTX " + err.Error())
	}
	defer cu(&err)

	err = s.createSubmodelInTransaction(tx, submodel)
	if err != nil {
		return err
	}

	if shouldEnforceABACWriteCheck(ctx) {
		exists, visible, visErr := s.checkSubmodelVisibilityInTx(ctx, tx, submodel.ID())
		if visErr != nil {
			return visErr
		}
		if !exists {
			return common.NewInternalServerError("SMREPO-NEWSM-ABACCHECKMISSING created submodel not found before commit")
		}
		if !visible {
			return common.NewErrDenied("SMREPO-NEWSM-ABACDENIED Created submodel is not accessible under ABAC constraints")
		}
	}

	err = tx.Commit()
	if err != nil {
		return common.NewInternalServerError("SMREPO-NEWSM-CREATE-COMMIT " + err.Error())
	}

	return nil
}

func (s *SubmodelDatabase) createSubmodelInTransaction(tx *sql.Tx, submodel types.ISubmodel) error {
	dialect := goqu.Dialect("postgres")

	ids, args, err := buildSubmodelQuery(&dialect, submodel)
	if err != nil {
		return common.NewInternalServerError("SMREPO-NEWSM-CREATE-INSERTSQL " + err.Error())
	}

	var submodelDBID int64
	if err := tx.QueryRow(ids, args...).Scan(&submodelDBID); err != nil {
		return common.NewInternalServerError("SMREPO-NEWSM-CREATE-EXECSQL " + err.Error())
	}

	jsonizedPayload, err := jsonizeSubmodelPayload(submodel)
	if err != nil {
		return common.NewInternalServerError("SMREPO-NEWSM-CREATE-JSON " + err.Error())
	}

	ids, args, err = buildSubmodelPayloadQuery(
		&dialect,
		submodelDBID,
		jsonizedPayload.description,
		jsonizedPayload.displayName,
		jsonizedPayload.administrativeInformation,
		jsonizedPayload.embeddedDataSpecification,
		jsonizedPayload.supplementalSemanticIDs,
		jsonizedPayload.extensions,
		jsonizedPayload.qualifiers,
	)
	if err != nil {
		return common.NewInternalServerError("SMREPO-NEWSM-CREATE-PAYLOADSQL " + err.Error())
	}

	if _, err := tx.Exec(ids, args...); err != nil {
		return common.NewInternalServerError("SMREPO-NEWSM-CREATE-EXECPAYLOADSQL " + err.Error())
	}

	semanticID := submodel.SemanticID()
	if semanticID != nil {
		ids, args, err = buildSubmodelSemanticIDReferenceQuery(&dialect, submodelDBID, semanticID)
		if err != nil {
			return common.NewInternalServerError("SMREPO-NEWSM-CREATE-SEMIDREFSQL " + err.Error())
		}

		if _, err := tx.Exec(ids, args...); err != nil {
			return common.NewInternalServerError("SMREPO-NEWSM-CREATE-EXECSEMIDREFSQL " + err.Error())
		}

		ids, args, err = buildSubmodelSemanticIDReferenceKeysQuery(&dialect, submodelDBID, semanticID)
		if err != nil {
			return common.NewInternalServerError("SMREPO-NEWSM-CREATE-SEMIDKEYSQL " + err.Error())
		}

		if ids != "" {
			if _, err := tx.Exec(ids, args...); err != nil {
				return common.NewInternalServerError("SMREPO-NEWSM-CREATE-EXECSEMIDKEYSQL " + err.Error())
			}
		}

		ids, args, err = buildSubmodelSemanticIDReferencePayloadQuery(&dialect, submodelDBID, semanticID)
		if err != nil {
			return common.NewInternalServerError("SMREPO-NEWSM-CREATE-SEMIDPAYLOADSQL " + err.Error())
		}

		if _, err := tx.Exec(ids, args...); err != nil {
			return common.NewInternalServerError("SMREPO-NEWSM-CREATE-EXECSEMIDPAYLOADSQL " + err.Error())
		}
	}

	if len(submodel.SubmodelElements()) > 0 {
		_, err = submodelelements.InsertSubmodelElements(s.db, submodel.ID(), submodel.SubmodelElements(), tx, nil)
		if err != nil {
			return common.NewInternalServerError("SMREPO-NEWSM-CREATESM-INSERTSME " + err.Error())
		}
	}

	return nil
}

func (s *SubmodelDatabase) verifySubmodel(submodel types.ISubmodel, errorPrefix string) error {
	if !s.strictVerification {
		return nil
	}

	verificationErrors := make([]verification.VerificationError, 0)

	verification.VerifySubmodel(submodel, func(ve *verification.VerificationError) bool {
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

func normalizeCtx(ctx context.Context) context.Context {
	if ctx == nil {
		return context.Background()
	}
	return ctx
}

func shouldEnforceABACWriteCheck(ctx context.Context) bool {
	cfg, ok := common.ConfigFromContext(ctx)
	if !ok || !cfg.ABAC.Enabled {
		return false
	}
	queryFilter := auth.GetQueryFilter(ctx)
	return queryFilter != nil && queryFilter.Formula != nil
}

func (s *SubmodelDatabase) checkSubmodelVisibilityInTx(ctx context.Context, tx *sql.Tx, submodelID string) (bool, bool, error) {
	_, err := persistenceutils.GetSubmodelDatabaseID(tx, submodelID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return false, false, nil
		}
		return false, false, common.NewInternalServerError("SMREPO-ABACCHKSM-GETSMDATABASEID " + err.Error())
	}

	if !shouldEnforceABACWriteCheck(ctx) {
		return true, true, nil
	}

	dialect := goqu.Dialect("postgres")
	query := dialect.
		From("submodel").
		Select(goqu.C("id")).
		Where(goqu.C("submodel_identifier").Eq(submodelID)).
		Limit(1)

	collector, collectorErr := grammar.NewResolvedFieldPathCollectorForRoot(grammar.CollectorRootSM)
	if collectorErr != nil {
		return false, false, common.NewInternalServerError("SMREPO-ABACCHKSM-BADCOLLECTOR " + collectorErr.Error())
	}

	query, addFormulaErr := auth.AddFormulaQueryFromContext(ctx, query, collector)
	if addFormulaErr != nil {
		return false, false, common.NewInternalServerError("SMREPO-ABACCHKSM-ADDFORMULA " + addFormulaErr.Error())
	}

	sqlQuery, args, toSQLErr := query.ToSQL()
	if toSQLErr != nil {
		return false, false, common.NewInternalServerError("SMREPO-ABACCHKSM-BUILDQ " + toSQLErr.Error())
	}

	var databaseID int64
	scanErr := tx.QueryRowContext(normalizeCtx(ctx), sqlQuery, args...).Scan(&databaseID)
	if scanErr == nil {
		return true, true, nil
	}
	if errors.Is(scanErr, sql.ErrNoRows) {
		return true, false, nil
	}

	return false, false, common.NewInternalServerError("SMREPO-ABACCHKSM-EXECQ " + scanErr.Error())
}

func (s *SubmodelDatabase) checkSubmodelElementVisibilityInTx(ctx context.Context, tx *sql.Tx, submodelID string, idShortPath string) (bool, bool, error) {
	submodelDatabaseID, err := persistenceutils.GetSubmodelDatabaseID(tx, submodelID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return false, false, nil
		}
		return false, false, common.NewInternalServerError("SMREPO-ABACCHKSME-GETSMDATABASEID " + err.Error())
	}

	dialect := goqu.Dialect("postgres")
	baseQuery := dialect.
		From(goqu.T("submodel_element").As("sme")).
		Select(goqu.I("sme.id")).
		Where(
			goqu.I("sme.submodel_id").Eq(submodelDatabaseID),
			goqu.I("sme.idshort_path").Eq(idShortPath),
		).
		Limit(1)

	existsSQL, existsArgs, existsToSQLErr := baseQuery.ToSQL()
	if existsToSQLErr != nil {
		return false, false, common.NewInternalServerError("SMREPO-ABACCHKSME-BUILDEXISTSQ " + existsToSQLErr.Error())
	}

	var elementID int64
	existsErr := tx.QueryRowContext(normalizeCtx(ctx), existsSQL, existsArgs...).Scan(&elementID)
	if existsErr != nil {
		if errors.Is(existsErr, sql.ErrNoRows) {
			return false, false, nil
		}
		return false, false, common.NewInternalServerError("SMREPO-ABACCHKSME-EXECEXISTSQ " + existsErr.Error())
	}

	if !shouldEnforceABACWriteCheck(ctx) {
		return true, true, nil
	}

	collector, collectorErr := grammar.NewResolvedFieldPathCollectorForRoot(grammar.CollectorRootSME)
	if collectorErr != nil {
		return false, false, common.NewInternalServerError("SMREPO-ABACCHKSME-BADCOLLECTOR " + collectorErr.Error())
	}

	filteredQuery, addFormulaErr := auth.AddFormulaQueryFromContext(ctx, baseQuery, collector)
	if addFormulaErr != nil {
		return false, false, common.NewInternalServerError("SMREPO-ABACCHKSME-ADDFORMULA " + addFormulaErr.Error())
	}

	filteredSQL, filteredArgs, filteredToSQLErr := filteredQuery.ToSQL()
	if filteredToSQLErr != nil {
		return false, false, common.NewInternalServerError("SMREPO-ABACCHKSME-BUILDFILTERQ " + filteredToSQLErr.Error())
	}

	var visibleID int64
	visibleErr := tx.QueryRowContext(normalizeCtx(ctx), filteredSQL, filteredArgs...).Scan(&visibleID)
	if visibleErr == nil {
		return true, true, nil
	}
	if errors.Is(visibleErr, sql.ErrNoRows) {
		return true, false, nil
	}

	return false, false, common.NewInternalServerError("SMREPO-ABACCHKSME-EXECFILTERQ " + visibleErr.Error())
}

func (s *SubmodelDatabase) addTopLevelSubmodelElementInTransaction(tx *sql.Tx, submodelID string, submodelElement types.ISubmodelElement) (string, error) {
	submodelDatabaseID, err := persistenceutils.GetSubmodelDatabaseID(tx, submodelID)
	if err != nil {
		if err == sql.ErrNoRows {
			return "", common.NewErrNotFound("SMREPO-ADDSME-SMNOTFOUND Submodel with ID '" + submodelID + "' not found")
		}
		return "", err
	}

	dialect := goqu.Dialect("postgres")
	selectQuery, selectArgs, err := dialect.From("submodel_element").
		Select(goqu.MAX("position")).
		Where(
			goqu.C("submodel_id").Eq(submodelDatabaseID),
			goqu.C("parent_sme_id").IsNull(),
		).
		ToSQL()
	if err != nil {
		return "", err
	}

	var maxPosition sql.NullInt64
	err = tx.QueryRow(selectQuery, selectArgs...).Scan(&maxPosition)
	if err != nil {
		return "", err
	}

	startPosition := 0
	if maxPosition.Valid {
		startPosition = int(maxPosition.Int64) + 1
	}

	if isSiblingIDShortCollision(tx, submodelDatabaseID, nil, submodelElement) {
		return "", common.NewErrConflict("SMREPO-ADDSME-COLLISION Duplicate submodel element idShort")
	}

	_, err = submodelelements.InsertSubmodelElements(
		s.db,
		submodelID,
		[]types.ISubmodelElement{submodelElement},
		tx,
		&submodelelements.BatchInsertContext{
			StartPosition: startPosition,
		},
	)
	if err != nil {
		return "", err
	}

	idShort := submodelElement.IDShort()
	if idShort == nil {
		return "", nil
	}

	return *idShort, nil
}

func getSMEModelTypeByPathInTx(tx *sql.Tx, submodelID string, idShortOrPath string) (*types.ModelType, error) {
	submodelDatabaseID, err := persistenceutils.GetSubmodelDatabaseID(tx, submodelID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, common.NewErrNotFound("SMREPO-GETMODELTYPE-SMNOTFOUND Submodel with ID '" + submodelID + "' not found")
		}
		return nil, err
	}

	dialect := goqu.Dialect("postgres")
	query, args, err := dialect.From("submodel_element").
		Select("model_type").
		Where(
			goqu.C("submodel_id").Eq(submodelDatabaseID),
			goqu.C("idshort_path").Eq(idShortOrPath),
		).
		ToSQL()
	if err != nil {
		return nil, err
	}

	var modelType types.ModelType
	err = tx.QueryRow(query, args...).Scan(&modelType)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, common.NewErrNotFound("SMREPO-GETMODELTYPE-NOTFOUND Submodel-Element ID-Short: " + idShortOrPath)
		}
		return nil, err
	}

	return &modelType, nil
}

func (s *SubmodelDatabase) updateSubmodelElementInTransaction(tx *sql.Tx, submodelID string, idShortOrPath string, submodelElement types.ISubmodelElement, isPut bool) error {
	modelType, err := getSMEModelTypeByPathInTx(tx, submodelID, idShortOrPath)
	if err != nil {
		return err
	}

	if modelType == nil {
		return common.NewErrNotFound("SMREPO-UPDSME-NOTFOUND Submodel-Element ID-Short: " + idShortOrPath)
	}

	handler, err := submodelelements.GetSMEHandlerByModelType(*modelType, s.db)
	if err != nil {
		return err
	}

	return handler.Update(submodelID, idShortOrPath, submodelElement, tx, isPut)
}

// GetSubmodelElement retrieves a submodel element (including nested children) by idShort path.
func (s *SubmodelDatabase) GetSubmodelElement(submodelID string, idShortOrPath string, _ bool) (types.ISubmodelElement, error) {
	return s.GetSubmodelElementWithContext(context.Background(), submodelID, idShortOrPath, false)
}

// GetSubmodelElementWithContext retrieves a submodel element by path and applies optional ABAC formula filters from ctx.
func (s *SubmodelDatabase) GetSubmodelElementWithContext(ctx context.Context, submodelID string, idShortOrPath string, _ bool) (types.ISubmodelElement, error) {
	return submodelelements.GetSubmodelElementByIDShortOrPathWithContext(ctx, s.db, submodelID, idShortOrPath)
}

// GetSubmodelElements retrieves top-level submodel elements for a submodel and reconstructs each subtree.
func (s *SubmodelDatabase) GetSubmodelElements(submodelID string, limit *int, cursor string, _ bool) ([]types.ISubmodelElement, string, error) {
	return s.GetSubmodelElementsWithContext(context.Background(), submodelID, limit, cursor, false)
}

// GetSubmodelElementsWithContext retrieves submodel elements and applies optional ABAC formula filters from ctx.
func (s *SubmodelDatabase) GetSubmodelElementsWithContext(ctx context.Context, submodelID string, limit *int, cursor string, _ bool) ([]types.ISubmodelElement, string, error) {
	return submodelelements.GetSubmodelElementsBySubmodelID(ctx, s.db, submodelID, limit, cursor)
}

// GetSubmodelElementReferences retrieves references for top-level submodel elements of a submodel with optional pagination.
func (s *SubmodelDatabase) GetSubmodelElementReferences(submodelID string, limit *int, cursor string) ([]types.IReference, string, error) {
	return s.GetSubmodelElementReferencesWithContext(context.Background(), submodelID, limit, cursor)
}

// GetSubmodelElementReferencesWithContext retrieves SME references and applies optional ABAC formula filters from ctx.
func (s *SubmodelDatabase) GetSubmodelElementReferencesWithContext(ctx context.Context, submodelID string, limit *int, cursor string) ([]types.IReference, string, error) {
	return submodelelements.GetSubmodelElementReferencesBySubmodelID(ctx, s.db, submodelID, limit, cursor)
}

// AddSubmodelElement adds a top-level submodel element to a submodel.
func (s *SubmodelDatabase) AddSubmodelElement(submodelID string, submodelElement types.ISubmodelElement) error {
	return s.AddSubmodelElementWithContext(context.Background(), submodelID, submodelElement)
}

// AddSubmodelElementWithContext adds a top-level submodel element and performs an ABAC re-check before commit when ABAC is enabled.
func (s *SubmodelDatabase) AddSubmodelElementWithContext(ctx context.Context, submodelID string, submodelElement types.ISubmodelElement) (err error) {
	tx, cleanup, err := common.StartTransaction(s.db)
	if err != nil {
		return err
	}
	defer cleanup(&err)

	insertedPath, err := s.addTopLevelSubmodelElementInTransaction(tx, submodelID, submodelElement)
	if err != nil {
		return err
	}

	if shouldEnforceABACWriteCheck(ctx) && insertedPath != "" {
		exists, visible, visErr := s.checkSubmodelElementVisibilityInTx(ctx, tx, submodelID, insertedPath)
		if visErr != nil {
			return visErr
		}
		if !exists {
			return common.NewInternalServerError("SMREPO-ADDSME-ABACCHECKMISSING created submodel element not found before commit")
		}
		if !visible {
			return common.NewErrDenied("SMREPO-ADDSME-ABACDENIED Created submodel element is not accessible under ABAC constraints")
		}
	}

	return tx.Commit()
}

// AddSubmodelElementWithPath adds a submodel element under an existing container path.
func (s *SubmodelDatabase) AddSubmodelElementWithPath(submodelID string, parentPath string, submodelElement types.ISubmodelElement) error {
	tx, cleanup, err := common.StartTransaction(s.db)
	if err != nil {
		return err
	}
	defer cleanup(&err)

	submodelDatabaseID, err := persistenceutils.GetSubmodelDatabaseID(tx, submodelID)
	if err != nil {
		if err == sql.ErrNoRows {
			return common.NewErrNotFound("SMREPO-ADDSMEBYPATH-SMNOTFOUND Submodel with ID '" + submodelID + "' not found")
		}
		return err
	}

	baseCrudHandler, err := submodelelements.NewPostgreSQLSMECrudHandler(s.db)
	if err != nil {
		return err
	}

	parentElementID, err := baseCrudHandler.GetDatabaseID(submodelDatabaseID, parentPath)
	if err != nil {
		return err
	}

	rootSmeID, err := baseCrudHandler.GetRootSmeIDByElementID(parentElementID)
	if err != nil {
		return err
	}

	parentElement, err := submodelelements.GetSubmodelElementByIDShortOrPath(s.db, submodelID, parentPath)
	if err != nil {
		return err
	}

	isFromList := false
	switch parentElement.ModelType() {
	case types.ModelTypeSubmodelElementCollection, types.ModelTypeEntity, types.ModelTypeAnnotatedRelationshipElement:
		isFromList = false
	case types.ModelTypeSubmodelElementList:
		isFromList = true
	default:
		return common.NewErrBadRequest("SMREPO-ADDSMEBYPATH-BADPARENT Parent element does not support child elements")
	}

	nextPosition, err := baseCrudHandler.GetNextPosition(parentElementID)
	if err != nil {
		return err
	}

	if isSiblingIDShortCollision(tx, submodelDatabaseID, &parentElementID, submodelElement) {
		return common.NewErrConflict("SMREPO-ADDSMEBYPATH-COLLISION Duplicate submodel element idShort")
	}

	_, err = submodelelements.InsertSubmodelElements(
		s.db,
		submodelID,
		[]types.ISubmodelElement{submodelElement},
		tx,
		&submodelelements.BatchInsertContext{
			ParentID:      parentElementID,
			ParentPath:    parentPath,
			RootSmeID:     rootSmeID,
			IsFromList:    isFromList,
			StartPosition: nextPosition,
		},
	)

	if err != nil {
		return err
	}

	return tx.Commit()
}

func isSiblingIDShortCollision(tx *sql.Tx, submodelDatabaseID int, parentElementID *int, submodelElement types.ISubmodelElement) bool {
	idShortPtr := submodelElement.IDShort()
	if idShortPtr == nil || *idShortPtr == "" {
		return false
	}

	dialect := goqu.Dialect("postgres")
	query := dialect.From("submodel_element").
		Select(goqu.COUNT("*"))

	whereExpressions := []goqu.Expression{
		goqu.C("submodel_id").Eq(submodelDatabaseID),
		goqu.C("id_short").Eq(*idShortPtr),
	}

	if parentElementID == nil {
		whereExpressions = append(whereExpressions, goqu.C("parent_sme_id").IsNull())
	} else {
		whereExpressions = append(whereExpressions, goqu.C("parent_sme_id").Eq(*parentElementID))
	}

	sqlQuery, args, err := query.Where(whereExpressions...).ToSQL()
	if err != nil {
		return false
	}

	var count int
	if err = tx.QueryRow(sqlQuery, args...).Scan(&count); err != nil {
		return false
	}

	return count > 0
}

// DeleteSubmodelElementByPath deletes a submodel element by idShort path.
func (s *SubmodelDatabase) DeleteSubmodelElementByPath(submodelID string, idShortPath string) error {
	return s.DeleteSubmodelElementByPathWithContext(context.Background(), submodelID, idShortPath)
}

// DeleteSubmodelElementByPathWithContext deletes a submodel element and checks ABAC access on the current element when ABAC is enabled.
func (s *SubmodelDatabase) DeleteSubmodelElementByPathWithContext(ctx context.Context, submodelID string, idShortPath string) (err error) {
	tx, cleanup, err := common.StartTransaction(s.db)
	if err != nil {
		return err
	}
	defer cleanup(&err)

	if shouldEnforceABACWriteCheck(ctx) {
		exists, visible, visErr := s.checkSubmodelElementVisibilityInTx(ctx, tx, submodelID, idShortPath)
		if visErr != nil {
			return visErr
		}
		if !exists {
			return common.NewErrNotFound("SMREPO-DELSMEBPATH-NOTFOUND Submodel-Element ID-Short: " + idShortPath)
		}
		if !visible {
			return common.NewErrDenied("SMREPO-DELSMEBPATH-ABACDENIED Deleting this submodel element is not allowed")
		}
	}

	err = submodelelements.DeleteSubmodelElementByPath(tx, submodelID, idShortPath)
	if err != nil {
		return err
	}

	return tx.Commit()
}

// UpdateSubmodelElement updates a submodel element by path.
func (s *SubmodelDatabase) UpdateSubmodelElement(submodelID string, idShortOrPath string, submodelElement types.ISubmodelElement, isPut bool) error {
	modelType, err := submodelelements.GetModelTypeByIdShortPathAndSubmodelID(s.db, submodelID, idShortOrPath)
	if err != nil {
		return err
	}

	if modelType == nil {
		return common.NewErrNotFound("SMREPO-UPDSME-NOTFOUND Submodel-Element ID-Short: " + idShortOrPath)
	}

	handler, err := submodelelements.GetSMEHandlerByModelType(*modelType, s.db)
	if err != nil {
		return err
	}

	return handler.Update(submodelID, idShortOrPath, submodelElement, nil, isPut)
}

// UpdateSubmodelElementWithContext updates a submodel element and checks ABAC access on old and new state when ABAC is enabled.
func (s *SubmodelDatabase) UpdateSubmodelElementWithContext(ctx context.Context, submodelID string, idShortOrPath string, submodelElement types.ISubmodelElement, isPut bool) (err error) {
	tx, cleanup, err := common.StartTransaction(s.db)
	if err != nil {
		return err
	}
	defer cleanup(&err)

	if shouldEnforceABACWriteCheck(ctx) {
		exists, visible, visErr := s.checkSubmodelElementVisibilityInTx(ctx, tx, submodelID, idShortOrPath)
		if visErr != nil {
			return visErr
		}
		if !exists {
			return common.NewErrNotFound("SMREPO-UPDSME-NOTFOUND Submodel-Element ID-Short: " + idShortOrPath)
		}
		if !visible {
			return common.NewErrDenied("SMREPO-UPDSME-ABACDENIED Existing submodel element is not accessible under ABAC constraints")
		}
	}

	err = s.updateSubmodelElementInTransaction(tx, submodelID, idShortOrPath, submodelElement, isPut)
	if err != nil {
		return err
	}

	if shouldEnforceABACWriteCheck(ctx) {
		exists, visible, visErr := s.checkSubmodelElementVisibilityInTx(ctx, tx, submodelID, idShortOrPath)
		if visErr != nil {
			return visErr
		}
		if !exists || !visible {
			return common.NewErrDenied("SMREPO-UPDSME-ABACDENIED Updated submodel element is not accessible under ABAC constraints")
		}
	}

	return tx.Commit()
}

// UpdateSubmodelElementValueOnly updates a submodel element using value-only representation.
func (s *SubmodelDatabase) UpdateSubmodelElementValueOnly(submodelID string, idShortOrPath string, valueOnly gen.SubmodelElementValue) error {
	modelType, err := submodelelements.GetModelTypeByIdShortPathAndSubmodelID(s.db, submodelID, idShortOrPath)
	if err != nil {
		return err
	}

	if modelType == nil {
		return common.NewErrNotFound("SMREPO-UPDSMEVALONLY-NOTFOUND Submodel-Element ID-Short: " + idShortOrPath)
	}

	handler, err := submodelelements.GetSMEHandlerByModelType(*modelType, s.db)
	if err != nil {
		return err
	}

	return handler.UpdateValueOnly(submodelID, idShortOrPath, valueOnly)
}

// UpdateSubmodelValueOnly updates all included top-level submodel elements using value-only representation.
func (s *SubmodelDatabase) UpdateSubmodelValueOnly(submodelID string, valueOnly gen.SubmodelValue) error {
	for idShort, elementValue := range valueOnly {
		if err := s.UpdateSubmodelElementValueOnly(submodelID, idShort, elementValue); err != nil {
			return err
		}
	}

	return nil
}

// UploadFileAttachment uploads attachment content for a File submodel element.
func (s *SubmodelDatabase) UploadFileAttachment(submodelID string, idShortPath string, file *os.File, fileName string) error {
	fileHandler, err := submodelelements.NewPostgreSQLFileHandler(s.db)
	if err != nil {
		return err
	}

	return fileHandler.UploadFileAttachment(submodelID, idShortPath, file, fileName)
}

// DownloadFileAttachment downloads attachment content for a File submodel element.
func (s *SubmodelDatabase) DownloadFileAttachment(submodelID string, idShortPath string) ([]byte, string, string, error) {
	fileHandler, err := submodelelements.NewPostgreSQLFileHandler(s.db)
	if err != nil {
		return nil, "", "", err
	}

	return fileHandler.DownloadFileAttachment(submodelID, idShortPath)
}

// DeleteFileAttachment deletes attachment content of a File submodel element.
func (s *SubmodelDatabase) DeleteFileAttachment(submodelID string, idShortPath string) error {
	fileHandler, err := submodelelements.NewPostgreSQLFileHandler(s.db)
	if err != nil {
		return err
	}

	return fileHandler.DeleteFileAttachment(submodelID, idShortPath)
}

// PatchSubmodel updates an existing submodel in the database with the provided submodel data.
func (s *SubmodelDatabase) PatchSubmodel(submodelID string, submodel types.ISubmodel) error {
	if submodelID != submodel.ID() {
		return common.NewErrBadRequest("SMREPO-PATCHSM-IDMISMATCH Submodel ID in path and body do not match")
	}

	if err := s.verifySubmodel(submodel, "SMREPO-PATCHSM-VERIFY"); err != nil {
		return err
	}

	tx, cleanup, err := common.StartTransaction(s.db)
	if err != nil {
		return common.NewInternalServerError("SMREPO-PATCHSM-STARTTX " + err.Error())
	}
	defer cleanup(&err)

	_, err = s.replaceSubmodelInTransaction(tx, submodelID, submodel, true)
	if err != nil {
		return err
	}

	err = tx.Commit()
	if err != nil {
		return common.NewInternalServerError("SMREPO-PATCHSM-COMMIT " + err.Error())
	}

	return nil
}

// PatchSubmodelMetadata updates a submodel without rewriting submodel elements.
func (s *SubmodelDatabase) PatchSubmodelMetadata(submodelID string, submodel types.ISubmodel) error {
	if submodelID != submodel.ID() {
		return common.NewErrBadRequest("SMREPO-PATCHSMMETA-IDMISMATCH Submodel ID in path and body do not match")
	}

	if err := s.verifySubmodel(submodel, "SMREPO-PATCHSMMETA-VERIFY"); err != nil {
		return err
	}

	tx, cleanup, err := common.StartTransaction(s.db)
	if err != nil {
		return common.NewInternalServerError("SMREPO-PATCHSMMETA-STARTTX " + err.Error())
	}
	defer cleanup(&err)

	if err = s.patchSubmodelMetadataInTransaction(tx, submodelID, submodel); err != nil {
		return err
	}

	err = tx.Commit()
	if err != nil {
		return common.NewInternalServerError("SMREPO-PATCHSMMETA-COMMIT " + err.Error())
	}

	return nil
}

// PutSubmodel creates or replaces a submodel in the database with the provided submodel data.
func (s *SubmodelDatabase) PutSubmodel(submodelID string, submodel types.ISubmodel) (bool, error) {
	return s.PutSubmodelWithContext(context.Background(), submodelID, submodel)
}

// PutSubmodelWithContext creates or replaces a submodel and checks ABAC access on old/new state before commit when ABAC is enabled.
func (s *SubmodelDatabase) PutSubmodelWithContext(ctx context.Context, submodelID string, submodel types.ISubmodel) (bool, error) {
	if submodelID != submodel.ID() {
		return false, common.NewErrBadRequest("SMREPO-PUTSM-IDMISMATCH Submodel ID in path and body do not match")
	}

	if err := s.verifySubmodel(submodel, "SMREPO-PUTSM-VERIFY"); err != nil {
		return false, err
	}

	tx, cleanup, err := common.StartTransaction(s.db)
	if err != nil {
		return false, common.NewInternalServerError("SMREPO-PUTSM-STARTTX " + err.Error())
	}
	defer cleanup(&err)

	if shouldEnforceABACWriteCheck(ctx) {
		exists, visible, visErr := s.checkSubmodelVisibilityInTx(ctx, tx, submodelID)
		if visErr != nil {
			return false, visErr
		}
		if exists && !visible {
			return false, common.NewErrDenied("SMREPO-PUTSM-ABACDENIED Existing submodel is not accessible under ABAC constraints")
		}
	}

	isUpdate, err := s.replaceSubmodelInTransaction(tx, submodelID, submodel, false)
	if err != nil {
		return false, err
	}

	if shouldEnforceABACWriteCheck(ctx) {
		exists, visible, visErr := s.checkSubmodelVisibilityInTx(ctx, tx, submodelID)
		if visErr != nil {
			return false, visErr
		}
		if !exists {
			return false, common.NewInternalServerError("SMREPO-PUTSM-ABACCHECKMISSING written submodel not found before commit")
		}
		if !visible {
			return false, common.NewErrDenied("SMREPO-PUTSM-ABACDENIED Written submodel is not accessible under ABAC constraints")
		}
	}

	err = tx.Commit()
	if err != nil {
		return false, common.NewInternalServerError("SMREPO-PUTSM-COMMIT " + err.Error())
	}

	return isUpdate, nil
}

// DeleteSubmodel deletes a submodel by its identifier from the database.
func (s *SubmodelDatabase) DeleteSubmodel(submodelID string) error {
	return s.DeleteSubmodelWithContext(context.Background(), submodelID)
}

// DeleteSubmodelWithContext deletes a submodel and checks ABAC access on the existing submodel before delete when ABAC is enabled.
func (s *SubmodelDatabase) DeleteSubmodelWithContext(ctx context.Context, submodelID string) (err error) {
	tx, cleanup, err := common.StartTransaction(s.db)
	if err != nil {
		return common.NewInternalServerError("SMREPO-DELSM-STARTTX " + err.Error())
	}
	defer cleanup(&err)

	if shouldEnforceABACWriteCheck(ctx) {
		exists, visible, visErr := s.checkSubmodelVisibilityInTx(ctx, tx, submodelID)
		if visErr != nil {
			return visErr
		}
		if !exists {
			return common.NewErrNotFound("SMREPO-DELSM-NOTFOUND Submodel with ID '" + submodelID + "' not found")
		}
		if !visible {
			return common.NewErrDenied("SMREPO-DELSM-ABACDENIED Deleting this submodel is not allowed")
		}
	}

	submodelDatabaseID, err := persistenceutils.GetSubmodelDatabaseID(tx, submodelID)
	if err != nil {
		if err == sql.ErrNoRows {
			return common.NewErrNotFound("SMREPO-DELSM-NOTFOUND Submodel with ID '" + submodelID + "' not found")
		}
		return common.NewInternalServerError("SMREPO-DELSM-GETSMDATABASEID " + err.Error())
	}

	err = cleanupSubmodelLargeObjects(tx, int64(submodelDatabaseID))
	if err != nil {
		return err
	}

	err = deleteSubmodelByDatabaseID(tx, int64(submodelDatabaseID))
	if err != nil {
		return err
	}

	err = tx.Commit()
	if err != nil {
		return common.NewInternalServerError("SMREPO-DELSM-COMMIT " + err.Error())
	}

	return nil
}

func (s *SubmodelDatabase) replaceSubmodelInTransaction(tx *sql.Tx, submodelID string, submodel types.ISubmodel, requireExisting bool) (bool, error) {
	submodelDatabaseID, err := persistenceutils.GetSubmodelDatabaseID(tx, submodelID)
	if err != nil {
		if err == sql.ErrNoRows {
			if requireExisting {
				return false, common.NewErrNotFound("SMREPO-UPDSM-NOTFOUND Submodel with ID '" + submodelID + "' not found")
			}

			if createErr := s.createSubmodelInTransaction(tx, submodel); createErr != nil {
				return false, createErr
			}
			return false, nil
		}

		return false, common.NewInternalServerError("SMREPO-UPDSM-GETSMDATABASEID " + err.Error())
	}

	err = cleanupSubmodelLargeObjects(tx, int64(submodelDatabaseID))
	if err != nil {
		return false, err
	}

	err = deleteSubmodelByDatabaseID(tx, int64(submodelDatabaseID))
	if err != nil {
		return false, err
	}

	err = s.createSubmodelInTransaction(tx, submodel)
	if err != nil {
		return false, err
	}

	return true, nil
}

func cleanupSubmodelLargeObjects(tx *sql.Tx, submodelDatabaseID int64) error {
	dialect := goqu.Dialect("postgres")

	selectOIDQuery, selectOIDArgs, err := dialect.From(goqu.T("submodel_element").As("sme")).
		Join(goqu.T("file_data").As("fd"), goqu.On(goqu.I("fd.id").Eq(goqu.I("sme.id")))).
		Select(goqu.I("fd.file_oid")).
		Where(
			goqu.I("sme.submodel_id").Eq(submodelDatabaseID),
			goqu.I("fd.file_oid").IsNotNull(),
		).
		ToSQL()
	if err != nil {
		return common.NewInternalServerError("SMREPO-DELSM-LISTFILEOIDS " + err.Error())
	}

	rows, err := tx.Query(selectOIDQuery, selectOIDArgs...)
	if err != nil {
		return common.NewInternalServerError("SMREPO-DELSM-LISTFILEOIDS " + err.Error())
	}
	defer func() {
		_ = rows.Close()
	}()

	for rows.Next() {
		var oid sql.NullInt64
		if err := rows.Scan(&oid); err != nil {
			return common.NewInternalServerError("SMREPO-DELSM-SCANFILEOID " + err.Error())
		}

		if !oid.Valid {
			continue
		}

		unlinkQuery, unlinkArgs, unlinkErr := dialect.Select(goqu.Func("lo_unlink", oid.Int64)).ToSQL()
		if unlinkErr != nil {
			return common.NewInternalServerError("SMREPO-DELSM-BUILDUNLINKQUERY " + unlinkErr.Error())
		}

		if _, unlinkExecErr := tx.Exec(unlinkQuery, unlinkArgs...); unlinkExecErr != nil {
			return common.NewInternalServerError("SMREPO-DELSM-UNLINKLO " + unlinkExecErr.Error())
		}
	}

	if err := rows.Err(); err != nil {
		return common.NewInternalServerError("SMREPO-DELSM-LISTFILEOIDSROWS " + err.Error())
	}

	return nil
}

func deleteSubmodelByDatabaseID(tx *sql.Tx, submodelDatabaseID int64) error {
	dialect := goqu.Dialect("postgres")
	deleteSubmodelQuery, deleteSubmodelArgs, err := dialect.Delete("submodel").Where(goqu.I("id").Eq(submodelDatabaseID)).ToSQL()
	if err != nil {
		return common.NewInternalServerError("SMREPO-DELSM-BUILDDELETESM " + err.Error())
	}

	deleteResult, err := tx.Exec(deleteSubmodelQuery, deleteSubmodelArgs...)
	if err != nil {
		return common.NewInternalServerError("SMREPO-DELSM-DELETESM " + err.Error())
	}

	rowsAffected, err := deleteResult.RowsAffected()
	if err != nil {
		return common.NewInternalServerError("SMREPO-DELSM-ROWSAFFECTED " + err.Error())
	}
	if rowsAffected == 0 {
		return common.NewErrNotFound("SMREPO-DELSM-NOTFOUND Submodel not found")
	}

	return nil
}

func (s *SubmodelDatabase) patchSubmodelMetadataInTransaction(tx *sql.Tx, submodelID string, submodel types.ISubmodel) error {
	submodelDatabaseID, err := persistenceutils.GetSubmodelDatabaseID(tx, submodelID)
	if err != nil {
		if err == sql.ErrNoRows {
			return common.NewErrNotFound("SMREPO-PATCHSMMETA-NOTFOUND Submodel with ID '" + submodelID + "' not found")
		}

		return common.NewInternalServerError("SMREPO-PATCHSMMETA-GETSMDATABASEID " + err.Error())
	}

	dialect := goqu.Dialect("postgres")

	updateSubmodelQuery, updateSubmodelArgs, err := dialect.
		Update("submodel").
		Set(goqu.Record{
			"id_short": submodel.IDShort(),
			"category": submodel.Category(),
			"kind":     submodel.Kind(),
		}).
		Where(goqu.I("id").Eq(submodelDatabaseID)).
		ToSQL()
	if err != nil {
		return common.NewInternalServerError("SMREPO-PATCHSMMETA-BUILDUPDATESM " + err.Error())
	}

	if _, err = tx.Exec(updateSubmodelQuery, updateSubmodelArgs...); err != nil {
		return common.NewInternalServerError("SMREPO-PATCHSMMETA-UPDATESM " + err.Error())
	}

	jsonizedPayload, err := jsonizeSubmodelPayload(submodel)
	if err != nil {
		return common.NewInternalServerError("SMREPO-PATCHSMMETA-JSON " + err.Error())
	}

	upsertPayloadQuery, upsertPayloadArgs, err := dialect.
		Insert("submodel_payload").
		Rows(goqu.Record{
			"submodel_id":                         submodelDatabaseID,
			"description_payload":                 jsonizedPayload.description,
			"displayname_payload":                 jsonizedPayload.displayName,
			"administrative_information_payload":  jsonizedPayload.administrativeInformation,
			"embedded_data_specification_payload": jsonizedPayload.embeddedDataSpecification,
			"supplemental_semantic_ids_payload":   jsonizedPayload.supplementalSemanticIDs,
			"extensions_payload":                  jsonizedPayload.extensions,
			"qualifiers_payload":                  jsonizedPayload.qualifiers,
		}).
		OnConflict(goqu.DoUpdate(
			"submodel_id",
			goqu.Record{
				"description_payload":                 jsonizedPayload.description,
				"displayname_payload":                 jsonizedPayload.displayName,
				"administrative_information_payload":  jsonizedPayload.administrativeInformation,
				"embedded_data_specification_payload": jsonizedPayload.embeddedDataSpecification,
				"supplemental_semantic_ids_payload":   jsonizedPayload.supplementalSemanticIDs,
				"extensions_payload":                  jsonizedPayload.extensions,
				"qualifiers_payload":                  jsonizedPayload.qualifiers,
			},
		)).
		ToSQL()
	if err != nil {
		return common.NewInternalServerError("SMREPO-PATCHSMMETA-BUILDUPSERTPAYLOAD " + err.Error())
	}

	if _, err = tx.Exec(upsertPayloadQuery, upsertPayloadArgs...); err != nil {
		return common.NewInternalServerError("SMREPO-PATCHSMMETA-UPSERTPAYLOAD " + err.Error())
	}

	deleteSemanticIDQuery, deleteSemanticIDArgs, err := dialect.
		Delete("submodel_semantic_id_reference").
		Where(goqu.I("id").Eq(submodelDatabaseID)).
		ToSQL()
	if err != nil {
		return common.NewInternalServerError("SMREPO-PATCHSMMETA-BUILDDELSEMID " + err.Error())
	}

	if _, err = tx.Exec(deleteSemanticIDQuery, deleteSemanticIDArgs...); err != nil {
		return common.NewInternalServerError("SMREPO-PATCHSMMETA-DELSEMID " + err.Error())
	}

	semanticID := submodel.SemanticID()
	if semanticID == nil {
		return nil
	}

	insertSemanticIDQuery, insertSemanticIDArgs, err := buildSubmodelSemanticIDReferenceQuery(&dialect, int64(submodelDatabaseID), semanticID)
	if err != nil {
		return common.NewInternalServerError("SMREPO-PATCHSMMETA-BUILDSEMIDREF " + err.Error())
	}

	if _, err = tx.Exec(insertSemanticIDQuery, insertSemanticIDArgs...); err != nil {
		return common.NewInternalServerError("SMREPO-PATCHSMMETA-INSERTSEMIDREF " + err.Error())
	}

	insertSemanticKeysQuery, insertSemanticKeysArgs, err := buildSubmodelSemanticIDReferenceKeysQuery(&dialect, int64(submodelDatabaseID), semanticID)
	if err != nil {
		return common.NewInternalServerError("SMREPO-PATCHSMMETA-BUILDSEMIDKEYS " + err.Error())
	}

	if insertSemanticKeysQuery != "" {
		if _, err = tx.Exec(insertSemanticKeysQuery, insertSemanticKeysArgs...); err != nil {
			return common.NewInternalServerError("SMREPO-PATCHSMMETA-INSERTSEMIDKEYS " + err.Error())
		}
	}

	insertSemanticPayloadQuery, insertSemanticPayloadArgs, err := buildSubmodelSemanticIDReferencePayloadQuery(&dialect, int64(submodelDatabaseID), semanticID)
	if err != nil {
		return common.NewInternalServerError("SMREPO-PATCHSMMETA-BUILDSEMIDPAYLOAD " + err.Error())
	}

	if _, err = tx.Exec(insertSemanticPayloadQuery, insertSemanticPayloadArgs...); err != nil {
		return common.NewInternalServerError("SMREPO-PATCHSMMETA-INSERTSEMIDPAYLOAD " + err.Error())
	}

	return nil
}

func (s *SubmodelDatabase) getSubmodelsWithOptionalSemanticIDFilter(ctx context.Context, limit int32, cursor string, submodelIdentifier string, semanticID string) ([]types.ISubmodel, string, error) {
	dialect := goqu.Dialect("postgres")

	var limitFilter *int32

	if limit == 0 {
		limit = 100
	}

	if limit > 0 {
		limitFilter = &limit
	}

	var cursorFilter *string
	if cursor != "" {
		cursorFilter = &cursor
	}

	var submodelIdentifierFilter *string
	if submodelIdentifier != "" {
		submodelIdentifierFilter = &submodelIdentifier
	}

	selectDS, err := selectSubmodelGoquQuery(&dialect, submodelIdentifierFilter, limitFilter, cursorFilter)
	if err != nil {
		return nil, "", err
	}
	if semanticID != "" {
		semanticIDFilterDS := dialect.
			From(goqu.T("submodel_semantic_id_reference_key").As("ssrk_filter")).
			Select(goqu.V(1)).
			Where(goqu.I("ssrk_filter.reference_id").Eq(goqu.I("submodel.id"))).
			Where(goqu.I("ssrk_filter.value").Eq(semanticID))
		selectDS = selectDS.Where(goqu.Func("EXISTS", semanticIDFilterDS))
	}
	collector, collectorErr := grammar.NewResolvedFieldPathCollectorForRoot(grammar.CollectorRootSM)
	if collectorErr != nil {
		return nil, "", common.NewInternalServerError("SMREPO-GETSMS-BADCOLLECTOR " + collectorErr.Error())
	}
	selectDS, err = auth.AddFormulaQueryFromContext(ctx, selectDS, collector)
	if err != nil {
		return nil, "", common.NewInternalServerError("SMREPO-GETSMS-ABACFORMULA " + err.Error())
	}
	query, args, err := selectDS.ToSQL()
	if err != nil {
		return nil, "", common.NewInternalServerError("SMREPO-GETSMS-BUILDSQL " + err.Error())
	}

	var identifier, idShort, category, descriptionJsonString, displayNameJsonString, administrativeInformationJsonString, embeddedDataSpecificationJsonString, supplementalSemanticIDsJsonString, extensionsJsonString, qualifiersJsonString, semanticIDJSONString sql.NullString
	var kind sql.NullInt64

	rows, err := s.db.Query(query, args...)
	if err != nil {
		if err == sql.ErrNoRows {
			if submodelIdentifierFilter != nil {
				return nil, "", common.NewErrNotFound(*submodelIdentifierFilter)
			}
			return nil, "", common.NewErrNotFound("submodel")
		}
		return nil, "", err
	}
	defer func() {
		_ = rows.Close()
	}()

	pageLimit := 0
	if limitFilter != nil {
		pageLimit = int(*limitFilter)
	}

	submodels := make([]types.ISubmodel, 0)
	nextCursor := ""
	for rows.Next() {
		if err := rows.Scan(&identifier, &idShort, &category, &kind, &descriptionJsonString, &displayNameJsonString, &administrativeInformationJsonString, &embeddedDataSpecificationJsonString, &supplementalSemanticIDsJsonString, &extensionsJsonString, &qualifiersJsonString, &semanticIDJSONString); err != nil {
			return nil, "", err
		}

		if pageLimit > 0 && len(submodels) == pageLimit {
			nextCursor = identifier.String
			break
		}

		var submodel types.ISubmodel
		submodel = types.NewSubmodel(identifier.String)
		submodel.SetIDShort(&idShort.String)
		if category.Valid {
			submodel.SetCategory(&category.String)
		}
		if kind.Valid {
			modellingKind := types.ModellingKind(kind.Int64)
			submodel.SetKind(&modellingKind)
		}

		submodel, err = jsonPayloadToInstance(descriptionJsonString, displayNameJsonString, administrativeInformationJsonString, embeddedDataSpecificationJsonString, supplementalSemanticIDsJsonString, extensionsJsonString, qualifiersJsonString, submodel)
		if err != nil {
			return nil, "", err
		}

		if semanticIDJSONString.Valid {
			semanticID, parseSemanticErr := common.ParseReferenceJSON([]byte(semanticIDJSONString.String))
			if parseSemanticErr != nil {
				return nil, "", parseSemanticErr
			}
			if semanticID != nil {
				submodel.SetSemanticID(semanticID)
			}
		}

		submodels = append(submodels, submodel)
	}

	if err := rows.Err(); err != nil {
		return nil, "", err
	}

	return submodels, nextCursor, nil
}
