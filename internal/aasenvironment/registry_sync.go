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

package aasenvironment

import (
	"context"
	"fmt"
	"net/url"
	"strings"

	"github.com/FriedJannik/aas-go-sdk/types"
	"github.com/eclipse-basyx/basyx-go-components/internal/common"
	commonmodel "github.com/eclipse-basyx/basyx-go-components/internal/common/model"
)

const (
	externalURLKey              = "general.externalUrl"
	aasDescriptorInterface      = "AAS-3.0"
	submodelDescriptorInterface = "SUBMODEL-3.0"
)

// RegistrySyncConfig controls repository-to-registry synchronization behavior and endpoint generation.
type RegistrySyncConfig struct {
	AASRegistryIntegration      bool
	SubmodelRegistryIntegration bool
	ExternalBaseURLs            []string
}

// ValidateStandaloneAASRepositoryRegistrySyncConfig validates standalone AAS repository toggle usage.
func ValidateStandaloneAASRepositoryRegistrySyncConfig(cfg *common.Config) error {
	if cfg == nil {
		return common.NewErrBadRequest("AASENV-REGSYNCCFG-NILCFG configuration must not be nil")
	}

	if cfg.General.SubmodelRegistryIntegration {
		return common.NewErrBadRequest("AASENV-REGSYNCCFG-AASREPO-INVALIDTOGGLE unsupported standalone toggle: general.submodelRegistryIntegration=true")
	}

	return nil
}

// ValidateStandaloneSubmodelRepositoryRegistrySyncConfig validates standalone submodel repository toggle usage.
func ValidateStandaloneSubmodelRepositoryRegistrySyncConfig(cfg *common.Config) error {
	if cfg == nil {
		return common.NewErrBadRequest("AASENV-REGSYNCCFG-NILCFG configuration must not be nil")
	}

	if cfg.General.AASRegistryIntegration {
		return common.NewErrBadRequest("AASENV-REGSYNCCFG-SMREPO-INVALIDTOGGLE unsupported standalone toggle: general.aasRegistryIntegration=true")
	}

	return nil
}

// NewRegistrySyncConfig validates sync-related settings and normalizes configured external base URLs.
func NewRegistrySyncConfig(
	aasRegistryIntegration bool,
	submodelRegistryIntegration bool,
	rawExternalURL string,
) (RegistrySyncConfig, error) {
	config := RegistrySyncConfig{
		AASRegistryIntegration:      aasRegistryIntegration,
		SubmodelRegistryIntegration: submodelRegistryIntegration,
	}

	parsedExternalURLs, err := parseExternalBaseURLs(rawExternalURL)
	if err != nil {
		if aasRegistryIntegration || submodelRegistryIntegration {
			return RegistrySyncConfig{}, err
		}
	}
	config.ExternalBaseURLs = parsedExternalURLs

	return config, nil
}

func (c RegistrySyncConfig) buildAASDescriptor(aas types.IAssetAdministrationShell) (commonmodel.AssetAdministrationShellDescriptor, error) {
	return c.buildAASDescriptorForContext(context.Background(), aas)
}

func (c RegistrySyncConfig) buildAASDescriptorForContext(ctx context.Context, aas types.IAssetAdministrationShell) (commonmodel.AssetAdministrationShellDescriptor, error) {
	if aas == nil {
		return commonmodel.AssetAdministrationShellDescriptor{}, common.NewErrBadRequest(
			"AASENV-SYNCAAS-NILAAS asset administration shell must not be nil",
		)
	}

	assetInformation := aas.AssetInformation()
	if assetInformation == nil {
		return commonmodel.AssetAdministrationShellDescriptor{}, common.NewErrBadRequest(
			"AASENV-SYNCAAS-NILASSETINFO asset information must not be nil",
		)
	}

	descriptor := commonmodel.AssetAdministrationShellDescriptor{
		Administration:      aas.Administration(),
		Description:         aas.Description(),
		DisplayName:         aas.DisplayName(),
		Extensions:          toExtensionValues(aas.Extensions()),
		AssetKind:           assetKindPointer(assetInformation.AssetKind()),
		AssetType:           readOptionalString(assetInformation.AssetType()),
		Endpoints:           c.buildAASDescriptorEndpointsForContext(ctx, aas.ID()),
		GlobalAssetId:       readOptionalString(assetInformation.GlobalAssetID()),
		IdShort:             readOptionalString(aas.IDShort()),
		Id:                  aas.ID(),
		SpecificAssetIds:    assetInformation.SpecificAssetIDs(),
		SubmodelDescriptors: c.buildEmbeddedSubmodelDescriptorsForContext(ctx, aas.Submodels()),
	}

	return descriptor, nil
}

