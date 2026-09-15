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
	"time"

	"github.com/FriedJannik/aas-go-sdk/jsonization"
	"github.com/FriedJannik/aas-go-sdk/types"
	"github.com/doug-martin/goqu/v9"
	"github.com/doug-martin/goqu/v9/exp"
	"github.com/eclipse-basyx/basyx-go-components/internal/common"
	"github.com/eclipse-basyx/basyx-go-components/internal/common/history"
	auth "github.com/eclipse-basyx/basyx-go-components/internal/common/security"
)

const (
	dppIdentifierPropertyIDShort       = "digitalProductPassportId"
	maxHistoricalDPPIdentityCandidates = uint(1024)
)

type historicalDPPIdentityBudget struct {
	seen map[string]struct{}
}

type historicalAASLookupResult struct {
	aas   types.IAssetAdministrationShell
	found bool
}

// GetAssetAdministrationShellByDPPIDAndDate returns the historical AAS that
// owned DPP metadata with the supplied DPP identifier at the requested instant.
// It resolves both metadata and AAS references from version history and does
// not depend on either resource still existing in the live repository.
//
// Parameters:
//   - ctx: Request context carrying cancellation, database routing, and access filters.
//   - dppID: DPP identifier stored in the metadata submodel.
//   - metadataSemanticIDs: Accepted semantic identifiers for DPP metadata submodels.
//   - at: Point in time at which the DPP association must have been valid.
//
// Returns:
//   - types.IAssetAdministrationShell: Historical owning AAS valid at at.
//   - error: Not found for missing, deleted, or filtered history; conflict for
//     multiple historical owners; otherwise a coded persistence error.
func (s *AssetAdministrationShellDatabase) GetAssetAdministrationShellByDPPIDAndDate(
	ctx context.Context,
	dppID string,
	metadataSemanticIDs []string,
	at time.Time,
) (types.IAssetAdministrationShell, error) {
	if auth.HistoricalReadRequiresBackendFilter(ctx) {
		return nil, common.NewErrNotFound("AASREPO-HISTORY-DPP-FILTERED historical DPP is not visible with the active query filter")
	}

	readDB := s.readDB(ctx)
	budget := historicalDPPIdentityBudget{seen: make(map[string]struct{})}
	metadataIDs, err := historicalIdentifierCandidates(ctx, readDB, history.TableSubmodel, dppID, at, &budget)
	if err != nil {
		return nil, err
	}
	metadataIDs, err = matchingHistoricalDPPMetadataIDs(ctx, readDB, metadataIDs, dppID, metadataSemanticIDs, at)
	if err != nil {
		return nil, err
	}

	owners, err := s.historicalDPPMetadataOwners(ctx, readDB, metadataIDs, at, &budget)
	if err != nil {
		return nil, err
	}
	if len(owners) == 1 {
		return owners[0], nil
	}
	if len(owners) > 1 {
		return nil, common.NewErrConflict("AASREPO-HISTORY-DPP-AMBIGUOUS multiple historical AAS records contain DPP ID '" + dppID + "'")
	}
	return nil, common.NewErrNotFound("AASREPO-HISTORY-DPP-NOTFOUND historical DPP with ID '" + dppID + "' not found")
}

func matchingHistoricalDPPMetadataIDs(
	ctx context.Context,
	db *sql.DB,
	candidates []string,
	dppID string,
	metadataSemanticIDs []string,
	at time.Time,
) ([]string, error) {
	matching := make([]string, 0, len(candidates))
	for _, identifier := range candidates {
		snapshot, err := history.SnapshotByDate(ctx, db, history.TableSubmodel, identifier, at)
		if common.IsErrNotFound(err) {
			continue
		}
		if err != nil {
			return nil, err
		}
		submodel, err := jsonization.SubmodelFromJsonable(snapshot)
		if err != nil {
			return nil, common.NewInternalServerError("AASREPO-HISTORY-DPP-PARSEMETADATA " + err.Error())
		}
		if historicalSubmodelMatchesDPP(submodel, dppID, metadataSemanticIDs) {
			matching = append(matching, identifier)
		}
	}
	return matching, nil
}

