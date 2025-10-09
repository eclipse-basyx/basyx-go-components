/*
 * DotAAS Part 2 | HTTP/REST | Submodel Repository Service Specification
 *
 * The entire Submodel Repository Service Specification as part of the [Specification of the Asset Administration Shell: Part 2](http://industrialdigitaltwin.org/en/content-hub).   Publisher: Industrial Digital Twin Association (IDTA) 2023
 *
 * API version: V3.0.3_SSP-001
 * Contact: info@idtwin.org
 */

package model

// Type of SubmodelElement
type Property struct {
	Extensions []Extension `json:"extensions,omitempty"`

	Category string `json:"category,omitempty" validate:"regexp=^([\\\\x09\\\\x0a\\\\x0d\\\\x20-\\\\ud7ff\\\\ue000-\\\\ufffd]|\\\\ud800[\\\\udc00-\\\\udfff]|[\\\\ud801-\\\\udbfe][\\\\udc00-\\\\udfff]|\\\\udbff[\\\\udc00-\\\\udfff])*$"`

	IdShort string `json:"idShort,omitempty"`

	DisplayName []LangStringNameType `json:"displayName,omitempty"`

	Description []LangStringTextType `json:"description,omitempty"`

	ModelType string `json:"modelType" validate:"regexp=^Property$"`

	SemanticId *Reference `json:"semanticId,omitempty"`

	SupplementalSemanticIds []Reference `json:"supplementalSemanticIds,omitempty"`

	Qualifiers []Qualifier `json:"qualifiers,omitempty"`

	EmbeddedDataSpecifications []EmbeddedDataSpecification `json:"embeddedDataSpecifications,omitempty"`

	ValueType DataTypeDefXsd `json:"valueType"`

	Value string `json:"value,omitempty"`

	ValueId *Reference `json:"valueId,omitempty"`
}

// Constructor
func NewProperty(valueType DataTypeDefXsd) *Property {
	return &Property{
		ValueType: valueType,
		ModelType: "Property",
	}
}

func (p Property) GetIdShort() string {
	return p.IdShort
}

func (p Property) GetCategory() string {
	return p.Category
}

func (p Property) GetDisplayName() []LangStringNameType {
	return p.DisplayName
}

func (p Property) GetDescription() []LangStringTextType {
	return p.Description
}

func (p Property) GetModelType() string {
	return p.ModelType
}

func (p Property) GetSemanticId() *Reference {
	return p.SemanticId
}

func (p Property) GetSupplementalSemanticIds() []Reference {
	return p.SupplementalSemanticIds
}

func (p Property) GetQualifiers() []Qualifier {
	return p.Qualifiers
}

func (p Property) GetEmbeddedDataSpecifications() []EmbeddedDataSpecification {
	return p.EmbeddedDataSpecifications
}

func (p Property) GetExtensions() []Extension {
	return p.Extensions
}

func (p Property) GetValueType() DataTypeDefXsd {
	return p.ValueType
}

func (p Property) GetValue() string {
	return p.Value
}

func (p Property) GetValueId() *Reference {
	return p.ValueId
}

func (p *Property) SetModelType(modelType string) {
	p.ModelType = modelType
}

func (p *Property) SetIdShort(idShort string) {
	p.IdShort = idShort
}

func (p *Property) SetCategory(category string) {
	p.Category = category
}

func (p *Property) SetDisplayName(displayName []LangStringNameType) {
	p.DisplayName = displayName
}

func (p *Property) SetDescription(description []LangStringTextType) {
	p.Description = description
}

func (p *Property) SetSemanticId(semanticId *Reference) {
	p.SemanticId = semanticId
}

func (p *Property) SetSupplementalSemanticIds(supplementalSemanticIds []Reference) {
	p.SupplementalSemanticIds = supplementalSemanticIds
}

func (p *Property) SetQualifiers(qualifiers []Qualifier) {
	p.Qualifiers = qualifiers
}

func (p *Property) SetEmbeddedDataSpecifications(embeddedDataSpecifications []EmbeddedDataSpecification) {
	p.EmbeddedDataSpecifications = embeddedDataSpecifications
}

func (p *Property) SetExtensions(extensions []Extension) {
	p.Extensions = extensions
}

// AssertPropertyRequired checks if the required fields are not zero-ed
func AssertPropertyRequired(obj Property) error {
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
	if obj.SemanticId != nil {
		if err := AssertReferenceRequired(*obj.SemanticId); err != nil {
			return err
		}
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
	if obj.ValueId != nil {
		if err := AssertReferenceRequired(*obj.ValueId); err != nil {
			return err
		}
	}
	return nil
}

// AssertPropertyConstraints checks if the values respects the defined constraints
func AssertPropertyConstraints(obj Property) error {
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
	if obj.SemanticId != nil {
		if err := AssertReferenceConstraints(*obj.SemanticId); err != nil {
			return err
		}
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
	if obj.ValueId != nil {
		if err := AssertReferenceConstraints(*obj.ValueId); err != nil {
			return err
		}
	}
	return nil
}
