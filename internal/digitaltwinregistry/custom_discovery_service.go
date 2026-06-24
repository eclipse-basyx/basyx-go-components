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
*******************************************************************************/
// Author: Martin Stemmer ( Fraunhofer IESE )

// Package digitaltwinregistry package implements a custom discovery service for the Digital Twin Registry.
package digitaltwinregistry

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"strings"
	"time"

	"github.com/FriedJannik/aas-go-sdk/jsonization"
	"github.com/FriedJannik/aas-go-sdk/types"
	"github.com/eclipse-basyx/basyx-go-components/internal/common"
	"github.com/eclipse-basyx/basyx-go-components/internal/common/model"
	"github.com/eclipse-basyx/basyx-go-components/internal/common/model/grammar"
	auth "github.com/eclipse-basyx/basyx-go-components/internal/common/security"
	discoveryapiinternal "github.com/eclipse-basyx/basyx-go-components/internal/discoveryservice/api"
)

const customDiscoveryComponentName = "DTRDISC"
const publicReadableExternalSubjectValue = "PUBLIC_READABLE"

type aasExistenceChecker interface {
	ExistsAASByID(ctx context.Context, aasID string) (bool, error)
}

// CustomDiscoveryService wraps the default discovery service to allow custom logic.
type CustomDiscoveryService struct {
	*discoveryapiinternal.AssetAdministrationShellBasicDiscoveryAPIAPIService
	aasChecker aasExistenceChecker
}

// NewCustomDiscoveryService constructs a custom discovery service wrapper.
func NewCustomDiscoveryService(
	base *discoveryapiinternal.AssetAdministrationShellBasicDiscoveryAPIAPIService,
	checker aasExistenceChecker,
) *CustomDiscoveryService {
	return &CustomDiscoveryService{
		AssetAdministrationShellBasicDiscoveryAPIAPIService: base,
		aasChecker: baseCheckerOrFallback(checker),
	}
}

// SearchAllAssetAdministrationShellIdsByAssetLink Custom logic for /lookup/shellsbyAssetLink
func (s *CustomDiscoveryService) SearchAllAssetAdministrationShellIdsByAssetLink(
	ctx context.Context,
	limit int32,
	cursor string,
	assetLink []model.AssetLink,
) (model.ImplResponse, error) {
	if len(assetLink) == 0 {
		return model.Response(http.StatusOK, map[string]any{
			"paging_metadata": model.PagedResultPagingMetadata{},
		}), nil
	}

	shouldEnforceFormula, enforceErr := auth.ShouldEnforceFormula(ctx)
	if enforceErr != nil {
		log.Printf("🧭 [%s] Error SearchAllAssetAdministrationShellIdsByAssetLink: should-enforce-formula failed: %v", customDiscoveryComponentName, enforceErr)
		return common.NewErrorResponse(
			enforceErr,
			http.StatusInternalServerError,
			customDiscoveryComponentName,
			"SearchAllAssetAdministrationShellIdsByAssetLink",
			"ShouldEnforceFormula",
		), enforceErr
	}

	assetLinkQuery := buildDTRAssetLinkLookupQuery(ctx, assetLink, shouldEnforceFormula)
	if assetLinkQuery.Condition != nil || len(assetLinkQuery.FilterConditions) > 0 {
		ctx = auth.MergeQueryFilter(ctx, assetLinkQuery)
		ctx = discoveryapiinternal.WithAssetLinksAlreadyConstrained(ctx)
	}

	createdAfter, _ := CreatedAfterFromContext(ctx)
	if createdAfter != nil {
		createdAfterQuery := buildEdcBpnClaimEqualsHeaderExpression(createdAfter, "$bd#createdAt")
		ctx = auth.MergeQueryFilter(ctx, createdAfterQuery)
	}

	res, err := s.AssetAdministrationShellBasicDiscoveryAPIAPIService.SearchAllAssetAdministrationShellIdsByAssetLink(ctx, limit, cursor, assetLink)
	if err != nil {
		return res, err
	}

	return omitEmptySearchResultForDTR(res), nil
}

