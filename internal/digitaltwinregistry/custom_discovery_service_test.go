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

package digitaltwinregistry

import (
	"context"
	"fmt"
	"net/http"
	"strings"
	"testing"

	sqlmock "github.com/DATA-DOG/go-sqlmock"
	"github.com/doug-martin/goqu/v9"
	"github.com/eclipse-basyx/basyx-go-components/internal/common"
	"github.com/eclipse-basyx/basyx-go-components/internal/common/model"
	"github.com/eclipse-basyx/basyx-go-components/internal/common/model/grammar"
	auth "github.com/eclipse-basyx/basyx-go-components/internal/common/security"
	discoveryapiinternal "github.com/eclipse-basyx/basyx-go-components/internal/discoveryservice/api"
	persistencepostgresql "github.com/eclipse-basyx/basyx-go-components/internal/discoveryservice/persistence"
)

func TestBuildAssetLinkAuthorizationQuery_ReturnsEmptyWhenReadFormulaIsUnrestricted(t *testing.T) {
	t.Parallel()

	b := true
	ctx := auth.MergeQueryFilter(context.Background(), grammar.Query{
		Condition: &grammar.LogicalExpression{Boolean: &b},
	})

	query := buildAssetLinkAuthorizationQuery(ctx, nil, []model.AssetLink{{Name: "name", Value: "value"}})
	if query.Condition != nil {
		t.Fatalf("expected no additional condition when READ formula is unrestricted, got %#v", query.Condition)
	}
}

func TestBuildAssetLinkAuthorizationQuery_BuildsConditionWhenReadFormulaIsRestricted(t *testing.T) {
	t.Parallel()

	b := false
	ctx := auth.MergeQueryFilter(context.Background(), grammar.Query{
		Condition: &grammar.LogicalExpression{Boolean: &b},
	})

	query := buildAssetLinkAuthorizationQuery(ctx, nil, []model.AssetLink{{Name: "name", Value: "value"}})
	if query.Condition == nil {
		t.Fatalf("expected asset-link condition when READ formula is restricted")
	}
	if len(query.Condition.And) == 0 {
		t.Fatalf("expected AND conditions for asset-link query, got %#v", query.Condition)
	}
}

func TestBuildAssetLinkAuthorizationQuery_GlobalAssetIDUsesTwinExternalSubjects(t *testing.T) {
	t.Parallel()

	ctx := restrictedReadContext()
	ctx = context.WithValue(ctx, auth.ClaimsKey, auth.Claims{"Edc-Bpn": "BPN_COMPANY_001"})

	query := buildAssetLinkAuthorizationQuery(ctx, []model.AssetLink{
		{Name: common.GlobalAssetIDAssetLinkName, Value: "global-asset"},
	}, nil)
	if query.Condition == nil {
		t.Fatalf("expected globalAssetId authorization condition")
	}

	sql, args := renderBDQuerySQL(t, query)

	if strings.Contains(sql, `"aas_descriptor"."global_asset_id" =`) || containsArg(args, "global-asset") {
		t.Fatalf("did not expect globalAssetId authorization to duplicate lookup identity, got SQL: %s", sql)
	}
	if !strings.Contains(sql, "EXISTS") || !strings.Contains(sql, `"external_subject_reference_key"`) {
		t.Fatalf("expected external-subject EXISTS authorization, got SQL: %s", sql)
	}
	if !containsArg(args, "BPN_COMPANY_001") {
		t.Fatalf("expected globalAssetId lookup to authorize through the caller Edc-Bpn, got SQL: %s", sql)
	}
	if strings.Contains(sql, `"specific_asset_id"."name"`) {
		t.Fatalf("did not expect globalAssetId visibility to depend on generated specific_asset_id row, got SQL: %s", sql)
	}
	if containsArg(args, publicReadableExternalSubjectValue) {
		t.Fatalf("globalAssetId lookup must not be authorized through PUBLIC_READABLE specific asset IDs, got SQL: %s", sql)
	}
}

