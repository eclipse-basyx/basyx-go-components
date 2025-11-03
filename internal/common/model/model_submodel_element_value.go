/*
 * DotAAS Part 2 | HTTP/REST | Submodel Repository Service Specification
 *
 * The entire Submodel Repository Service Specification as part of the [Specification of the Asset Administration Shell: Part 2](http://industrialdigitaltwin.org/en/content-hub).   Publisher: Industrial Digital Twin Association (IDTA) 2023
 *
 * API version: V3.0.3_SSP-001
 * Contact: info@idtwin.org
 */

package model

// SubmodelElementValue struct representing a SubmodelElementValue.
type SubmodelElementValue struct {
	Observed ReferenceValue `json:"observed"`

	ContentType string `json:"contentType"`

	Value string `json:"value"`

	Max RangeValueType `json:"max,omitempty"`

	Min RangeValueType `json:"min,omitempty"`

	Type ReferenceTypes `json:"type,omitempty"`

	Keys []Key `json:"keys,omitempty"`

	First ReferenceValue `json:"first"`

	Second ReferenceValue `json:"second"`

	Annotations []map[string]interface{} `json:"annotations,omitempty"`

	EntityType EntityType `json:"entityType"`

	GlobalAssetID string `json:"globalAssetID,omitempty"`

	SpecificAssetIds []map[string]interface{} `json:"specificAssetIds,omitempty"`

	Statements []map[string]interface{} `json:"statements,omitempty"`
}

// AssertSubmodelElementValueRequired checks if the required fields are not zero-ed
func AssertSubmodelElementValueRequired(obj SubmodelElementValue) error {
	elements := map[string]interface{}{
		"observed":    obj.Observed,
		"contentType": obj.ContentType,
		"value":       obj.Value,
		"first":       obj.First,
		"second":      obj.Second,
		"entityType":  obj.EntityType,
	}
	for name, el := range elements {
		if isZero := IsZeroValue(el); isZero {
			return &RequiredError{Field: name}
		}
	}

	if err := AssertReferenceValueRequired(obj.Observed); err != nil {
		return err
	}
	if err := AssertRangeValueTypeRequired(obj.Max); err != nil {
		return err
	}
	if err := AssertRangeValueTypeRequired(obj.Min); err != nil {
		return err
	}
	for _, el := range obj.Keys {
		if err := AssertKeyRequired(el); err != nil {
			return err
		}
	}
	if err := AssertReferenceValueRequired(obj.First); err != nil {
		return err
	}
	if err := AssertReferenceValueRequired(obj.Second); err != nil {
		return err
	}
	return nil
}

// AssertSubmodelElementValueConstraints checks if the values respects the defined constraints
func AssertSubmodelElementValueConstraints(obj SubmodelElementValue) error {
	if err := AssertReferenceValueConstraints(obj.Observed); err != nil {
		return err
	}
	if err := AssertRangeValueTypeConstraints(obj.Max); err != nil {
		return err
	}
	if err := AssertRangeValueTypeConstraints(obj.Min); err != nil {
		return err
	}
	for _, el := range obj.Keys {
		if err := AssertKeyConstraints(el); err != nil {
			return err
		}
	}
	if err := AssertReferenceValueConstraints(obj.First); err != nil {
		return err
	}
	if err := AssertReferenceValueConstraints(obj.Second); err != nil {
		return err
	}
	return nil
}