// GetAllAssetAdministrationShellIdsByAssetLink Custom logic for /lookup/shells
func (s *CustomDiscoveryService) GetAllAssetAdministrationShellIdsByAssetLink(
	ctx context.Context,
	assetIds []string,
	limit int32,
	cursor string,
) (model.ImplResponse, error) {
	links := make([]model.AssetLink, 0, len(assetIds))
	for idx, enc := range assetIds {
		if strings.TrimSpace(enc) == "" {
			continue
		}

		dec, err := common.DecodeString(enc)
		if err != nil {
			log.Printf("🧭 [%s] Error GetAllAssetAdministrationShellIdsByAssetLink: decode assetIds[%d]=%q failed: %v", customDiscoveryComponentName, idx, enc, err)
			return common.NewErrorResponse(
				err,
				http.StatusBadRequest,
				customDiscoveryComponentName,
				"GetAllAssetAdministrationShellIdsByAssetLink",
				"BadRequest-DecodeAssetIds",
			), nil
		}

		var al model.AssetLink
		if err := json.Unmarshal([]byte(dec), &al); err != nil {
			log.Printf("🧭 [%s] Error GetAllAssetAdministrationShellIdsByAssetLink: unmarshal assetIds[%d] decoded=%q failed: %v", customDiscoveryComponentName, idx, dec, err)
			return common.NewErrorResponse(
				err,
				http.StatusBadRequest,
				customDiscoveryComponentName,
				"GetAllAssetAdministrationShellIdsByAssetLink",
				"BadRequest-UnmarshalAssetIds",
			), nil
		}

		links = append(links, al)
	}

	if len(links) == 0 {
		return model.Response(http.StatusOK, map[string]any{
			"paging_metadata": model.PagedResultPagingMetadata{},
		}), nil
	}

	return s.SearchAllAssetAdministrationShellIdsByAssetLink(ctx, limit, cursor, links)
}

func omitEmptySearchResultForDTR(res model.ImplResponse) model.ImplResponse {
	if res.Code != http.StatusOK {
		return res
	}

	switch body := res.Body.(type) {
	case model.GetAllAssetAdministrationShellIdsByAssetLink200Response:
		if len(body.Result) != 0 {
			return res
		}
		return model.Response(http.StatusOK, map[string]any{
			"paging_metadata": body.PagingMetadata,
		})
	case *model.GetAllAssetAdministrationShellIdsByAssetLink200Response:
		if body == nil || len(body.Result) != 0 {
			return res
		}
		return model.Response(http.StatusOK, map[string]any{
			"paging_metadata": body.PagingMetadata,
		})
	default:
		return res
	}
}

func baseCheckerOrFallback(checker aasExistenceChecker) aasExistenceChecker {
	if checker != nil {
		return checker
	}
	return noopAASChecker{}
}

type noopAASChecker struct{}

func (noopAASChecker) ExistsAASByID(_ context.Context, _ string) (bool, error) {
	return true, nil
}

// PostAllAssetLinksByID Custom logic for /lookup/shells/{aasIdentifier}
// in DTR: update/merge instead of replace semantics.
func (s *CustomDiscoveryService) PostAllAssetLinksByID(
	ctx context.Context,
	aasIdentifier string,
	specificAssetID []types.ISpecificAssetID,
) (model.ImplResponse, error) {
	decoded, decodeErr := common.DecodeString(aasIdentifier)
	if decodeErr != nil {
		log.Printf("🧭 [%s] Error PostAllAssetLinksById: decode aasIdentifier=%q failed: %v", customDiscoveryComponentName, aasIdentifier, decodeErr)
		return common.NewErrorResponse(
			decodeErr,
			http.StatusBadRequest,
			customDiscoveryComponentName,
			"PostAllAssetLinksById",
			"BadRequest-Decode",
		), nil
	}

	aasID := string(decoded)
	exists, existsErr := s.aasChecker.ExistsAASByID(ctx, aasID)
	if existsErr != nil {
		log.Printf("🧭 [%s] Error PostAllAssetLinksById: existence check failed (aasId=%q): %v", customDiscoveryComponentName, aasID, existsErr)
		return common.NewErrorResponse(
			existsErr,
			http.StatusInternalServerError,
			customDiscoveryComponentName,
			"PostAllAssetLinksById",
			"AAS-ExistenceCheck",
		), existsErr
	}
	if !exists {
		notFoundErr := common.NewErrNotFound(fmt.Sprintf("Shell for identifier %s not found", aasID))
		return common.NewErrorResponse(
			notFoundErr,
			http.StatusNotFound,
			customDiscoveryComponentName,
			"PostAllAssetLinksById",
			"NotFound",
		), nil
	}

	persistResp, persistErr := s.AddAllAssetLinksByID(ctx, aasIdentifier, specificAssetID)
	if persistErr != nil {
		return persistResp, persistErr
	}
	if persistResp.Code < http.StatusOK || persistResp.Code >= http.StatusMultipleChoices {
		return persistResp, nil
	}

	jsonableIncoming, convErr := specificAssetIDsToJSONable(specificAssetID)
	if convErr != nil {
		log.Printf("🧭 [%s] Error PostAllAssetLinksById: convert incoming links failed (aasId=%q): %v", customDiscoveryComponentName, aasID, convErr)
		return common.NewErrorResponse(
			convErr,
			http.StatusInternalServerError,
			customDiscoveryComponentName,
			"PostAllAssetLinksById",
			"JsonConversion",
		), convErr
	}
	return model.Response(http.StatusCreated, jsonableIncoming), nil
}

