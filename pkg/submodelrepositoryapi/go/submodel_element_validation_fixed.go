package openapi

// AssertSubmodelElementRequiredFixed is a fixed version of AssertSubmodelElementRequired 
// that doesn't validate empty optional references
func AssertSubmodelElementRequiredFixed(obj SubmodelElement) error {
	elements := map[string]interface{}{
		"modelType": obj.ModelType,
	}
	for name, el := range elements {
		if isZero := IsZeroValue(el); isZero {
			return &RequiredError{Field: name}
		}
	}

	for _, el := range obj.Extensions {
		if err := AssertExtensionRequired(el); err != nil {
			return err
		}
	}
	if err := AssertReferableAllOfIdShortRequired(obj.IdShort); err != nil {
		return err
	}
	for _, el := range obj.DisplayName {
		if err := AssertLangStringNameTypeRequired(el); err != nil {
			return err
		}
	}
	for _, el := range obj.Description {
		if err := AssertLangStringTextTypeRequired(el); err != nil {
			return err
		}
	}
	// Only validate non-empty references
	if !isEmptyReference(obj.SemanticId) {
		if err := AssertReferenceRequired(obj.SemanticId); err != nil {
			return err
		}
	}
	for _, el := range obj.SupplementalSemanticIds {
		if !isEmptyReference(el) {
			if err := AssertReferenceRequired(el); err != nil {
				return err
			}
		}
	}
	for _, el := range obj.Qualifiers {
		if err := AssertQualifierRequired(el); err != nil {
			return err
		}
	}
	for _, el := range obj.EmbeddedDataSpecifications {
		if err := AssertEmbeddedDataSpecificationRequired(el); err != nil {
			return err
		}
	}
	return nil
}

// AssertSubmodelElementConstraintsFixed is a fixed version of AssertSubmodelElementConstraints 
// that doesn't validate empty optional references
func AssertSubmodelElementConstraintsFixed(obj SubmodelElement) error {
	for _, el := range obj.Extensions {
		if err := AssertExtensionConstraints(el); err != nil {
			return err
		}
	}
	if err := AssertReferableAllOfIdShortConstraints(obj.IdShort); err != nil {
		return err
	}
	for _, el := range obj.DisplayName {
		if err := AssertLangStringNameTypeConstraints(el); err != nil {
			return err
		}
	}
	for _, el := range obj.Description {
		if err := AssertLangStringTextTypeConstraints(el); err != nil {
			return err
		}
	}
	// Only validate constraints on non-empty references
	if !isEmptyReference(obj.SemanticId) {
		if err := AssertReferenceConstraints(obj.SemanticId); err != nil {
			return err
		}
	}
	for _, el := range obj.SupplementalSemanticIds {
		if !isEmptyReference(el) {
			if err := AssertReferenceConstraints(el); err != nil {
				return err
			}
		}
	}
	for _, el := range obj.Qualifiers {
		if err := AssertQualifierConstraints(el); err != nil {
			return err
		}
	}
	for _, el := range obj.EmbeddedDataSpecifications {
		if err := AssertEmbeddedDataSpecificationConstraints(el); err != nil {
			return err
		}
	}
	return nil
}
