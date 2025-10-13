/*
 * DotAAS Part 2 | HTTP/REST | Submodel Repository Service Specification
 *
 * The entire Submodel Repository Service Specification as part of the [Specification of the Asset Administration Shell: Part 2](http://industrialdigitaltwin.org/en/content-hub).   Publisher: Industrial Digital Twin Association (IDTA) 2023
 *
 * API version: V3.0.3_SSP-001
 * Contact: info@idtwin.org
 */

package model

type Reference struct {
	Type ReferenceTypes `json:"type"`

	Keys []Key `json:"keys"`

	ReferredSemanticId *Reference `json:"referredSemanticId,omitempty"`
}

// AssertReferenceRequired checks if the required fields are not zero-ed
func AssertReferenceRequired(obj Reference) error {
	elements := map[string]interface{}{
		"type": obj.Type,
		"keys": obj.Keys,
	}
	for name, el := range elements {
		if isZero := IsZeroValue(el); isZero {
			return &RequiredError{Field: name}
		}
	}

	for _, el := range obj.Keys {
		if err := AssertKeyRequired(el); err != nil {
			return err
		}
	}

	// Stack
	stack := make([]*Reference, 0)
	if obj.ReferredSemanticId != nil {
		stack = append(stack, obj.ReferredSemanticId)
	}
	for len(stack) > 0 {
		// Pop
		n := len(stack) - 1
		current := stack[n]
		stack = stack[:n]

		if err := AssertReferenceRequired(*current); err != nil {
			return err
		}

		if current.ReferredSemanticId != nil {
			stack = append(stack, current.ReferredSemanticId)
		}
	}

	return nil
}

// AssertReferenceConstraints checks if the values respects the defined constraints
func AssertReferenceConstraints(obj Reference) error {
	for _, el := range obj.Keys {
		if err := AssertKeyConstraints(el); err != nil {
			return err
		}
	}

	stack := make([]*Reference, 0)
	if obj.ReferredSemanticId != nil {
		stack = append(stack, obj.ReferredSemanticId)
	}
	for len(stack) > 0 {
		n := len(stack) - 1
		current := stack[n]
		stack = stack[:n]

		if err := AssertReferenceConstraints(*current); err != nil {
			return err
		}

		if current.ReferredSemanticId != nil {
			stack = append(stack, current.ReferredSemanticId)
		}
	}

	return nil
}