func specificAssetIDsToJSONable(specificAssetIDs []types.ISpecificAssetID) ([]map[string]any, error) {
	out := make([]map[string]any, 0, len(specificAssetIDs))
	for _, link := range specificAssetIDs {
		jsonableLink, err := jsonization.ToJsonable(link)
		if err != nil {
			return nil, err
		}
		out = append(out, jsonableLink)
	}
	return out, nil
}

// buildEdcBpnClaimEqualsHeaderExpression creates a logical expression that checks
// whether the Edc-Bpn claim equals the provided header value.
func buildEdcBpnClaimEqualsHeaderExpression(t *time.Time, pattern string) grammar.Query {
	dt := grammar.DateTimeLiteralPattern(t.UTC())

	timePattern := grammar.ModelStringPattern(pattern)
	timeLe := grammar.LogicalExpression{
		Le: grammar.ComparisonItems{
			{DateTimeVal: &dt},
			{Field: &timePattern},
		},
	}

	return grammar.Query{
		Condition: &timeLe,
	}
}

// buildAssetLinkQuery creates the discovery lookup filter for asset links.
//
// Behavior:
//   - Returns an empty query when no asset links are provided.
//   - Returns an empty query when ABAC READ access is already unrestricted.
//   - Otherwise builds an AND of per-link conditions.
//
// Per link, the condition is an OR:
//   - exact link match + Edc-Bpn authorization (when Edc-Bpn claim exists), or
//   - exact link match + PUBLIC_READABLE authorization.
//
// globalAssetId links are handled separately and require an Edc-Bpn match when
// authorization is enforced.
func buildAssetLinkQuery(ctx context.Context, assetLink []model.AssetLink) grammar.Query {
	if len(assetLink) == 0 {
		return grammar.Query{}
	}

	if auth.HasUnrestrictedFormulaForRight(ctx, grammar.RightsEnumREAD) {
		return grammar.Query{}
	}

	return buildDTRAssetLinkLookupQuery(ctx, assetLink, true)
}

func buildDTRAssetLinkLookupQuery(ctx context.Context, assetLink []model.AssetLink, enforceAuthorization bool) grammar.Query {
	if len(assetLink) == 0 {
		return grammar.Query{}
	}

	edcBpnClaim, hasEdcBpnClaim := edcBpnClaimFromContext(ctx)
	requireAuthorization := enforceAuthorization && !auth.HasUnrestrictedFormulaForRight(ctx, grammar.RightsEnumREAD)
	assetLinkLe := grammar.LogicalExpression{And: make([]grammar.LogicalExpression, 0, len(assetLink))}
	for _, link := range assetLink {
		assetLinkLe.And = append(assetLinkLe.And, buildAssetLinkCondition(link, edcBpnClaim, hasEdcBpnClaim, requireAuthorization))
	}

	return grammar.Query{
		Condition: &assetLinkLe,
	}
}

func edcBpnClaimFromContext(ctx context.Context) (string, bool) {
	claims := auth.ClaimsFromContext(ctx)
	edcBpnClaim, hasEdcBpnClaim := claims.GetString("Edc-Bpn")
	return edcBpnClaim, hasEdcBpnClaim && strings.TrimSpace(edcBpnClaim) != ""
}

