/*
 * DotAAS Part 2 | HTTP/REST | Submodel Repository Service Specification
 *
 * The entire Submodel Repository Service Specification as part of the [Specification of the Asset Administration Shell: Part 2](http://industrialdigitaltwin.org/en/content-hub).   Publisher: Industrial Digital Twin Association (IDTA) 2023
 *
 * API version: V3.0.3_SSP-001
 * Contact: info@idtwin.org
 */

package openapi

type Range struct {
	Extensions []Extension `json:"extensions,omitempty"`

	Category string `json:"category,omitempty" validate:"regexp=^([\\\\x09\\\\x0a\\\\x0d\\\\x20-\\\\ud7ff\\\\ue000-\\\\ufffd]|\\\\ud800[\\\\udc00-\\\\udfff]|[\\\\ud801-\\\\udbfe][\\\\udc00-\\\\udfff]|\\\\udbff[\\\\udc00-\\\\udfff])*$"`

	IdShort string `json:"idShort,omitempty"`

	DisplayName []LangStringNameType `json:"displayName,omitempty"`

	Description []LangStringTextType `json:"description,omitempty"`

	ModelType string `json:"modelType" validate:"regexp=^Range$"`

	SemanticId *Reference `json:"semanticId,omitempty"`

	SupplementalSemanticIds []Reference `json:"supplementalSemanticIds,omitempty"`

	Qualifiers []Qualifier `json:"qualifiers,omitempty"`

	EmbeddedDataSpecifications []EmbeddedDataSpecification `json:"embeddedDataSpecifications,omitempty"`

	ValueType DataTypeDefXsd `json:"valueType"`

	Min string `json:"min,omitempty"`

	Max string `json:"max,omitempty"`
}

// Getters
func (a Range) GetExtensions() []Extension {
	return a.Extensions
}

func (a Range) GetIdShort() string {
	return a.IdShort
}

func (a Range) GetCategory() string {
	return a.Category
}

func (a Range) GetDisplayName() []LangStringNameType {
	return a.DisplayName
}

func (a Range) GetDescription() []LangStringTextType {
	return a.Description
}

func (a Range) GetModelType() string {
	return a.ModelType
}

func (a Range) GetSemanticId() *Reference {
	return a.SemanticId
}

func (a Range) GetSupplementalSemanticIds() []Reference {
	return a.SupplementalSemanticIds
}

func (a Range) GetQualifiers() []Qualifier {
	return a.Qualifiers
}

func (a Range) GetEmbeddedDataSpecifications() []EmbeddedDataSpecification {
	return a.EmbeddedDataSpecifications
}

// Setters

func (p *Range) SetModelType(modelType string) {
	p.ModelType = modelType
}

func (a *Range) SetExtensions(v []Extension) {
	a.Extensions = v
}

func (a *Range) SetIdShort(v string) {
	a.IdShort = v
}

func (a *Range) SetCategory(v string) {
	a.Category = v
}

func (a *Range) SetDisplayName(v []LangStringNameType) {
	a.DisplayName = v
}

func (a *Range) SetDescription(v []LangStringTextType) {
	a.Description = v
}

func (a *Range) SetSemanticId(v *Reference) {
	a.SemanticId = v
}

func (a *Range) SetSupplementalSemanticIds(v []Reference) {
	a.SupplementalSemanticIds = v
}

func (a *Range) SetQualifiers(v []Qualifier) {
	a.Qualifiers = v
}

func (a *Range) SetEmbeddedDataSpecifications(v []EmbeddedDataSpecification) {
	a.EmbeddedDataSpecifications = v
}

// AssertRangeRequired checks if the required fields are not zero-ed
func AssertRangeRequired(obj Range) error {
	elements := map[string]interface{}{
		"modelType": obj.ModelType,
		"valueType": obj.ValueType,
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
	if err := AssertReferenceRequired(*obj.SemanticId); err != nil {
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

// AssertRangeConstraints checks if the values respects the defined constraints
func AssertRangeConstraints(obj Range) error {
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
	if err := AssertReferenceConstraints(*obj.SemanticId); err != nil {
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