func (c RegistrySyncConfig) buildSubmodelDescriptor(submodel types.ISubmodel) (commonmodel.SubmodelDescriptor, error) {
	return c.buildSubmodelDescriptorForContext(context.Background(), submodel)
}

func (c RegistrySyncConfig) buildSubmodelDescriptorForContext(ctx context.Context, submodel types.ISubmodel) (commonmodel.SubmodelDescriptor, error) {
	if submodel == nil {
		return commonmodel.SubmodelDescriptor{}, common.NewErrBadRequest(
			"AASENV-SYNCSM-NILSUBMODEL submodel must not be nil",
		)
	}

	descriptor := commonmodel.SubmodelDescriptor{
		Administration:         submodel.Administration(),
		Description:            submodel.Description(),
		DisplayName:            submodel.DisplayName(),
		Endpoints:              c.buildSubmodelDescriptorEndpointsForContext(ctx, submodel.ID()),
		Extensions:             toExtensionValues(submodel.Extensions()),
		Id:                     submodel.ID(),
		IdShort:                readOptionalString(submodel.IDShort()),
		SemanticId:             submodel.SemanticID(),
		SupplementalSemanticId: submodel.SupplementalSemanticIDs(),
	}

	return descriptor, nil
}

func (c RegistrySyncConfig) buildAASDescriptorEndpoints(aasID string) []commonmodel.Endpoint {
	return c.buildAASDescriptorEndpointsForContext(context.Background(), aasID)
}

func (c RegistrySyncConfig) buildAASDescriptorEndpointsForContext(ctx context.Context, aasID string) []commonmodel.Endpoint {
	encodedID := common.EncodeString(aasID)
	return c.buildEndpointsForContext(ctx, "/shells/"+encodedID)
}

func (c RegistrySyncConfig) buildSubmodelDescriptorEndpoints(submodelID string) []commonmodel.Endpoint {
	return c.buildSubmodelDescriptorEndpointsForContext(context.Background(), submodelID)
}

func (c RegistrySyncConfig) buildSubmodelDescriptorEndpointsForContext(ctx context.Context, submodelID string) []commonmodel.Endpoint {
	encodedID := common.EncodeString(submodelID)
	return c.buildEndpointsForContext(ctx, "/submodels/"+encodedID)
}

func (c RegistrySyncConfig) buildEmbeddedSubmodelDescriptors(references []types.IReference) []commonmodel.SubmodelDescriptor {
	return c.buildEmbeddedSubmodelDescriptorsForContext(context.Background(), references)
}

func (c RegistrySyncConfig) buildEmbeddedSubmodelDescriptorsForContext(ctx context.Context, references []types.IReference) []commonmodel.SubmodelDescriptor {
	if len(references) == 0 {
		return []commonmodel.SubmodelDescriptor{}
	}

	seen := make(map[string]struct{}, len(references))
	result := make([]commonmodel.SubmodelDescriptor, 0, len(references))
	for _, reference := range references {
		if reference == nil {
			continue
		}

		for _, key := range reference.Keys() {
			if key == nil {
				continue
			}

			if key.Type() != types.KeyTypesSubmodel {
				continue
			}

			submodelID := strings.TrimSpace(key.Value())
			if submodelID == "" {
				continue
			}
			if _, alreadyAdded := seen[submodelID]; alreadyAdded {
				continue
			}

			seen[submodelID] = struct{}{}
			result = append(result, commonmodel.SubmodelDescriptor{
				Id:        submodelID,
				Endpoints: c.buildSubmodelDescriptorEndpointsForContext(ctx, submodelID),
			})
		}
	}

	return result
}