func (s *AssetAdministrationShellDatabase) historicalDPPMetadataOwners(
	ctx context.Context,
	db *sql.DB,
	metadataIDs []string,
	at time.Time,
	budget *historicalDPPIdentityBudget,
) ([]types.IAssetAdministrationShell, error) {
	ownersByID := make(map[string]types.IAssetAdministrationShell)
	aasByID := make(map[string]historicalAASLookupResult)
	for _, metadataID := range metadataIDs {
		ambiguous, err := addHistoricalDPPMetadataOwners(ctx, db, metadataID, at, budget, aasByID, ownersByID)
		if err != nil {
			return nil, err
		}
		if ambiguous {
			return historicalAASValues(ownersByID), nil
		}
	}
	return historicalAASValues(ownersByID), nil
}

func addHistoricalDPPMetadataOwners(
	ctx context.Context,
	db *sql.DB,
	metadataID string,
	at time.Time,
	budget *historicalDPPIdentityBudget,
	aasByID map[string]historicalAASLookupResult,
	ownersByID map[string]types.IAssetAdministrationShell,
) (bool, error) {
	candidateIDs, err := historicalIdentifierCandidates(ctx, db, history.TableAAS, metadataID, at, budget)
	if err != nil {
		return false, err
	}
	for _, identifier := range candidateIDs {
		aas, found, err := cachedHistoricalAASByID(ctx, db, identifier, at, aasByID)
		if err != nil {
			return false, err
		}
		if !found || !aasReferencesSubmodel(aas, metadataID) {
			continue
		}
		ownersByID[aas.ID()] = aas
		if len(ownersByID) > 1 {
			return true, nil
		}
	}
	return false, nil
}

func cachedHistoricalAASByID(
	ctx context.Context,
	db *sql.DB,
	identifier string,
	at time.Time,
	cache map[string]historicalAASLookupResult,
) (types.IAssetAdministrationShell, bool, error) {
	if cached, exists := cache[identifier]; exists {
		return cached.aas, cached.found, nil
	}
	aas, err := historicalAASByID(ctx, db, identifier, at)
	if common.IsErrNotFound(err) {
		cache[identifier] = historicalAASLookupResult{}
		return nil, false, nil
	}
	if err != nil {
		return nil, false, err
	}
	cache[identifier] = historicalAASLookupResult{aas: aas, found: true}
	return aas, true, nil
}

func historicalAASValues(ownersByID map[string]types.IAssetAdministrationShell) []types.IAssetAdministrationShell {
	owners := make([]types.IAssetAdministrationShell, 0, len(ownersByID))
	for _, aas := range ownersByID {
		owners = append(owners, aas)
	}
	return owners
}

func historicalAASByID(ctx context.Context, db *sql.DB, identifier string, at time.Time) (types.IAssetAdministrationShell, error) {
	snapshot, err := history.SnapshotByDate(ctx, db, history.TableAAS, identifier, at)
	if err != nil {
		return nil, err
	}
	aas, err := jsonization.AssetAdministrationShellFromJsonable(snapshot)
	if err != nil {
		return nil, common.NewInternalServerError("AASREPO-HISTORY-DPP-PARSEAAS " + err.Error())
	}
	return aas, nil
}

func historicalSubmodelMatchesDPP(submodel types.ISubmodel, dppID string, metadataSemanticIDs []string) bool {
	if !historicalSubmodelHasSemanticID(submodel, metadataSemanticIDs) {
		return false
	}
	for _, element := range submodel.SubmodelElements() {
		if element.IDShort() == nil || *element.IDShort() != dppIdentifierPropertyIDShort {
			continue
		}
		property, ok := element.(types.IProperty)
		return ok && property.Value() != nil && *property.Value() == dppID
	}
	return false
}

func historicalSubmodelHasSemanticID(submodel types.ISubmodel, metadataSemanticIDs []string) bool {
	if submodel == nil {
		return false
	}
	semanticID := historicalReferenceLastValue(submodel.SemanticID())
	for _, expected := range metadataSemanticIDs {
		if semanticID == expected {
			return true
		}
	}
	return false
}

func aasReferencesSubmodel(aas types.IAssetAdministrationShell, submodelID string) bool {
	for _, reference := range aas.Submodels() {
		if historicalReferenceLastValue(reference) == submodelID {
			return true
		}
	}
	return false
}

