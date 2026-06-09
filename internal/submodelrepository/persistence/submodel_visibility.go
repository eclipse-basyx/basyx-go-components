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
// Author: Jannik Fried (Fraunhofer IESE)

package persistence

import (
	"context"
	"database/sql"
	"errors"

	"github.com/eclipse-basyx/basyx-go-components/internal/common"
	"github.com/eclipse-basyx/basyx-go-components/internal/common/model/grammar"
	auth "github.com/eclipse-basyx/basyx-go-components/internal/common/security"
	submodelqueries "github.com/eclipse-basyx/basyx-go-components/internal/submodelrepository/persistence/queries"
	persistenceutils "github.com/eclipse-basyx/basyx-go-components/internal/submodelrepository/persistence/utils"
)

func (s *SubmodelDatabase) checkSubmodelVisibilityInTx(ctx context.Context, tx *sql.Tx, submodelID string) (bool, bool, error) {
	_, err := persistenceutils.GetSubmodelDatabaseID(tx, submodelID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return false, false, nil
		}
		return false, false, common.NewInternalServerError("SMREPO-ABACCHKSM-GETSMDATABASEID " + err.Error())
	}

	shouldEnforce, enforceErr := shouldEnforceFormula(ctx, "SMREPO-ABACCHKSM-SHOULDENFORCE")
	if enforceErr != nil {
		return false, false, enforceErr
	}
	if !shouldEnforce {
		return true, true, nil
	}

	query := submodelqueries.SelectVisibleSubmodelDataset(submodelID)

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
	scanErr := tx.QueryRowContext(ctx, sqlQuery, args...).Scan(&databaseID)
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

	baseQuery := submodelqueries.SelectSubmodelElementByPathDataset(submodelDatabaseID, idShortPath)

	existsSQL, existsArgs, existsToSQLErr := baseQuery.ToSQL()
	if existsToSQLErr != nil {
		return false, false, common.NewInternalServerError("SMREPO-ABACCHKSME-BUILDEXISTSQ " + existsToSQLErr.Error())
	}

	var elementID int64
	existsErr := tx.QueryRowContext(ctx, existsSQL, existsArgs...).Scan(&elementID)
	if existsErr != nil {
		if errors.Is(existsErr, sql.ErrNoRows) {
			return false, false, nil
		}
		return false, false, common.NewInternalServerError("SMREPO-ABACCHKSME-EXECEXISTSQ " + existsErr.Error())
	}

	shouldEnforce, enforceErr := shouldEnforceFormula(ctx, "SMREPO-ABACCHKSME-SHOULDENFORCE")
	if enforceErr != nil {
		return false, false, enforceErr
	}
	if !shouldEnforce {
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
	visibleErr := tx.QueryRowContext(ctx, filteredSQL, filteredArgs...).Scan(&visibleID)
	if visibleErr == nil {
		return true, true, nil
	}
	if errors.Is(visibleErr, sql.ErrNoRows) {
		return true, false, nil
	}

	return false, false, common.NewInternalServerError("SMREPO-ABACCHKSME-EXECFILTERQ " + visibleErr.Error())
}

func shouldEnforceFormula(ctx context.Context, step string) (bool, error) {
	shouldEnforce, err := auth.ShouldEnforceFormula(ctx)
	if err != nil {
		return false, common.NewInternalServerError(step + " " + err.Error())
	}
	return shouldEnforce, nil
}
