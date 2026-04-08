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
	"strings"

	"github.com/aas-core-works/aas-core3.1-golang/types"
	registryapiinternal "github.com/eclipse-basyx/basyx-go-components/internal/aasregistry/api"
	"github.com/eclipse-basyx/basyx-go-components/internal/common"
	"github.com/eclipse-basyx/basyx-go-components/internal/common/descriptors"
	"github.com/eclipse-basyx/basyx-go-components/internal/common/model"
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

// PostAssetAdministrationShellDescriptor executes default POST behavior and
// appends a discovery asset link from globalAssetId when present.
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

	if errResp, err := s.appendGlobalAssetLink(
		ctx,
		assetAdministrationShellDescriptor.Id,
		assetAdministrationShellDescriptor.GlobalAssetId,
		"PostAssetAdministrationShellDescriptor",
	); errResp != nil || err != nil {
		return mapAppendGlobalAssetLinkResult(errResp, err, "PostAssetAdministrationShellDescriptor")
	}

	return baseResp, nil
}

// PutAssetAdministrationShellDescriptorById executes default PUT behavior and
// appends a discovery asset link from globalAssetId when present.
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

	if errResp, err := s.appendGlobalAssetLink(
		ctx,
		decodedAASID,
		assetAdministrationShellDescriptor.GlobalAssetId,
		"PutAssetAdministrationShellDescriptorById",
	); errResp != nil || err != nil {
		return mapAppendGlobalAssetLinkResult(errResp, err, "PutAssetAdministrationShellDescriptorById")
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

func (s *CustomRegistryService) appendGlobalAssetLink(
	ctx context.Context,
	aasID string,
	globalAssetID string,
	method string,
) (*model.ImplResponse, error) {
	if s.discovery == nil {
		return nil, nil
	}
	if strings.TrimSpace(globalAssetID) == "" || strings.TrimSpace(aasID) == "" {
		return nil, nil
	}

	aasIdentifier := common.EncodeString(aasID)
	links := []types.ISpecificAssetID{
		types.NewSpecificAssetID("globalAssetId", globalAssetID),
	}

	discoveryOnlyCtx := descriptors.WithDiscoveryOnlySpecificAssetIDs(ctx)
	addResp, addErr := s.discovery.AddAllAssetLinksByID(discoveryOnlyCtx, aasIdentifier, links)
	if addErr != nil {
		resp := common.NewErrorResponse(
			addErr,
			http.StatusInternalServerError,
			customRegistryComponentName,
			method,
			"AddGlobalAssetLink",
		)
		return &resp, addErr
	}
	if !is2xx(addResp.Code) {
		resp := common.NewErrorResponse(
			common.NewInternalServerError("failed to add globalAssetId discovery link"),
			http.StatusInternalServerError,
			customRegistryComponentName,
			method,
			"AddGlobalAssetLink-Non2xx",
		)
		return &resp, nil
	}
	return nil, nil
}

func is2xx(code int) bool {
	return code >= http.StatusOK && code < http.StatusMultipleChoices
}

func mapAppendGlobalAssetLinkResult(
	errResp *model.ImplResponse,
	err error,
	method string,
) (model.ImplResponse, error) {
	if errResp != nil {
		return *errResp, err
	}

	resp := common.NewErrorResponse(
		common.NewInternalServerError("failed to append globalAssetId discovery link"),
		http.StatusInternalServerError,
		customRegistryComponentName,
		method,
		"AddGlobalAssetLink-NilResponse",
	)
	return resp, err
}
