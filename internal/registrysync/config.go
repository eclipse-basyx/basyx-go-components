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

package registrysync

import (
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

// Config controls repository-to-registry synchronization and descriptor endpoint generation.
type Config struct {
	AASRegistryIntegration      bool
	SubmodelRegistryIntegration bool
	ExternalBaseURLs            []string
}

// NewConfig validates registry synchronization flags and normalizes the configured external base URLs.
func NewConfig(aasRegistryIntegration bool, submodelRegistryIntegration bool, rawExternalURL string) (Config, error) {
	config := Config{
		AASRegistryIntegration:      aasRegistryIntegration,
		SubmodelRegistryIntegration: submodelRegistryIntegration,
	}
	parsedExternalURLs, err := parseExternalBaseURLs(rawExternalURL)
	if err != nil && (aasRegistryIntegration || submodelRegistryIntegration) {
		return Config{}, err
	}
	config.ExternalBaseURLs = parsedExternalURLs
	if (aasRegistryIntegration || submodelRegistryIntegration) && len(config.ExternalBaseURLs) == 0 {
		return Config{}, common.NewErrBadRequest(
			"AASENV-REGSYNCCFG-MISSINGEXTERNALURL " + externalURLKey + " must be set when registry synchronization is enabled",
		)
	}
	return config, nil
}

// BuildAASDescriptor builds an AAS registry descriptor from an AAS repository resource.
func (c Config) BuildAASDescriptor(aas types.IAssetAdministrationShell) (commonmodel.AssetAdministrationShellDescriptor, error) {
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
	return commonmodel.AssetAdministrationShellDescriptor{
		Administration:      aas.Administration(),
		Description:         aas.Description(),
		DisplayName:         aas.DisplayName(),
		Extensions:          toExtensionValues(aas.Extensions()),
		AssetKind:           assetKindPointer(assetInformation.AssetKind()),
		AssetType:           readOptionalString(assetInformation.AssetType()),
		Endpoints:           c.AASDescriptorEndpoints(aas.ID()),
		GlobalAssetId:       readOptionalString(assetInformation.GlobalAssetID()),
		IdShort:             readOptionalString(aas.IDShort()),
		Id:                  aas.ID(),
		SpecificAssetIds:    assetInformation.SpecificAssetIDs(),
		SubmodelDescriptors: c.buildEmbeddedSubmodelDescriptors(aas.Submodels()),
	}, nil
}

// BuildSubmodelDescriptor builds a Submodel registry descriptor from a Submodel repository resource.
func (c Config) BuildSubmodelDescriptor(submodel types.ISubmodel) (commonmodel.SubmodelDescriptor, error) {
	if submodel == nil {
		return commonmodel.SubmodelDescriptor{}, common.NewErrBadRequest(
			"AASENV-SYNCSM-NILSUBMODEL submodel must not be nil",
		)
	}
	return commonmodel.SubmodelDescriptor{
		Administration:         submodel.Administration(),
		Description:            submodel.Description(),
		DisplayName:            submodel.DisplayName(),
		Endpoints:              c.SubmodelDescriptorEndpoints(submodel.ID()),
		Extensions:             toExtensionValues(submodel.Extensions()),
		Id:                     submodel.ID(),
		IdShort:                readOptionalString(submodel.IDShort()),
		SemanticId:             submodel.SemanticID(),
		SupplementalSemanticId: submodel.SupplementalSemanticIDs(),
	}, nil
}

// DescriptorsEqual reports whether two descriptors have the same canonical JSON representation.
func DescriptorsEqual(previous any, submitted any) (bool, error) {
	previousHash, err := common.CanonicalJSONHash(previous)
	if err != nil {
		return false, common.NewInternalServerError("AASENV-REGSYNC-HASHPREVIOUS " + err.Error())
	}
	submittedHash, err := common.CanonicalJSONHash(submitted)
	if err != nil {
		return false, common.NewInternalServerError("AASENV-REGSYNC-HASHSUBMITTED " + err.Error())
	}
	return previousHash == submittedHash, nil
}

// ChangedSubmodelDescriptor builds the submitted descriptor and reports whether descriptor-relevant data changed.
func (c Config) ChangedSubmodelDescriptor(previous types.ISubmodel, submitted types.ISubmodel) (commonmodel.SubmodelDescriptor, bool, error) {
	descriptor, err := c.BuildSubmodelDescriptor(submitted)
	if err != nil || previous == nil {
		return descriptor, previous == nil, err
	}
	previousDescriptor, err := c.BuildSubmodelDescriptor(previous)
	if err != nil {
		return commonmodel.SubmodelDescriptor{}, false, err
	}
	equal, err := DescriptorsEqual(previousDescriptor, descriptor)
	return descriptor, !equal, err
}

// ChangedAASDescriptor builds the submitted descriptor and reports whether descriptor-relevant data changed.
func (c Config) ChangedAASDescriptor(previous types.IAssetAdministrationShell, submitted types.IAssetAdministrationShell) (commonmodel.AssetAdministrationShellDescriptor, bool, error) {
	descriptor, err := c.BuildAASDescriptor(submitted)
	if err != nil || previous == nil {
		return descriptor, previous == nil, err
	}
	previousDescriptor, err := c.BuildAASDescriptor(previous)
	if err != nil {
		return commonmodel.AssetAdministrationShellDescriptor{}, false, err
	}
	equal, err := DescriptorsEqual(previousDescriptor, descriptor)
	return descriptor, !equal, err
}

// AASDescriptorEndpoints builds repository endpoints for an AAS identifier.
func (c Config) AASDescriptorEndpoints(aasID string) []commonmodel.Endpoint {
	return c.buildEndpoints("/shells/"+common.EncodeString(aasID), aasDescriptorInterface)
}

// SubmodelDescriptorEndpoints builds repository endpoints for a Submodel identifier.
func (c Config) SubmodelDescriptorEndpoints(submodelID string) []commonmodel.Endpoint {
	return c.buildEndpoints("/submodels/"+common.EncodeString(submodelID), submodelDescriptorInterface)
}

func (c Config) buildEmbeddedSubmodelDescriptors(references []types.IReference) []commonmodel.SubmodelDescriptor {
	if len(references) == 0 {
		return []commonmodel.SubmodelDescriptor{}
	}
	seen := make(map[string]struct{}, len(references))
	result := make([]commonmodel.SubmodelDescriptor, 0, len(references))
	for _, reference := range references {
		submodelIDs := embeddedSubmodelIDs(reference)
		for _, submodelID := range submodelIDs {
			if _, exists := seen[submodelID]; exists {
				continue
			}
			seen[submodelID] = struct{}{}
			result = append(result, commonmodel.SubmodelDescriptor{
				Id:        submodelID,
				Endpoints: c.SubmodelDescriptorEndpoints(submodelID),
			})
		}
	}
	return result
}

func embeddedSubmodelIDs(reference types.IReference) []string {
	if reference == nil {
		return nil
	}
	result := make([]string, 0, len(reference.Keys()))
	for _, key := range reference.Keys() {
		if key == nil || key.Type() != types.KeyTypesSubmodel {
			continue
		}
		submodelID := strings.TrimSpace(key.Value())
		if submodelID != "" {
			result = append(result, submodelID)
		}
	}
	return result
}

func (c Config) buildEndpoints(resourcePath string, endpointInterface string) []commonmodel.Endpoint {
	endpoints := make([]commonmodel.Endpoint, 0, len(c.ExternalBaseURLs))
	for _, externalBaseURL := range c.ExternalBaseURLs {
		endpoints = append(endpoints, commonmodel.Endpoint{
			Interface: endpointInterface,
			ProtocolInformation: commonmodel.ProtocolInformation{
				Href:             strings.TrimRight(externalBaseURL, "/") + resourcePath,
				EndpointProtocol: protocolFromURL(externalBaseURL),
			},
		})
	}
	return endpoints
}

func parseExternalBaseURLs(rawExternalURL string) ([]string, error) {
	trimmed := strings.TrimSpace(rawExternalURL)
	if trimmed == "" {
		return []string{}, nil
	}
	rawEntries := strings.Split(trimmed, ",")
	normalized := make([]string, 0, len(rawEntries))
	seen := make(map[string]struct{}, len(rawEntries))
	for entryIndex, rawEntry := range rawEntries {
		entry := strings.TrimSpace(rawEntry)
		if entry == "" {
			return nil, common.NewErrBadRequest(fmt.Sprintf(
				"AASENV-REGSYNCCFG-EMPTYURL %s contains an empty URL at position %d", externalURLKey, entryIndex,
			))
		}
		normalizedURL, err := normalizeExternalBaseURL(entry)
		if err != nil {
			return nil, err
		}
		if _, exists := seen[normalizedURL]; exists {
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
		return "", common.NewErrBadRequest("AASENV-REGSYNCCFG-BADEXTERNALURL invalid URL in " + externalURLKey + ": " + err.Error())
	}
	if parsed.Scheme == "" || parsed.Host == "" {
		return "", common.NewErrBadRequest("AASENV-REGSYNCCFG-BADEXTERNALURL " + externalURLKey + " entries must include scheme and host")
	}
	scheme := strings.ToLower(strings.TrimSpace(parsed.Scheme))
	if scheme != "http" && scheme != "https" {
		return "", common.NewErrBadRequest("AASENV-REGSYNCCFG-BADEXTERNALURL " + externalURLKey + " entries must use http or https scheme")
	}
	if parsed.RawQuery != "" || parsed.Fragment != "" {
		return "", common.NewErrBadRequest("AASENV-REGSYNCCFG-BADEXTERNALURL " + externalURLKey + " entries must not contain query parameters or fragments")
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
		if ok && typedExtension != nil {
			result = append(result, *typedExtension)
		}
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
