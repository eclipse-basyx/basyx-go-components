/*
 * DotAAS Part 2 | HTTP/REST | Submodel Repository Service Specification
 *
 * The entire Submodel Repository Service Specification as part of the [Specification of the Asset Administration Shell: Part 2](http://industrialdigitaltwin.org/en/content-hub).   Publisher: Industrial Digital Twin Association (IDTA) 2023
 *
 * API version: V3.0.3_SSP-001
 * Contact: info@idtwin.org
 */

package model

// Qualifier  type of Qualifier
type Qualifier struct {
	SemanticID *Reference `json:"semanticId,omitempty"`

	//nolint:all
	SupplementalSemanticIds []Reference `json:"supplementalSemanticIds,omitempty"`

	Kind QualifierKind `json:"kind,omitempty"`

	Type string `json:"type" validate:"regexp=^([\\\\x09\\\\x0a\\\\x0d\\\\x20-\\\\ud7ff\\\\ue000-\\\\ufffd]|\\\\ud800[\\\\udc00-\\\\udfff]|[\\\\ud801-\\\\udbfe][\\\\udc00-\\\\udfff]|\\\\udbff[\\\\udc00-\\\\udfff])*$"`

	ValueType DataTypeDefXsd `json:"valueType"`

	Value string `json:"value,omitempty"`

	ValueID *Reference `json:"valueId,omitempty"`
}

// AssertQualifierRequired checks if the required fields are not zero-ed
func AssertQualifierRequired(obj Qualifier) error {
	elements := map[string]interface{}{
		"type":      obj.Type,
		"valueType": obj.ValueType,
	}
	for name, el := range elements {
		if isZero := IsZeroValue(el); isZero {
			return &RequiredError{Field: name}
		}
	}

	if obj.SemanticID != nil {
		if err := AssertReferenceRequired(*obj.SemanticID); err != nil {
			return err
		}
	}
	for _, el := range obj.SupplementalSemanticIds {
		if err := AssertReferenceRequired(el); err != nil {
			return err
		}
	}
	if obj.ValueID != nil {
		if err := AssertReferenceRequired(*obj.ValueID); err != nil {
			return err
		}
	}
	return nil
}

// AssertQualifierConstraints checks if the values respects the defined constraints
func AssertQualifierConstraints(obj Qualifier) error {
	if obj.SemanticID != nil {
		if err := AssertReferenceConstraints(*obj.SemanticID); err != nil {
			return err
		}
	}
	for _, el := range obj.SupplementalSemanticIds {
		if err := AssertReferenceConstraints(el); err != nil {
			return err
		}
	}
	if obj.ValueID != nil {
		if err := AssertReferenceConstraints(*obj.ValueID); err != nil {
			return err
		}
	}
	return nil
}