func TestBuildAssetLinkAuthorizationQuery_GlobalAssetIDDoesNotAllowAnonymousPublicReadableSubject(t *testing.T) {
	t.Parallel()

	query := buildAssetLinkAuthorizationQuery(restrictedReadContext(), []model.AssetLink{
		{Name: common.GlobalAssetIDAssetLinkName, Value: "global-asset"},
	}, nil)
	if query.Condition == nil {
		t.Fatalf("expected globalAssetId authorization condition")
	}

	sql, args := renderBDQuerySQL(t, query)

	if strings.Contains(sql, `"aas_descriptor"."global_asset_id" =`) || containsArg(args, "global-asset") {
		t.Fatalf("did not expect globalAssetId authorization to duplicate lookup identity, got SQL: %s", sql)
	}
	if containsArg(args, publicReadableExternalSubjectValue) {
		t.Fatalf("anonymous globalAssetId lookup must not be authorized through PUBLIC_READABLE specific asset IDs, got SQL: %s", sql)
	}
}

func TestBuildGlobalAssetIDLookupQuery_UsesDescriptorValueWithoutAssetLinkFallback(t *testing.T) {
	t.Parallel()

	query := buildGlobalAssetIDLookupQuery([]model.AssetLink{
		{Name: common.GlobalAssetIDAssetLinkName, Value: "global-asset"},
	})
	if query.Condition == nil {
		t.Fatalf("expected globalAssetId condition")
	}

	sql, args := renderBDQuerySQL(t, query)

	if !strings.Contains(sql, `"aas_descriptor"."global_asset_id" =`) || !containsArg(args, "global-asset") {
		t.Fatalf("expected direct global_asset_id comparison, got SQL: %s", sql)
	}
	if strings.Contains(sql, `"specific_asset_id"."name"`) {
		t.Fatalf("did not expect generated globalAssetId asset-link fallback, got SQL: %s", sql)
	}
	if strings.Contains(sql, `"external_subject_reference_key"`) {
		t.Fatalf("did not expect external-subject authorization in lookup-only query, got SQL: %s", sql)
	}
}

func TestSearchAllAssetAdministrationShellIdsByAssetLink_WhenFormulaDisabledUsesDTRGlobalAssetIDFilter(t *testing.T) {
	t.Parallel()

	matcher := sqlmock.QueryMatcherFunc(func(_ string, actualSQL string) error {
		if !strings.Contains(actualSQL, `"aas_descriptor"."global_asset_id" =`) {
			return fmt.Errorf("expected direct global_asset_id lookup, got SQL: %s", actualSQL)
		}
		if strings.Contains(actualSQL, "OR EXISTS") {
			return fmt.Errorf("did not expect backend globalAssetId fallback OR, got SQL: %s", actualSQL)
		}
		if strings.Contains(actualSQL, `"sai"."name" =`) {
			return fmt.Errorf("did not expect generated globalAssetId asset-link fallback, got SQL: %s", actualSQL)
		}
		return nil
	})
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(matcher))
	if err != nil {
		t.Fatalf("failed to create sqlmock: %v", err)
	}
	defer func() { _ = db.Close() }()

	backend, err := persistencepostgresql.NewPostgreSQLDiscoveryBackendFromDB(db)
	if err != nil {
		t.Fatalf("failed to create backend: %v", err)
	}
	base := discoveryapiinternal.NewAssetAdministrationShellBasicDiscoveryAPIAPIService(*backend)
	service := NewCustomDiscoveryService(base, nil)

	rows := sqlmock.NewRows([]string{"aasid"}).AddRow("urn:aas:test:global")
	mock.ExpectQuery("global asset id lookup").WillReturnRows(rows)

	ctx := common.ContextWithConfig(context.Background(), &common.Config{})
	response, searchErr := service.SearchAllAssetAdministrationShellIdsByAssetLink(
		ctx,
		100,
		"",
		[]model.AssetLink{{Name: common.GlobalAssetIDAssetLinkName, Value: "global-asset"}},
	)
	if searchErr != nil {
		t.Fatalf("expected search to succeed: %v", searchErr)
	}
	if response.Code != http.StatusOK {
		t.Fatalf("expected status %d, got %d", http.StatusOK, response.Code)
	}

	body, ok := response.Body.(model.GetAllAssetAdministrationShellIdsByAssetLink200Response)
	if !ok {
		t.Fatalf("expected response body type GetAllAssetAdministrationShellIdsByAssetLink200Response, got %T", response.Body)
	}
	if len(body.Result) != 1 || body.Result[0] != "urn:aas:test:global" {
		t.Fatalf("expected global AAS id result, got %#v", body.Result)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("expected query to be executed: %v", err)
	}
}

