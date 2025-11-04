/*
 * DotAAS Part 2 | HTTP/REST | Submodel Repository Service Specification
 *
 * The entire Submodel Repository Service Specification as part of the [Specification of the Asset Administration Shell: Part 2](http://industrialdigitaltwin.org/en/content-hub).   Publisher: Industrial Digital Twin Association (IDTA) 2023
 *
 * API version: V3.0.3_SSP-001
 * Contact: info@idtwin.org
 */

package model

// SpecificAssetID type of SpecificAssetID
type SpecificAssetID struct {
	SemanticID *Reference `json:"semanticId,omitempty"`

	//nolint:all
	SupplementalSemanticIds []Reference `json:"supplementalSemanticIds,omitempty"`

	Name string `json:"name" validate:"regexp=^([\\\\x09\\\\x0a\\\\x0d\\\\x20-\\\\ud7ff\\\\ue000-\\\\ufffd]|\\\\ud800[\\\\udc00-\\\\udfff]|[\\\\ud801-\\\\udbfe][\\\\udc00-\\\\udfff]|\\\\udbff[\\\\udc00-\\\\udfff])*$"`

	Value string `json:"value" validate:"regexp=^([\\\\x09\\\\x0a\\\\x0d\\\\x20-\\\\ud7ff\\\\ue000-\\\\ufffd]|\\\\ud800[\\\\udc00-\\\\udfff]|[\\\\ud801-\\\\udbfe][\\\\udc00-\\\\udfff]|\\\\udbff[\\\\udc00-\\\\udfff])*$"`

	ExternalSubjectID *Reference `json:"externalSubjectId,omitempty"`
}

// AssertSpecificAssetIdRequired checks if the required fields are not zero-ed
//
//nolint:all
func AssertSpecificAssetIdRequired(obj SpecificAssetID) error {
	elements := map[string]interface{}{
		"name":  obj.Name,
		"value": obj.Value,
	}
	for name, el := range elements {
		if isZero := IsZeroValue(el); isZero {
			return &RequiredError{Field: name}
		}
	}

	if obj.ExternalSubjectID != nil {
		if err := AssertReferenceRequired(*obj.ExternalSubjectID); err != nil {
			return err
		}
	}

	if obj.SemanticID != nil {
		if err := AssertReferenceRequired(*obj.SemanticID); err != nil {
			return err
		}
	}

	for _, ref := range obj.SupplementalSemanticIds {
		if err := AssertReferenceRequired(ref); err != nil {
			return err
		}
	}

	return nil
}

// AssertSpecificAssetIdConstraints checks if the values respects the defined constraints
//
//nolint:all
func AssertSpecificAssetIdConstraints(obj SpecificAssetID) error {

	if obj.ExternalSubjectID != nil {
		if err := AssertReferenceConstraints(*obj.ExternalSubjectID); err != nil {
			return err
		}
	}

	if obj.SemanticID != nil {
		if err := AssertReferenceConstraints(*obj.SemanticID); err != nil {
			return err
		}
	}

	for _, ref := range obj.SupplementalSemanticIds {
		if err := AssertReferenceConstraints(ref); err != nil {
			return err
		}
	}
	return nil
}