func historicalReferenceLastValue(reference types.IReference) string {
	if reference == nil || len(reference.Keys()) == 0 {
		return ""
	}
	return reference.Keys()[len(reference.Keys())-1].Value()
}

func historicalIdentifierCandidates(
	ctx context.Context,
	db *sql.DB,
	historyTable string,
	value string,
	at time.Time,
	budget *historicalDPPIdentityBudget,
) ([]string, error) {
	dataset, err := buildHistoricalIdentifierCandidatesDataset(historyTable, value, at)
	if err != nil {
		return nil, err
	}
	query, args, err := dataset.Prepared(true).ToSQL()
	if err != nil {
		return nil, common.NewInternalServerError("AASREPO-HISTORY-DPP-BUILDCANDIDATES " + err.Error())
	}
	rows, err := db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, common.NewInternalServerError("AASREPO-HISTORY-DPP-QUERYCANDIDATES " + err.Error())
	}
	defer func() { _ = rows.Close() }()
	identifiers := make([]string, 0)
	for rows.Next() {
		var identifier string
		if err = rows.Scan(&identifier); err != nil {
			return nil, common.NewInternalServerError("AASREPO-HISTORY-DPP-SCANCANDIDATE " + err.Error())
		}
		identifiers = append(identifiers, identifier)
	}
	if err = rows.Err(); err != nil {
		return nil, common.NewInternalServerError("AASREPO-HISTORY-DPP-ITERCANDIDATES " + err.Error())
	}
	if len(identifiers) > int(maxHistoricalDPPIdentityCandidates) {
		return nil, historicalDPPIdentityCandidatesExceeded()
	}
	if err = budget.consume(historyTable, identifiers); err != nil {
		return nil, err
	}
	return identifiers, nil
}

func (budget *historicalDPPIdentityBudget) consume(historyTable string, identifiers []string) error {
	for _, identifier := range identifiers {
		key := historyTable + "\x00" + identifier
		if _, exists := budget.seen[key]; exists {
			continue
		}
		budget.seen[key] = struct{}{}
		if len(budget.seen) > int(maxHistoricalDPPIdentityCandidates) {
			return historicalDPPIdentityCandidatesExceeded()
		}
	}
	return nil
}

func historicalDPPIdentityCandidatesExceeded() error {
	return common.NewErrConflict("AASREPO-HISTORY-DPP-TOOBROAD historical DPP identity matches too many candidate resources")
}

func buildHistoricalIdentifierCandidatesDataset(historyTable string, value string, at time.Time) (*goqu.SelectDataset, error) {
	payloadTable, err := historicalPayloadTable(historyTable)
	if err != nil {
		return nil, err
	}
	encodedValue, err := json.Marshal(value)
	if err != nil {
		return nil, common.NewInternalServerError("AASREPO-HISTORY-DPP-ENCODEJSONPATH " + err.Error())
	}
	jsonPath := "$.** == " + string(encodedValue)
	historyAlias := goqu.T(historyTable).As("history")
	payloadAlias := goqu.T(payloadTable).As("payload")
	return goqu.Dialect("postgres").From(historyAlias).
		InnerJoin(payloadAlias, goqu.On(historyAlias.Col("history_id").Eq(payloadAlias.Col("history_id")))).
		Select(historyAlias.Col("identifier")).
		Distinct().
		Where(
			historyAlias.Col("valid_from").Lte(at.UTC()),
			goqu.Or(
				historicalJSONPathMatch(payloadAlias.Col("snapshot"), jsonPath),
				historicalJSONPathMatch(payloadAlias.Col("diff"), jsonPath),
			),
		).
		Order(historyAlias.Col("identifier").Asc()).
		Limit(maxHistoricalDPPIdentityCandidates + 1), nil
}

func historicalJSONPathMatch(column exp.IdentifierExpression, jsonPath string) exp.LiteralExpression {
	return goqu.L("? @@ CAST(? AS jsonpath)", column, jsonPath)
}

func historicalPayloadTable(historyTable string) (string, error) {
	switch historyTable {
	case history.TableAAS:
		return "aas_history_payload", nil
	case history.TableSubmodel:
		return "submodel_history_payload", nil
	default:
		return "", common.NewInternalServerError("AASREPO-HISTORY-DPP-BADTABLE unsupported history table '" + historyTable + "'")
	}
}
