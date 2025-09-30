/*
 * DotAAS Part 2 | HTTP/REST | Submodel Repository Service Specification
 *
 * The entire Submodel Repository Service Specification as part of the [Specification of the Asset Administration Shell: Part 2](http://industrialdigitaltwin.org/en/content-hub).   Publisher: Industrial Digital Twin Association (IDTA) 2023
 *
 * API version: V3.0.3_SSP-001
 * Contact: info@idtwin.org
 */

package openapi

type Entity struct {
	Extensions []Extension `json:"extensions,omitempty"`

	Category string `json:"category,omitempty" validate:"regexp=^([\\\\x09\\\\x0a\\\\x0d\\\\x20-\\\\ud7ff\\\\ue000-\\\\ufffd]|\\\\ud800[\\\\udc00-\\\\udfff]|[\\\\ud801-\\\\udbfe][\\\\udc00-\\\\udfff]|\\\\udbff[\\\\udc00-\\\\udfff])*$"`

	IdShort string `json:"idShort,omitempty"`

	DisplayName []LangStringNameType `json:"displayName,omitempty"`

	Description []LangStringTextType `json:"description,omitempty"`

	ModelType string `json:"modelType" validate:"regexp=^Entity$"`

	SemanticId *Reference `json:"semanticId,omitempty"`

	SupplementalSemanticIds []Reference `json:"supplementalSemanticIds,omitempty"`

	Qualifiers []Qualifier `json:"qualifiers,omitempty"`

	EmbeddedDataSpecifications []EmbeddedDataSpecification `json:"embeddedDataSpecifications,omitempty"`

	Statements []SubmodelElement `json:"statements,omitempty"`

	EntityType EntityType `json:"entityType"`

	GlobalAssetId string `json:"globalAssetId,omitempty" validate:"regexp=^([\\\\x09\\\\x0a\\\\x0d\\\\x20-\\\\ud7ff\\\\ue000-\\\\ufffd]|\\\\ud800[\\\\udc00-\\\\udfff]|[\\\\ud801-\\\\udbfe][\\\\udc00-\\\\udfff]|\\\\udbff[\\\\udc00-\\\\udfff])*$"`

	SpecificAssetIds []SpecificAssetId `json:"specificAssetIds,omitempty"`
}

// Getters
func (a Entity) GetExtensions() []Extension {
	return a.Extensions
}

func (a Entity) GetIdShort() string {
	return a.IdShort
}

func (a Entity) GetCategory() string {
	return a.Category
}

func (a Entity) GetDisplayName() []LangStringNameType {
	return a.DisplayName
}

func (a Entity) GetDescription() []LangStringTextType {
	return a.Description
}

func (a Entity) GetModelType() string {
	return a.ModelType
}

func (a Entity) GetSemanticId() *Reference {
	return a.SemanticId
}

func (a Entity) GetSupplementalSemanticIds() []Reference {
	return a.SupplementalSemanticIds
}

func (a Entity) GetQualifiers() []Qualifier {
	return a.Qualifiers
}

func (a Entity) GetEmbeddedDataSpecifications() []EmbeddedDataSpecification {
	return a.EmbeddedDataSpecifications
}

// Setters

func (a *Entity) SetModelType(v string) {
	a.ModelType = v
}

func (a *Entity) SetExtensions(v []Extension) {
	a.Extensions = v
}

func (a *Entity) SetIdShort(v string) {
	a.IdShort = v
}

func (a *Entity) SetCategory(v string) {
	a.Category = v
}

func (a *Entity) SetDisplayName(v []LangStringNameType) {
	a.DisplayName = v
}

func (a *Entity) SetDescription(v []LangStringTextType) {
	a.Description = v
}

func (a *Entity) SetSemanticId(v *Reference) {
	a.SemanticId = v
}

func (a *Entity) SetSupplementalSemanticIds(v []Reference) {
	a.SupplementalSemanticIds = v
}

func (a *Entity) SetQualifiers(v []Qualifier) {
	a.Qualifiers = v
}

func (a *Entity) SetEmbeddedDataSpecifications(v []EmbeddedDataSpecification) {
	a.EmbeddedDataSpecifications = v
}

// AssertEntityRequired checks if the required fields are not zero-ed
func AssertEntityRequired(obj Entity) error {
	elements := map[string]interface{}{
		"modelType":  obj.ModelType,
		"entityType": obj.EntityType,
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
	for _, el := range obj.SpecificAssetIds {
		if err := AssertSpecificAssetIdRequired(el); err != nil {
			return err
		}
	}
	return nil
}

// AssertEntityConstraints checks if the values respects the defined constraints
func AssertEntityConstraints(obj Entity) error {
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
	for _, el := range obj.SpecificAssetIds {
		if err := AssertSpecificAssetIdConstraints(el); err != nil {
			return err
		}
	}
	return nil
}
