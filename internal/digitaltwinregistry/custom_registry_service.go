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

package digitaltwinregistry

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"math"
	"net/http"
	"strings"
	"time"

	registryapiinternal "github.com/eclipse-basyx/basyx-go-components/internal/aasregistry/api"
	"github.com/eclipse-basyx/basyx-go-components/internal/common"
	"github.com/eclipse-basyx/basyx-go-components/internal/common/asyncbulk"
	descriptorsutil "github.com/eclipse-basyx/basyx-go-components/internal/common/descriptors"
	"github.com/eclipse-basyx/basyx-go-components/internal/common/model"
	"github.com/eclipse-basyx/basyx-go-components/internal/common/model/grammar"
	auth "github.com/eclipse-basyx/basyx-go-components/internal/common/security"
	discoveryapiinternal "github.com/eclipse-basyx/basyx-go-components/internal/discoveryservice/api"
)

const customRegistryComponentName = "DTRREG"

// CustomRegistryService wraps the default registry service to allow custom logic.
type CustomRegistryService struct {
	*registryapiinternal.AssetAdministrationShellRegistryAPIAPIService
	discovery *discoveryapiinternal.AssetAdministrationShellBasicDiscoveryAPIAPIService
}

// NewCustomRegistryService constructs a custom registry service wrapper.
func NewCustomRegistryService(
	base *registryapiinternal.AssetAdministrationShellRegistryAPIAPIService,
	discovery *discoveryapiinternal.AssetAdministrationShellBasicDiscoveryAPIAPIService,
) *CustomRegistryService {
	return &CustomRegistryService{
		AssetAdministrationShellRegistryAPIAPIService: base,
		discovery: discovery,
	}
}

// GetAllAssetAdministrationShellDescriptors - Returns all Asset Administration Shell Descriptors
func (s *CustomRegistryService) GetAllAssetAdministrationShellDescriptors(
	ctx context.Context,
	limit int32,
	cursor string,
	assetKind model.AssetKind,
	assetType string,
	assetIds []string,
	createdFrom time.Time,
	updatedFrom time.Time,
) (model.ImplResponse, error) {
	createdAfter, _ := CreatedAfterFromContext(ctx)
	if createdAfter != nil {
		query := buildEdcBpnClaimEqualsHeaderExpression(createdAfter, "$aasdesc#createdAt")
		ctx = auth.MergeQueryFilter(ctx, query)
	}
	ctx = descriptorsutil.WithIncludeAASDescriptorCreatedAt(ctx)

	if len(assetIds) > 0 {
		links, resp, err := decodeRegistryAssetLinkQueryAssetIDs(assetIds)
		if resp != nil || err != nil {
			return *resp, err
		}
		return s.getAllAssetAdministrationShellDescriptorsByAssetLinks(
			ctx,
			limit,
			cursor,
			assetKind,
			assetType,
			links,
			createdFrom,
			updatedFrom,
		)
	}

	return s.AssetAdministrationShellRegistryAPIAPIService.GetAllAssetAdministrationShellDescriptors(
		ctx,
		limit,
		cursor,
		assetKind,
		assetType,
		assetIds,
		createdFrom,
		updatedFrom,
	)
}

