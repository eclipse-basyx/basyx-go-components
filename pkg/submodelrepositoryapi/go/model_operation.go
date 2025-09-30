/*
 * DotAAS Part 2 | HTTP/REST | Submodel Repository Service Specification
 *
 * The entire Submodel Repository Service Specification as part of the [Specification of the Asset Administration Shell: Part 2](http://industrialdigitaltwin.org/en/content-hub).   Publisher: Industrial Digital Twin Association (IDTA) 2023
 *
 * API version: V3.0.3_SSP-001
 * Contact: info@idtwin.org
 */

package openapi

type Operation struct {
	Extensions []Extension `json:"extensions,omitempty"`

	Category string `json:"category,omitempty" validate:"regexp=^([\\\\x09\\\\x0a\\\\x0d\\\\x20-\\\\ud7ff\\\\ue000-\\\\ufffd]|\\\\ud800[\\\\udc00-\\\\udfff]|[\\\\ud801-\\\\udbfe][\\\\udc00-\\\\udfff]|\\\\udbff[\\\\udc00-\\\\udfff])*$"`

	IdShort string `json:"idShort,omitempty"`

	DisplayName []LangStringNameType `json:"displayName,omitempty"`

	Description []LangStringTextType `json:"description,omitempty"`

	ModelType string `json:"modelType" validate:"regexp=^Operation$"`

	SemanticId *Reference `json:"semanticId,omitempty"`

	SupplementalSemanticIds []Reference `json:"supplementalSemanticIds,omitempty"`

	Qualifiers []Qualifier `json:"qualifiers,omitempty"`

	EmbeddedDataSpecifications []EmbeddedDataSpecification `json:"embeddedDataSpecifications,omitempty"`

	InputVariables []OperationVariable `json:"inputVariables,omitempty"`

	OutputVariables []OperationVariable `json:"outputVariables,omitempty"`

	InoutputVariables []OperationVariable `json:"inoutputVariables,omitempty"`
}

// Getters
func (a Operation) GetExtensions() []Extension {
	return a.Extensions
}

func (a Operation) GetIdShort() string {
	return a.IdShort
}

func (a Operation) GetCategory() string {
	return a.Category
}

func (a Operation) GetDisplayName() []LangStringNameType {
	return a.DisplayName
}

func (a Operation) GetDescription() []LangStringTextType {
	return a.Description
}

func (a Operation) GetModelType() string {
	return a.ModelType
}

func (a Operation) GetSemanticId() *Reference {
	return a.SemanticId
}

func (a Operation) GetSupplementalSemanticIds() []Reference {
	return a.SupplementalSemanticIds
}

func (a Operation) GetQualifiers() []Qualifier {
	return a.Qualifiers
}

func (a Operation) GetEmbeddedDataSpecifications() []EmbeddedDataSpecification {
	return a.EmbeddedDataSpecifications
}

// Setters

func (a *Operation) SetModelType(modelType string) {
	a.ModelType = modelType
}

func (a *Operation) SetExtensions(v []Extension) {
	a.Extensions = v
}

func (a *Operation) SetIdShort(v string) {
	a.IdShort = v
}

func (a *Operation) SetCategory(v string) {
	a.Category = v
}

func (a *Operation) SetDisplayName(v []LangStringNameType) {
	a.DisplayName = v
}

func (a *Operation) SetDescription(v []LangStringTextType) {
	a.Description = v
}

func (a *Operation) SetSemanticId(v *Reference) {
	a.SemanticId = v
}

func (a *Operation) SetSupplementalSemanticIds(v []Reference) {
	a.SupplementalSemanticIds = v
}

func (a *Operation) SetQualifiers(v []Qualifier) {
	a.Qualifiers = v
}

func (a *Operation) SetEmbeddedDataSpecifications(v []EmbeddedDataSpecification) {
	a.EmbeddedDataSpecifications = v
}

// AssertOperationRequired checks if the required fields are not zero-ed
func AssertOperationRequired(obj Operation) error {
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
	for _, el := range obj.InputVariables {
		if err := AssertOperationVariableRequired(el); err != nil {
			return err
		}
	}
	for _, el := range obj.OutputVariables {
		if err := AssertOperationVariableRequired(el); err != nil {
			return err
		}
	}
	for _, el := range obj.InoutputVariables {
		if err := AssertOperationVariableRequired(el); err != nil {
			return err
		}
	}
	return nil
}

// AssertOperationConstraints checks if the values respects the defined constraints
func AssertOperationConstraints(obj Operation) error {
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
	for _, el := range obj.InputVariables {
		if err := AssertOperationVariableConstraints(el); err != nil {
			return err
		}
	}
	for _, el := range obj.OutputVariables {
		if err := AssertOperationVariableConstraints(el); err != nil {
			return err
		}
	}
	for _, el := range obj.InoutputVariables {
		if err := AssertOperationVariableConstraints(el); err != nil {
			return err
		}
	}
	return nil
}
