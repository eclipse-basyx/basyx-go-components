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
	"net/http"

	registryapiinternal "github.com/eclipse-basyx/basyx-go-components/internal/aasregistry/api"
	"github.com/eclipse-basyx/basyx-go-components/internal/common"
	"github.com/eclipse-basyx/basyx-go-components/internal/common/descriptors"
	"github.com/eclipse-basyx/basyx-go-components/internal/common/model"
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
) (model.ImplResponse, error) {
	createdAfter, _ := CreatedAfterFromContext(ctx)
	if createdAfter != nil {
		query := buildEdcBpnClaimEqualsHeaderExpression(createdAfter, "$aasdesc#createdAt")
		ctx = auth.MergeQueryFilter(ctx, query)
	}
	ctx = descriptors.WithIncludeAASDescriptorCreatedAt(ctx)

	return s.AssetAdministrationShellRegistryAPIAPIService.GetAllAssetAdministrationShellDescriptors(
		ctx,
		limit,
		cursor,
		assetKind,
		assetType,
	)
}

// GetAssetAdministrationShellDescriptorById - Returns a specific Asset Administration Shell Descriptor
// nolint:revive // defined by standard
func (s *CustomRegistryService) GetAssetAdministrationShellDescriptorById(
	ctx context.Context,
	aasIdentifier string,
) (model.ImplResponse, error) {
	ctx = descriptors.WithIncludeAASDescriptorCreatedAt(ctx)
	return s.AssetAdministrationShellRegistryAPIAPIService.GetAssetAdministrationShellDescriptorById(ctx, aasIdentifier)
}

// PostAssetAdministrationShellDescriptor executes default POST behavior.
func (s *CustomRegistryService) PostAssetAdministrationShellDescriptor(
	ctx context.Context,
	assetAdministrationShellDescriptor model.AssetAdministrationShellDescriptor,
) (model.ImplResponse, error) {
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

func is2xx(code int) bool {
	return code >= http.StatusOK && code < http.StatusMultipleChoices
}