func TestSearchAllAssetAdministrationShellIdsByAssetLink_WhenFormulaEnabledDoesNotDuplicateGlobalAssetID(t *testing.T) {
	t.Parallel()

	matcher := sqlmock.QueryMatcherFunc(func(_ string, actualSQL string) error {
		if count := strings.Count(actualSQL, `"aas_descriptor"."global_asset_id" =`); count != 1 {
			return fmt.Errorf("expected one direct global_asset_id lookup, got %d in SQL: %s", count, actualSQL)
		}
		if strings.Contains(actualSQL, "OR EXISTS") {
			return fmt.Errorf("did not expect backend globalAssetId fallback OR, got SQL: %s", actualSQL)
		}
		if strings.Contains(actualSQL, `"sai"."name" =`) {
			return fmt.Errorf("did not expect generated globalAssetId asset-link fallback, got SQL: %s", actualSQL)
		}
		if !strings.Contains(actualSQL, `"external_subject_reference_key"`) {
			return fmt.Errorf("expected globalAssetId authorization through external subject references, got SQL: %s", actualSQL)
		}
		return nil
	})
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(matcher))
	if err != nil {
		t.Fatalf("failed to create sqlmock: %v", err)
	}
	defer func() { _ = db.Close() }()

	backend, err := persistencepostgresql.NewPostgreSQLDiscoveryBackendFromDB(db)
	if err != nil {
		t.Fatalf("failed to create backend: %v", err)
	}
	base := discoveryapiinternal.NewAssetAdministrationShellBasicDiscoveryAPIAPIService(*backend)
	service := NewCustomDiscoveryService(base, nil)

	rows := sqlmock.NewRows([]string{"aasid"}).AddRow("urn:aas:test:global")
	mock.ExpectQuery("secured global asset id lookup").WillReturnRows(rows)

	ctx := restrictedSearchContext()
	ctx = context.WithValue(ctx, auth.ClaimsKey, auth.Claims{"Edc-Bpn": "BPN_COMPANY_001"})
	response, searchErr := service.SearchAllAssetAdministrationShellIdsByAssetLink(
		ctx,
		100,
		"",
		[]model.AssetLink{{Name: common.GlobalAssetIDAssetLinkName, Value: "global-asset"}},
	)
	if searchErr != nil {
		t.Fatalf("expected search to succeed: %v", searchErr)
	}
	if response.Code != http.StatusOK {
		t.Fatalf("expected status %d, got %d", http.StatusOK, response.Code)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("expected query to be executed: %v", err)
	}
}