func (s *CustomRegistryService) getAllAssetAdministrationShellDescriptorsByAssetLinks(
	ctx context.Context,
	limit int32,
	cursor string,
	assetKind model.AssetKind,
	assetType string,
	links []model.AssetLink,
	createdFrom time.Time,
	updatedFrom time.Time,
) (model.ImplResponse, error) {
	if len(links) == 0 {
		return emptyDescriptorPage(), nil
	}

	lookupCtx, lookupErr := mergeAssetLinkLookupFilter(ctx, links)
	if lookupErr != nil {
		return common.NewErrorResponse(
			lookupErr,
			http.StatusInternalServerError,
			customRegistryComponentName,
			"GetAllAssetAdministrationShellDescriptors",
			"AssetLinkFilter",
		), lookupErr
	}

	lookupResp, lookupErr := s.discovery.SearchAllAssetAdministrationShellIdsByAssetLink(lookupCtx, limit, cursor, links)
	if lookupErr != nil || lookupResp.Code != http.StatusOK {
		return lookupResp, lookupErr
	}

	aasIDs, paging, extractErr := extractLookupAASIDs(lookupResp)
	if extractErr != nil {
		return common.NewErrorResponse(
			extractErr,
			http.StatusInternalServerError,
			customRegistryComponentName,
			"GetAllAssetAdministrationShellDescriptors",
			"LookupResponse",
		), extractErr
	}
	if len(aasIDs) == 0 {
		return model.Response(http.StatusOK, map[string]any{
			"paging_metadata": paging,
		}), nil
	}
	descriptorLimit, limitErr := int32Length(len(aasIDs))
	if limitErr != nil {
		return common.NewErrorResponse(
			limitErr,
			http.StatusInternalServerError,
			customRegistryComponentName,
			"GetAllAssetAdministrationShellDescriptors",
			"LookupResultLimit",
		), limitErr
	}

	descriptorQuery := buildAASIDQuery(aasIDs)
	descriptorCtx := auth.MergeQueryFilter(ctx, descriptorQuery)
	descriptorResp, descriptorErr := s.AssetAdministrationShellRegistryAPIAPIService.GetAllAssetAdministrationShellDescriptors(
		descriptorCtx,
		descriptorLimit,
		"",
		assetKind,
		assetType,
		nil,
		createdFrom,
		updatedFrom,
	)
	if descriptorErr != nil || descriptorResp.Code != http.StatusOK {
		return descriptorResp, descriptorErr
	}

	return replaceDescriptorPagingMetadata(descriptorResp, paging)
}

func int32Length(length int) (int32, error) {
	if length > math.MaxInt32 {
		return 0, fmt.Errorf("length %d exceeds int32 max %d", length, math.MaxInt32)
	}
	// #nosec G115 -- length is checked against math.MaxInt32 above.
	return int32(length), nil
}

func decodeRegistryAssetLinkQueryAssetIDs(encodedAssetIDs []string) ([]model.AssetLink, *model.ImplResponse, error) {
	links := make([]model.AssetLink, 0, len(encodedAssetIDs))
	for idx, encodedAssetID := range encodedAssetIDs {
		if strings.TrimSpace(encodedAssetID) == "" {
			continue
		}

		decoded, err := common.DecodeString(encodedAssetID)
		if err != nil {
			log.Printf("[%s] Error GetAllAssetAdministrationShellDescriptors: decode assetIds[%d]=%q failed: %v", customRegistryComponentName, idx, encodedAssetID, err)
			resp := common.NewErrorResponse(
				err,
				http.StatusBadRequest,
				customRegistryComponentName,
				"GetAllAssetAdministrationShellDescriptors",
				"BadRequest-DecodeAssetIds",
			)
			return nil, &resp, nil
		}

		var link model.AssetLink
		if err := json.Unmarshal([]byte(decoded), &link); err != nil {
			log.Printf("[%s] Error GetAllAssetAdministrationShellDescriptors: unmarshal assetIds[%d] decoded=%q failed: %v", customRegistryComponentName, idx, decoded, err)
			resp := common.NewErrorResponse(
				err,
				http.StatusBadRequest,
				customRegistryComponentName,
				"GetAllAssetAdministrationShellDescriptors",
				"BadRequest-UnmarshalAssetIds",
			)
			return nil, &resp, nil
		}

		links = append(links, link)
	}

	return links, nil, nil
}

func mergeAssetLinkLookupFilter(ctx context.Context, links []model.AssetLink) (context.Context, error) {
	shouldEnforceFormula, enforceErr := auth.ShouldEnforceFormula(ctx)
	if enforceErr != nil {
		return ctx, enforceErr
	}
	if !shouldEnforceFormula {
		return ctx, nil
	}

	assetLinkQuery := buildAssetLinkQuery(ctx, links)
	if assetLinkQuery.Condition == nil && len(assetLinkQuery.FilterConditions) == 0 {
		return ctx, nil
	}

	return discoveryapiinternal.WithAssetLinksAlreadyConstrained(auth.MergeQueryFilter(ctx, assetLinkQuery)), nil
}

