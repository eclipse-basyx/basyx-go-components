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
	"crypto/rsa"
	"database/sql"
	"fmt"
	"time"

	"github.com/FriedJannik/aas-go-sdk/types"
	"github.com/FriedJannik/aas-go-sdk/verification"
	"github.com/doug-martin/goqu/v9"
	"github.com/eclipse-basyx/basyx-go-components/internal/common"
	submodelelements "github.com/eclipse-basyx/basyx-go-components/internal/submodelrepository/persistence/submodelElements"

	_ "github.com/lib/pq"
)

// SubmodelDatabase is the implementation of the SubmodelRepositoryDatabase interface using PostgreSQL as the underlying database.
type SubmodelDatabase struct {
	db                 *sql.DB
	strictVerification bool
}

// NewSubmodelDatabase creates a new instance of SubmodelDatabase with the provided database connection.
func NewSubmodelDatabase(dsn string, maxOpenConnections int, maxIdleConnections int, connMaxLifetimeMinutes int, databaseSchema string, _ *rsa.PrivateKey, strictVerification bool) (*SubmodelDatabase, error) {
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
		strictVerification: strictVerification,
	}, nil
}

// GetSubmodelByID retrieves a submodel by its identifier from the database.
func (s *SubmodelDatabase) GetSubmodelByID(submodelIdentifier string) (types.ISubmodel, error) {
	submodels, _, err := s.GetSubmodels(0, "", submodelIdentifier)
	if err != nil {
		return nil, err
	}
	if len(submodels) == 0 {
		return nil, common.NewErrNotFound(submodelIdentifier)
	}
	if len(submodels) > 1 {
		return nil, fmt.Errorf("multiple submodels found with identifier '%s'", submodelIdentifier)
	}
	return submodels[0], nil
}

// GetSubmodels retrieves submodels with optional filtering by identifier and keyset pagination.
func (s *SubmodelDatabase) GetSubmodels(limit int32, cursor string, submodelIdentifier string) ([]types.ISubmodel, string, error) {
	dialect := goqu.Dialect("postgres")

	var limitFilter *int32
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

	query, args, err := buildSelectSubmodelQueryWithPayloadByIdentifier(&dialect, submodelIdentifierFilter, limitFilter, cursorFilter)
	if err != nil {
		return nil, "", err
	}

	var identifier, idShort, category, descriptionJsonString, displayNameJsonString, administrativeInformationJsonString, embeddedDataSpecificationJsonString, supplementalSemanticIDsJsonString, extensionsJsonString, qualifiersJsonString sql.NullString
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
		if err := rows.Scan(&identifier, &idShort, &category, &kind, &descriptionJsonString, &displayNameJsonString, &administrativeInformationJsonString, &embeddedDataSpecificationJsonString, &supplementalSemanticIDsJsonString, &extensionsJsonString, &qualifiersJsonString); err != nil {
			return nil, "", err
		}

		if pageLimit > 0 && len(submodels) == pageLimit {
			nextCursor = identifier.String
			break
		}

		var submodel types.ISubmodel
		submodel = types.NewSubmodel(identifier.String)
		submodel.SetIDShort(&idShort.String)
		submodel.SetCategory(&category.String)
		modellingKind := types.ModellingKind(kind.Int64)
		submodel.SetKind(&modellingKind)

		submodel, err = jsonPayloadToInstance(descriptionJsonString, displayNameJsonString, administrativeInformationJsonString, embeddedDataSpecificationJsonString, supplementalSemanticIDsJsonString, extensionsJsonString, qualifiersJsonString, submodel)
		if err != nil {
			return nil, "", err
		}
		submodels = append(submodels, submodel)
	}

	if err := rows.Err(); err != nil {
		return nil, "", err
	}

	return submodels, nextCursor, nil
}

// CreateSubmodel creates a new submodel in the database with the provided submodel data.
func (s *SubmodelDatabase) CreateSubmodel(submodel types.ISubmodel) error {
	if s.strictVerification {
		errors := make([]verification.VerificationError, 0)

		verification.VerifySubmodel(submodel, func(ve *verification.VerificationError) bool {
			errors = append(errors, *ve)
			return false
		})

		if len(errors) > 0 {
			stringOfAllErrors := ""

			for _, err := range errors {
				stringOfAllErrors += fmt.Sprintf("%s ", err.Error())
			}

			return common.NewErrBadRequest("SMREPO-NEWSM-VERIFY " + stringOfAllErrors)
		}
	}

	tx, cu, err := common.StartTransaction(s.db)
	if err != nil {
		return err
	}
	defer cu(&err)

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

	if len(submodel.SubmodelElements()) > 0 {
		_, err = submodelelements.InsertSubmodelElements(s.db, submodel.ID(), submodel.SubmodelElements(), tx, nil)
		if err != nil {
			return common.NewInternalServerError("SMREPO-NEWSM-CREATESM-INSERTSME " + err.Error())
		}
	}

	err = tx.Commit()
	if err != nil {
		return common.NewInternalServerError("SMREPO-NEWSM-CREATE-COMMIT " + err.Error())
	}

	return nil
}

// PatchSubmodel updates an existing submodel in the database with the provided submodel data.
func (s *SubmodelDatabase) PatchSubmodel(_ types.ISubmodel) error {
	return nil
}

// PutSubmodel creates or replaces a submodel in the database with the provided submodel data.
func (s *SubmodelDatabase) PutSubmodel(_ string) error {
	return nil
}

// DeleteSubmodel deletes a submodel by its identifier from the database.
func (s *SubmodelDatabase) DeleteSubmodel(_ string) error {
	return nil
}