func TestSearchAllAssetAdministrationShellIdsByAssetLink_WhenFormulaDisabledKeepsNormalLinksInBackend(t *testing.T) {
	t.Parallel()

	matcher := sqlmock.QueryMatcherFunc(func(_ string, actualSQL string) error {
		if !strings.Contains(actualSQL, `"aas_descriptor"."global_asset_id" =`) {
			return fmt.Errorf("expected direct global_asset_id lookup, got SQL: %s", actualSQL)
		}
		if !strings.Contains(actualSQL, "EXISTS") || !strings.Contains(actualSQL, `"sai"."name" =`) {
			return fmt.Errorf("expected normal asset-link backend EXISTS, got SQL: %s", actualSQL)
		}
		if !strings.Contains(actualSQL, "customerPartId") {
			return fmt.Errorf("expected normal asset-link name in backend query, got SQL: %s", actualSQL)
		}
		if strings.Contains(actualSQL, "globalAssetId") {
			return fmt.Errorf("did not expect globalAssetId to be passed as backend asset link, got SQL: %s", actualSQL)
		}
		return nil
	})
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(matcher))
	if err != nil {
		t.Fatalf("failed to create sqlmock: %v", err)
	}
	defer func() { _ = db.Close() }()

	backend, err := persistencepostgresql.NewPostgreSQLDiscoveryBackendFromDB(db)
	if err != nil {
		t.Fatalf("failed to create backend: %v", err)
	}
	base := discoveryapiinternal.NewAssetAdministrationShellBasicDiscoveryAPIAPIService(*backend)
	service := NewCustomDiscoveryService(base, nil)

	rows := sqlmock.NewRows([]string{"aasid"}).AddRow("urn:aas:test:mixed")
	mock.ExpectQuery("mixed asset link lookup").WillReturnRows(rows)

	ctx := common.ContextWithConfig(context.Background(), &common.Config{})
	response, searchErr := service.SearchAllAssetAdministrationShellIdsByAssetLink(
		ctx,
		100,
		"",
		[]model.AssetLink{
			{Name: common.GlobalAssetIDAssetLinkName, Value: "global-asset"},
			{Name: "customerPartId", Value: "4711"},
		},
	)
	if searchErr != nil {
		t.Fatalf("expected search to succeed: %v", searchErr)
	}
	if response.Code != http.StatusOK {
		t.Fatalf("expected status %d, got %d", http.StatusOK, response.Code)
	}

	body, ok := response.Body.(model.GetAllAssetAdministrationShellIdsByAssetLink200Response)
	if !ok {
		t.Fatalf("expected response body type GetAllAssetAdministrationShellIdsByAssetLink200Response, got %T", response.Body)
	}
	if len(body.Result) != 1 || body.Result[0] != "urn:aas:test:mixed" {
		t.Fatalf("expected mixed AAS id result, got %#v", body.Result)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("expected query to be executed: %v", err)
	}
}

func renderBDQuerySQL(t *testing.T, query grammar.Query) (string, []interface{}) {
	t.Helper()

	collector, err := grammar.NewResolvedFieldPathCollectorForRoot(grammar.CollectorRootBD)
	if err != nil {
		t.Fatalf("failed to build collector: %v", err)
	}
	expr, _, err := query.Condition.EvaluateToExpression(collector)
	if err != nil {
		t.Fatalf("expected globalAssetId query to compile: %v", err)
	}

	sql, args, err := goqu.Dialect(common.Dialect).
		From(goqu.T(common.TblAASIdentifier)).
		LeftJoin(
			goqu.T(common.TblAASDescriptor),
			goqu.On(goqu.I(common.TblAASDescriptor+"."+common.ColAASID).Eq(goqu.I(common.TblAASIdentifier+".aasid"))),
		).
		Select(goqu.V(1)).
		Where(expr).
		Prepared(true).
		ToSQL()
	if err != nil {
		t.Fatalf("expected SQL generation to succeed: %v", err)
	}
	return sql, args
}

func restrictedReadContext() context.Context {
	b := false
	return auth.MergeQueryFilter(context.Background(), grammar.Query{
		Condition: &grammar.LogicalExpression{Boolean: &b},
	})
}

func restrictedSearchContext() context.Context {
	ctx := common.ContextWithConfig(context.Background(), &common.Config{})
	condition := eqFieldToStringExpression("$aasdesc#idShort", "visible")
	return auth.MergeQueryFilter(ctx, grammar.Query{Condition: &condition})
}

func containsArg(args []interface{}, want string) bool {
	for _, arg := range args {
		if arg == want {
			return true
		}
	}
	return false
}
