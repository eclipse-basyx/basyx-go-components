/*
 * DotAAS Part 2 | HTTP/REST | Submodel Repository Service Specification
 *
 * The entire Submodel Repository Service Specification as part of the [Specification of the Asset Administration Shell: Part 2](http://industrialdigitaltwin.org/en/content-hub).   Publisher: Industrial Digital Twin Association (IDTA) 2023
 *
 * API version: V3.0.3_SSP-001
 * Contact: info@idtwin.org
 */

package model

// AssetInformation type of AssetAdministrationShell
type AssetInformation struct {
	AssetKind AssetKind `json:"assetKind"`

	GlobalAssetID string `json:"globalAssetId,omitempty" validate:"regexp=^([\\\\x09\\\\x0a\\\\x0d\\\\x20-\\\\ud7ff\\\\ue000-\\\\ufffd]|\\\\ud800[\\\\udc00-\\\\udfff]|[\\\\ud801-\\\\udbfe][\\\\udc00-\\\\udfff]|\\\\udbff[\\\\udc00-\\\\udfff])*$"`

	//nolint:all
	SpecificAssetIds []SpecificAssetID `json:"specificAssetIds,omitempty"`

	AssetType string `json:"assetType,omitempty" validate:"regexp=^([\\\\x09\\\\x0a\\\\x0d\\\\x20-\\\\ud7ff\\\\ue000-\\\\ufffd]|\\\\ud800[\\\\udc00-\\\\udfff]|[\\\\ud801-\\\\udbfe][\\\\udc00-\\\\udfff]|\\\\udbff[\\\\udc00-\\\\udfff])*$"`

	DefaultThumbnail Resource `json:"defaultThumbnail,omitempty"`
}

// AssertAssetInformationRequired checks if the required fields are not zero-ed
func AssertAssetInformationRequired(obj AssetInformation) error {
	elements := map[string]interface{}{
		"assetKind": obj.AssetKind,
	}
	for name, el := range elements {
		if isZero := IsZeroValue(el); isZero {
			return &RequiredError{Field: name}
		}
	}

	for _, el := range obj.SpecificAssetIds {
		if err := AssertSpecificAssetIdRequired(el); err != nil {
			return err
		}
	}
	if err := AssertResourceRequired(obj.DefaultThumbnail); err != nil {
		return err
	}
	return nil
}

// AssertAssetInformationConstraints checks if the values respects the defined constraints
func AssertAssetInformationConstraints(obj AssetInformation) error {
	for _, el := range obj.SpecificAssetIds {
		if err := AssertSpecificAssetIdConstraints(el); err != nil {
			return err
		}
	}
	if err := AssertResourceConstraints(obj.DefaultThumbnail); err != nil {
		return err
	}
	return nil
}
