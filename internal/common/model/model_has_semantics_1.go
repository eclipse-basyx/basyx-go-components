/*
 * DotAAS Part 2 | HTTP/REST | Submodel Repository Service Specification
 *
 * The entire Submodel Repository Service Specification as part of the [Specification of the Asset Administration Shell: Part 2](http://industrialdigitaltwin.org/en/content-hub).   Publisher: Industrial Digital Twin Association (IDTA) 2023
 *
 * API version: V3.0.3_SSP-001
 * Contact: info@idtwin.org
 */

package model

type HasSemantics1 struct {
	SemanticId *Reference `json:"semanticId,omitempty"`

	SupplementalSemanticIds []Reference `json:"supplementalSemanticIds,omitempty"`
}

// AssertHasSemantics1Required checks if the required fields are not zero-ed
func AssertHasSemantics1Required(obj HasSemantics1) error {
	if err := AssertReferenceRequired(*obj.SemanticId); err != nil {
		return err
	}
	for _, el := range obj.SupplementalSemanticIds {
		if err := AssertReferenceRequired(el); err != nil {
			return err
		}
	}
	return nil
}

// AssertHasSemantics1Constraints checks if the values respects the defined constraints
func AssertHasSemantics1Constraints(obj HasSemantics1) error {
	if err := AssertReferenceConstraints(*obj.SemanticId); err != nil {
		return err
	}
	for _, el := range obj.SupplementalSemanticIds {
		if err := AssertReferenceConstraints(el); err != nil {
			return err
		}
	}
	return nil
}
