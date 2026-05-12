package aasenvironment

import (
	"fmt"
	"net/url"
	"strings"

	"github.com/aas-core-works/aas-core3.1-golang/types"
	"github.com/eclipse-basyx/basyx-go-components/internal/common"
	commonmodel "github.com/eclipse-basyx/basyx-go-components/internal/common/model"
)

const externalURLKey = "general.externalUrl"

// RegistrySyncConfig controls repository-to-registry synchronization behavior and endpoint generation.
type RegistrySyncConfig struct {
	AASRegistryIntegration      bool
	SubmodelRegistryIntegration bool
	ExternalBaseURLs            []string
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

	if (aasRegistryIntegration || submodelRegistryIntegration) && len(config.ExternalBaseURLs) == 0 {
		return RegistrySyncConfig{}, common.NewErrBadRequest(
			"AASENV-REGSYNCCFG-MISSINGEXTERNALURL " + externalURLKey + " must be set when registry synchronization is enabled",
		)
	}

	return config, nil
}

func (c RegistrySyncConfig) buildAASDescriptor(aas types.IAssetAdministrationShell) (commonmodel.AssetAdministrationShellDescriptor, error) {
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
		Endpoints:           c.buildAASDescriptorEndpoints(aas.ID()),
		GlobalAssetId:       readOptionalString(assetInformation.GlobalAssetID()),
		IdShort:             readOptionalString(aas.IDShort()),
		Id:                  aas.ID(),
		SpecificAssetIds:    assetInformation.SpecificAssetIDs(),
		SubmodelDescriptors: c.buildEmbeddedSubmodelDescriptors(aas.Submodels()),
	}

	return descriptor, nil
}

func (c RegistrySyncConfig) buildSubmodelDescriptor(submodel types.ISubmodel) (commonmodel.SubmodelDescriptor, error) {
	if submodel == nil {
		return commonmodel.SubmodelDescriptor{}, common.NewErrBadRequest(
			"AASENV-SYNCSM-NILSUBMODEL submodel must not be nil",
		)
	}

	descriptor := commonmodel.SubmodelDescriptor{
		Administration:         submodel.Administration(),
		Description:            submodel.Description(),
		DisplayName:            submodel.DisplayName(),
		Endpoints:              c.buildSubmodelDescriptorEndpoints(submodel.ID()),
		Extensions:             toExtensionValues(submodel.Extensions()),
		Id:                     submodel.ID(),
		IdShort:                readOptionalString(submodel.IDShort()),
		SemanticId:             submodel.SemanticID(),
		SupplementalSemanticId: submodel.SupplementalSemanticIDs(),
	}

	return descriptor, nil
}

func (c RegistrySyncConfig) buildAASDescriptorEndpoints(aasID string) []commonmodel.Endpoint {
	encodedID := common.EncodeString(aasID)
	return c.buildEndpoints("/shells/" + encodedID)
}

func (c RegistrySyncConfig) buildSubmodelDescriptorEndpoints(submodelID string) []commonmodel.Endpoint {
	encodedID := common.EncodeString(submodelID)
	return c.buildEndpoints("/submodels/" + encodedID)
}

func (c RegistrySyncConfig) buildEmbeddedSubmodelDescriptors(references []types.IReference) []commonmodel.SubmodelDescriptor {
	if len(references) == 0 {
		return []commonmodel.SubmodelDescriptor{}
	}

	seen := make(map[string]struct{}, len(references))
	result := make([]commonmodel.SubmodelDescriptor, 0, len(references))
	for _, reference := range references {
		for _, key := range reference.Keys() {
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
				Endpoints: c.buildSubmodelDescriptorEndpoints(submodelID),
			})
		}
	}

	return result
}

func (c RegistrySyncConfig) buildEndpoints(resourcePath string) []commonmodel.Endpoint {
	endpoints := make([]commonmodel.Endpoint, 0, len(c.ExternalBaseURLs))
	for _, externalBaseURL := range c.ExternalBaseURLs {
		endpointURL := strings.TrimRight(externalBaseURL, "/") + resourcePath
		endpoints = append(endpoints, commonmodel.Endpoint{
			Interface: "AAS-3.0",
			ProtocolInformation: commonmodel.ProtocolInformation{
				Href:             endpointURL,
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