func buildAASIDQuery(aasIDs []string) grammar.Query {
	if len(aasIDs) == 0 {
		return grammar.Query{}
	}

	aasIDField := grammar.ModelStringPattern("$aasdesc#id")
	condition := grammar.LogicalExpression{Or: make([]grammar.LogicalExpression, 0, len(aasIDs))}
	for _, aasID := range aasIDs {
		aasIDValue := grammar.StandardString(aasID)
		condition.Or = append(condition.Or, grammar.LogicalExpression{
			Eq: grammar.ComparisonItems{
				{Field: &aasIDField},
				{StrVal: &aasIDValue},
			},
		})
	}

	return grammar.Query{Condition: &condition}
}

func extractLookupAASIDs(resp model.ImplResponse) ([]string, model.PagedResultPagingMetadata, error) {
	switch body := resp.Body.(type) {
	case model.GetAllAssetAdministrationShellIdsByAssetLink200Response:
		return body.Result, body.PagingMetadata, nil
	case *model.GetAllAssetAdministrationShellIdsByAssetLink200Response:
		if body == nil {
			return nil, model.PagedResultPagingMetadata{}, nil
		}
		return body.Result, body.PagingMetadata, nil
	default:
		payload, err := json.Marshal(body)
		if err != nil {
			return nil, model.PagedResultPagingMetadata{}, err
		}
		var decoded model.GetAllAssetAdministrationShellIdsByAssetLink200Response
		if err = json.Unmarshal(payload, &decoded); err != nil {
			return nil, model.PagedResultPagingMetadata{}, err
		}
		return decoded.Result, decoded.PagingMetadata, nil
	}
}

func replaceDescriptorPagingMetadata(resp model.ImplResponse, paging model.PagedResultPagingMetadata) (model.ImplResponse, error) {
	payload, err := json.Marshal(resp.Body)
	if err != nil {
		return model.ImplResponse{}, err
	}
	var decoded struct {
		PagingMetadata model.PagedResultPagingMetadata `json:"paging_metadata"`
		Result         []map[string]any                `json:"result"`
	}
	if err = json.Unmarshal(payload, &decoded); err != nil {
		return model.ImplResponse{}, err
	}
	return model.Response(http.StatusOK, map[string]any{
		"paging_metadata": paging,
		"result":          decoded.Result,
	}), nil
}

func emptyDescriptorPage() model.ImplResponse {
	return model.Response(http.StatusOK, map[string]any{
		"paging_metadata": model.PagedResultPagingMetadata{},
	})
}

// GetAssetAdministrationShellDescriptorById - Returns a specific Asset Administration Shell Descriptor
// nolint:revive // defined by standard
func (s *CustomRegistryService) GetAssetAdministrationShellDescriptorById(
	ctx context.Context,
	aasIdentifier string,
) (model.ImplResponse, error) {
	ctx = descriptorsutil.WithIncludeAASDescriptorCreatedAt(ctx)
	return s.AssetAdministrationShellRegistryAPIAPIService.GetAssetAdministrationShellDescriptorById(ctx, aasIdentifier)
}

// PostAssetAdministrationShellDescriptor executes default POST behavior.
func (s *CustomRegistryService) PostAssetAdministrationShellDescriptor(
	ctx context.Context,
	assetAdministrationShellDescriptor model.AssetAdministrationShellDescriptor,
) (model.ImplResponse, error) {
	ctx = withDTRDescriptorWriteContext(ctx)

	baseResp, baseErr := s.AssetAdministrationShellRegistryAPIAPIService.PostAssetAdministrationShellDescriptor(
		ctx,
		assetAdministrationShellDescriptor,
	)
	if baseErr != nil || !is2xx(baseResp.Code) {
		return baseResp, baseErr
	}

	return baseResp, nil
}

