package openapi

import "reflect"

func isReferenceEmpty(ref Reference) bool {
	return reflect.DeepEqual(ref, Reference{})
}

// AssertSubmodelElementRequiredFixed is a fixed version of AssertSubmodelElementRequired
// that doesn't validate empty optional references
func AssertSubmodelElementRequiredFixed(obj SubmodelElement) error {
	elements := map[string]interface{}{
		"modelType": obj.GetModelType(),
	}
	for name, el := range elements {
		if isZero := IsZeroValue(el); isZero {
			return &RequiredError{Field: name}
		}
	}

	for _, el := range obj.GetExtensions() {
		if err := AssertExtensionRequired(el); err != nil {
			return err
		}
	}
	if err := AssertReferableAllOfIdShortRequired(obj.GetIdShort()); err != nil {
		return err
	}
	for _, el := range obj.GetDisplayName() {
		if err := AssertLangStringNameTypeRequired(el); err != nil {
			return err
		}
	}
	for _, el := range obj.GetDescription() {
		if err := AssertLangStringTextTypeRequired(el); err != nil {
			return err
		}
	}
	// Only validate non-empty references
	if obj.GetSemanticId() != nil && !isReferenceEmpty(*obj.GetSemanticId()) {
		if err := AssertReferenceRequired(*obj.GetSemanticId()); err != nil {
			return err
		}
	}
	for _, el := range obj.GetSupplementalSemanticIds() {
		if !isReferenceEmpty(el) {
			if err := AssertReferenceRequired(el); err != nil {
				return err
			}
		}
	}
	for _, el := range obj.GetQualifiers() {
		if err := AssertQualifierRequired(el); err != nil {
			return err
		}
	}
	for _, el := range obj.GetEmbeddedDataSpecifications() {
		if err := AssertEmbeddedDataSpecificationRequired(el); err != nil {
			return err
		}
	}
	return nil
}

// AssertSubmodelElementConstraintsFixed is a fixed version of AssertSubmodelElementConstraints
// that doesn't validate empty optional references
func AssertSubmodelElementConstraintsFixed(obj SubmodelElement) error {
	for _, el := range obj.GetExtensions() {
		if err := AssertExtensionConstraints(el); err != nil {
			return err
		}
	}
	if err := AssertReferableAllOfIdShortConstraints(obj.GetIdShort()); err != nil {
		return err
	}
	for _, el := range obj.GetDisplayName() {
		if err := AssertLangStringNameTypeConstraints(el); err != nil {
			return err
		}
	}
	for _, el := range obj.GetDescription() {
		if err := AssertLangStringTextTypeConstraints(el); err != nil {
			return err
		}
	}
	// Only validate constraints on non-empty references
	if obj.GetSemanticId() != nil && !isReferenceEmpty(*obj.GetSemanticId()) {
		if err := AssertReferenceConstraints(*obj.GetSemanticId()); err != nil {
			return err
		}
	}
	for _, el := range obj.GetSupplementalSemanticIds() {
		if !isReferenceEmpty(el) {
			if err := AssertReferenceConstraints(el); err != nil {
				return err
			}
		}
	}
	for _, el := range obj.GetQualifiers() {
		if err := AssertQualifierConstraints(el); err != nil {
			return err
		}
	}
	for _, el := range obj.GetEmbeddedDataSpecifications() {
		if err := AssertEmbeddedDataSpecificationConstraints(el); err != nil {
			return err
		}
	}
	return nil
}
