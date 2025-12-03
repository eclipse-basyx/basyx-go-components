/*
 * DotAAS Part 2 | HTTP/REST | Submodel Repository Service Specification
 *
 * The entire Submodel Repository Service Specification as part of the [Specification of the Asset Administration Shell: Part 2](http://industrialdigitaltwin.org/en/content-hub).   Publisher: Industrial Digital Twin Association (IDTA) 2023
 *
 * API version: V3.0.3_SSP-001
 * Contact: info@idtwin.org
 */

package model

// Capability type of SubmodelElement
type Capability struct {
	Extensions []Extension `json:"extensions,omitempty"`

	Category string `json:"category,omitempty" validate:"regexp=^([\\\\x09\\\\x0a\\\\x0d\\\\x20-\\\\ud7ff\\\\ue000-\\\\ufffd]|\\\\ud800[\\\\udc00-\\\\udfff]|[\\\\ud801-\\\\udbfe][\\\\udc00-\\\\udfff]|\\\\udbff[\\\\udc00-\\\\udfff])*$"`

	//nolint:all
	IdShort string `json:"idShort,omitempty"`

	DisplayName []LangStringNameType `json:"displayName,omitempty"`

	Description []LangStringTextType `json:"description,omitempty"`

	ModelType string `json:"modelType" validate:"regexp=^Capability$"`

	SemanticID *Reference `json:"semanticId,omitempty"`

	//nolint:all
	SupplementalSemanticIds []Reference `json:"supplementalSemanticIds,omitempty"`

	Qualifiers []Qualifier `json:"qualifiers,omitempty"`

	EmbeddedDataSpecifications []EmbeddedDataSpecification `json:"embeddedDataSpecifications,omitempty"`
}

// Getters
//
//nolint:all
func (a Capability) GetExtensions() []Extension {
	return a.Extensions
}

//nolint:all
func (a Capability) GetIdShort() string {
	return a.IdShort
}

//nolint:all
func (a Capability) GetCategory() string {
	return a.Category
}

//nolint:all
func (a Capability) GetDisplayName() []LangStringNameType {
	return a.DisplayName
}

//nolint:all
func (a Capability) GetDescription() []LangStringTextType {
	return a.Description
}

//nolint:all
func (a Capability) GetModelType() string {
	return a.ModelType
}

//nolint:all
func (a Capability) GetSemanticID() *Reference {
	return a.SemanticID
}

//nolint:all
func (a Capability) GetSupplementalSemanticIds() []Reference {
	return a.SupplementalSemanticIds
}

//nolint:all
func (a Capability) GetQualifiers() []Qualifier {
	return a.Qualifiers
}

//nolint:all
func (a Capability) GetEmbeddedDataSpecifications() []EmbeddedDataSpecification {
	return a.EmbeddedDataSpecifications
}

// Setters

//nolint:all
func (a *Capability) SetModelType(v string) {
	a.ModelType = v
}

//nolint:all
func (a *Capability) SetExtensions(v []Extension) {
	a.Extensions = v
}

//nolint:all
func (a *Capability) SetIdShort(v string) {
	a.IdShort = v
}

//nolint:all
func (a *Capability) SetCategory(v string) {
	a.Category = v
}

//nolint:all
func (a *Capability) SetDisplayName(v []LangStringNameType) {
	a.DisplayName = v
}

//nolint:all
func (a *Capability) SetDescription(v []LangStringTextType) {
	a.Description = v
}

//nolint:all
func (a *Capability) SetSemanticID(v *Reference) {
	a.SemanticID = v
}

//nolint:all
func (a *Capability) SetSupplementalSemanticIds(v []Reference) {
	a.SupplementalSemanticIds = v
}

//nolint:all
func (a *Capability) SetQualifiers(v []Qualifier) {
	a.Qualifiers = v
}

//nolint:all
func (a *Capability) SetEmbeddedDataSpecifications(v []EmbeddedDataSpecification) {
	a.EmbeddedDataSpecifications = v
}

// AssertCapabilityRequired checks if the required fields are not zero-ed
func AssertCapabilityRequired(obj Capability) error {
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
	if err := AssertIdShortRequired(obj.IdShort); err != nil {
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
	if err := AssertReferenceRequired(*obj.SemanticID); err != nil {
		return err
	}
	for _, el := range obj.SupplementalSemanticIds {
		if err := AssertReferenceRequired(el); err != nil {
			return err
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

// AssertCapabilityConstraints checks if the values respects the defined constraints
func AssertCapabilityConstraints(obj Capability) error {
	for _, el := range obj.Extensions {
		if err := AssertExtensionConstraints(el); err != nil {
			return err
		}
	}
	if err := AssertstringConstraints(obj.IdShort); err != nil {
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
	if err := AssertReferenceConstraints(*obj.SemanticID); err != nil {
		return err
	}
	for _, el := range obj.SupplementalSemanticIds {
		if err := AssertReferenceConstraints(el); err != nil {
			return err
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

// ToValueOnly converts the Capability element to its Value Only representation.
// Capability elements have no value, so this always returns nil.
func (a *Capability) ToValueOnly() interface{} {
	return nil
}

// UpdateFromValueOnly updates the Capability element from a Value Only representation.
// Capability elements have no value, so this does nothing.
func (a *Capability) UpdateFromValueOnly(_ interface{}) error {
	// Capability has no value to update
	return nil
}