// PutAssetAdministrationShellDescriptorById executes default PUT behavior.
func (s *CustomRegistryService) PutAssetAdministrationShellDescriptorById(
	ctx context.Context,
	aasIdentifier string,
	assetAdministrationShellDescriptor model.AssetAdministrationShellDescriptor,
) (model.ImplResponse, error) {
	ctx = withDTRDescriptorWriteContext(ctx)

	decodedAASID, decodeErr := common.DecodeString(aasIdentifier)
	if decodeErr != nil {
		resp := common.NewErrorResponse(
			decodeErr,
			http.StatusBadRequest,
			customRegistryComponentName,
			"PutAssetAdministrationShellDescriptorById",
			"BadRequest-DecodeAAS",
		)
		return resp, nil
	}
	// DTR customization: path id wins, so base strict-check remains bypassed only here.
	assetAdministrationShellDescriptor.Id = decodedAASID

	baseResp, baseErr := s.AssetAdministrationShellRegistryAPIAPIService.PutAssetAdministrationShellDescriptorById(
		ctx,
		aasIdentifier,
		assetAdministrationShellDescriptor,
	)
	if baseErr != nil || !is2xx(baseResp.Code) {
		return baseResp, baseErr
	}

	return baseResp, nil
}

// PutSubmodelDescriptorByIdThroughSuperpath executes default PUT behavior for
// submodel descriptors while deactivating strict body-id/path-id mismatch only
// for Digital Twin Registry.
//
// Payload compatibility note:
//   - Default field is plural "supplementalSemanticIds".
//   - Singular "supplementalSemanticId" support is controlled via config key
//     general.supportsSingularSupplementalSemanticId
//     (env: GENERAL_SUPPORTSSINGULARSUPPLEMENTALSEMANTICID).
func (s *CustomRegistryService) PutSubmodelDescriptorByIdThroughSuperpath(
	ctx context.Context,
	aasIdentifier string,
	submodelIdentifier string,
	submodelDescriptor model.SubmodelDescriptor,
) (model.ImplResponse, error) {
	decodedSMD, decodeErr := common.DecodeString(submodelIdentifier)
	if decodeErr != nil {
		resp := common.NewErrorResponse(
			decodeErr,
			http.StatusBadRequest,
			customRegistryComponentName,
			"PutSubmodelDescriptorByIdThroughSuperpath",
			"BadRequest-DecodeSubmodel",
		)
		return resp, nil
	}
	// DTR customization: path id wins, so base strict-check remains bypassed only here.
	submodelDescriptor.Id = decodedSMD

	return s.AssetAdministrationShellRegistryAPIAPIService.PutSubmodelDescriptorByIdThroughSuperpath(
		ctx,
		aasIdentifier,
		submodelIdentifier,
		submodelDescriptor,
	)
}

// ExecuteBulkCreateAtomic executes atomic bulk create with DTR-specific context flags.
func (s *CustomRegistryService) ExecuteBulkCreateAtomic(
	ctx context.Context,
	descriptors []model.AssetAdministrationShellDescriptor,
) asyncbulk.OperationResult {
	ctx = withDTRDescriptorWriteContext(ctx)
	return s.AssetAdministrationShellRegistryAPIAPIService.ExecuteBulkCreateAtomic(ctx, descriptors)
}

// ExecuteBulkPutAtomic executes atomic bulk put with DTR-specific context flags.
func (s *CustomRegistryService) ExecuteBulkPutAtomic(
	ctx context.Context,
	descriptors []model.AssetAdministrationShellDescriptor,
) asyncbulk.OperationResult {
	ctx = withDTRDescriptorWriteContext(ctx)
	return s.AssetAdministrationShellRegistryAPIAPIService.ExecuteBulkPutAtomic(ctx, descriptors)
}

func withDTRDescriptorWriteContext(ctx context.Context) context.Context {
	ctx = descriptorsutil.WithAllowAASDescriptorCreatedAtOverride(ctx)
	ctx = descriptorsutil.WithIncludeAASDescriptorCreatedAt(ctx)
	return descriptorsutil.WithPublicReadableGlobalAssetIDExternalSubjectID(ctx)
}

func is2xx(code int) bool {
	return code >= http.StatusOK && code < http.StatusMultipleChoices
}