func buildAssetLinkCondition(link model.AssetLink, edcBpnClaim string, hasEdcBpnClaim bool, requireAuthorization bool) grammar.LogicalExpression {
	if link.Name == common.GlobalAssetIDAssetLinkName {
		return buildGlobalAssetIDAssetLinkCondition(link.Value, edcBpnClaim, hasEdcBpnClaim, requireAuthorization)
	}
	return buildSpecificAssetLinkCondition(link, edcBpnClaim, hasEdcBpnClaim, requireAuthorization)
}

func buildSpecificAssetLinkCondition(link model.AssetLink, edcBpnClaim string, hasEdcBpnClaim bool, requireAuthorization bool) grammar.LogicalExpression {
	matches := []grammar.MatchExpression{
		eqFieldToStringMatch("$aasdesc#specificAssetIds[].value", link.Value),
		eqFieldToStringMatch("$aasdesc#specificAssetIds[].name", link.Name),
	}
	if !requireAuthorization {
		return grammar.LogicalExpression{Match: matches}
	}

	return grammar.LogicalExpression{
		Or: authorizedSubjectExpressions(edcBpnClaim, hasEdcBpnClaim, func(subject string) grammar.LogicalExpression {
			subjectMatches := append([]grammar.MatchExpression(nil), matches...)
			subjectMatches = append(subjectMatches, eqStringToFieldMatch(subject, "$aasdesc#specificAssetIds[].externalSubjectId.keys[].value"))
			return grammar.LogicalExpression{
				Match: subjectMatches,
			}
		}),
	}
}

func buildGlobalAssetIDAssetLinkCondition(value string, edcBpnClaim string, hasEdcBpnClaim bool, requireAuthorization bool) grammar.LogicalExpression {
	globalAssetIDCondition := eqFieldToStringExpression("$aasdesc#globalAssetId", value)
	if !requireAuthorization {
		return globalAssetIDCondition
	}

	return grammar.LogicalExpression{
		And: []grammar.LogicalExpression{
			globalAssetIDCondition,
			globalAssetIDBPNAuthorizationCondition(edcBpnClaim, hasEdcBpnClaim),
		},
	}
}

func globalAssetIDBPNAuthorizationCondition(edcBpnClaim string, hasEdcBpnClaim bool) grammar.LogicalExpression {
	if !hasEdcBpnClaim {
		b := false
		return grammar.LogicalExpression{Boolean: &b}
	}
	return grammar.LogicalExpression{
		Match: []grammar.MatchExpression{
			eqStringToFieldMatch(edcBpnClaim, "$aasdesc#specificAssetIds[].externalSubjectId.keys[].value"),
		},
	}
}

func authorizedSubjectExpressions(
	edcBpnClaim string,
	hasEdcBpnClaim bool,
	build func(string) grammar.LogicalExpression,
) []grammar.LogicalExpression {
	expressions := make([]grammar.LogicalExpression, 0, 2)
	if hasEdcBpnClaim {
		expressions = append(expressions, build(edcBpnClaim))
	}
	return append(expressions, build(publicReadableExternalSubjectValue))
}

func eqFieldToStringExpression(fieldPath string, value string) grammar.LogicalExpression {
	field := grammar.ModelStringPattern(fieldPath)
	strValue := grammar.StandardString(value)
	return grammar.LogicalExpression{
		Eq: grammar.ComparisonItems{
			{Field: &field},
			{StrVal: &strValue},
		},
	}
}

func eqFieldToStringMatch(fieldPath string, value string) grammar.MatchExpression {
	field := grammar.ModelStringPattern(fieldPath)
	strValue := grammar.StandardString(value)
	return grammar.MatchExpression{
		Eq: grammar.ComparisonItems{
			{Field: &field},
			{StrVal: &strValue},
		},
	}
}

func eqStringToFieldMatch(value string, fieldPath string) grammar.MatchExpression {
	field := grammar.ModelStringPattern(fieldPath)
	strValue := grammar.StandardString(value)
	return grammar.MatchExpression{
		Eq: grammar.ComparisonItems{
			{StrVal: &strValue},
			{Field: &field},
		},
	}
}