func (c RegistrySyncConfig) buildEndpointsForContext(ctx context.Context, resourcePath string) []commonmodel.Endpoint {
	externalBaseURLs := c.externalBaseURLsForContext(ctx)
	endpoints := make([]commonmodel.Endpoint, 0, len(externalBaseURLs))
	for _, externalBaseURL := range externalBaseURLs {
		endpointURL := strings.TrimRight(externalBaseURL, "/") + resourcePath
		endpoints = append(endpoints, commonmodel.Endpoint{
			Interface: endpointInterface,
			ProtocolInformation: commonmodel.ProtocolInformation{
				Href:             endpointURL,
				EndpointProtocol: protocolFromURL(externalBaseURL),
			},
		})
	}
	return endpoints
}

func (c RegistrySyncConfig) externalBaseURLsForContext(ctx context.Context) []string {
	if len(c.ExternalBaseURLs) > 0 {
		return c.ExternalBaseURLs
	}

	requestExternalBaseURL := common.RequestExternalBaseURLFromContext(ctx)
	if requestExternalBaseURL == "" {
		return []string{}
	}

	return []string{requestExternalBaseURL}
}

func (c RegistrySyncConfig) hasEndpointBaseURL(ctx context.Context) bool {
	return len(c.externalBaseURLsForContext(ctx)) > 0
}

func parseExternalBaseURLs(rawExternalURL string) ([]string, error) {
	trimmed := strings.TrimSpace(rawExternalURL)
	if trimmed == "" {
		return []string{}, nil
	}

	rawEntries := strings.Split(trimmed, ",")
	normalized := make([]string, 0, len(rawEntries))
	seen := map[string]struct{}{}
	for entryIndex, rawEntry := range rawEntries {
		entry := strings.TrimSpace(rawEntry)
		if entry == "" {
			return nil, common.NewErrBadRequest(
				fmt.Sprintf("AASENV-REGSYNCCFG-EMPTYURL %s contains an empty URL at position %d", externalURLKey, entryIndex),
			)
		}

		normalizedURL, err := normalizeExternalBaseURL(entry)
		if err != nil {
			return nil, err
		}

		if _, alreadyPresent := seen[normalizedURL]; alreadyPresent {
			continue
		}
		seen[normalizedURL] = struct{}{}
		normalized = append(normalized, normalizedURL)
	}

	return normalized, nil
}

func normalizeExternalBaseURL(rawURL string) (string, error) {
	parsed, err := url.Parse(rawURL)
	if err != nil {
		return "", common.NewErrBadRequest(
			"AASENV-REGSYNCCFG-BADEXTERNALURL invalid URL in " + externalURLKey + ": " + err.Error(),
		)
	}

	if parsed.Scheme == "" || parsed.Host == "" {
		return "", common.NewErrBadRequest(
			"AASENV-REGSYNCCFG-BADEXTERNALURL " + externalURLKey + " entries must include scheme and host",
		)
	}
	scheme := strings.ToLower(strings.TrimSpace(parsed.Scheme))
	if scheme != "http" && scheme != "https" {
		return "", common.NewErrBadRequest(
			"AASENV-REGSYNCCFG-BADEXTERNALURL " + externalURLKey + " entries must use http or https scheme",
		)
	}
	if parsed.RawQuery != "" || parsed.Fragment != "" {
		return "", common.NewErrBadRequest(
			"AASENV-REGSYNCCFG-BADEXTERNALURL " + externalURLKey + " entries must not contain query parameters or fragments",
		)
	}

	parsed.Path = strings.TrimRight(parsed.Path, "/")
	parsed.RawPath = strings.TrimRight(parsed.RawPath, "/")

	return strings.TrimRight(parsed.String(), "/"), nil
}

func toExtensionValues(extensions []types.IExtension) []types.Extension {
	if len(extensions) == 0 {
		return nil
	}

	result := make([]types.Extension, 0, len(extensions))
	for _, extension := range extensions {
		typedExtension, ok := extension.(*types.Extension)
		if !ok || typedExtension == nil {
			continue
		}
		result = append(result, *typedExtension)
	}

	return result
}

func assetKindPointer(assetKind types.AssetKind) *types.AssetKind {
	copyValue := assetKind
	return &copyValue
}

func readOptionalString(value *string) string {
	if value == nil {
		return ""
	}
	return strings.TrimSpace(*value)
}

func protocolFromURL(baseURL string) string {
	parsed, err := url.Parse(baseURL)
	if err != nil {
		return ""
	}
	return strings.ToLower(strings.TrimSpace(parsed.Scheme))
}
