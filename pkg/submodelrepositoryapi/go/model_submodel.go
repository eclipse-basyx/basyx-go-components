/*
 * DotAAS Part 2 | HTTP/REST | Submodel Repository Service Specification
 *
 * The entire Submodel Repository Service Specification as part of the [Specification of the Asset Administration Shell: Part 2](http://industrialdigitaltwin.org/en/content-hub).   Publisher: Industrial Digital Twin Association (IDTA) 2023
 *
 * API version: V3.0.3_SSP-001
 * Contact: info@idtwin.org
 */

package openapi

import (
	"encoding/json"
)

type Submodel struct {
	Extensions []Extension `json:"extensions,omitempty"`

	Category string `json:"category,omitempty" validate:"regexp=^([\\\\x09\\\\x0a\\\\x0d\\\\x20-\\\\ud7ff\\\\ue000-\\\\ufffd]|\\\\ud800[\\\\udc00-\\\\udfff]|[\\\\ud801-\\\\udbfe][\\\\udc00-\\\\udfff]|\\\\udbff[\\\\udc00-\\\\udfff])*$"`

	IdShort string `json:"idShort,omitempty"`

	DisplayName *[]LangStringNameType `json:"displayName,omitempty"`

	Description *[]LangStringTextType `json:"description,omitempty"`

	ModelType string `json:"modelType" validate:"regexp=^Submodel$"`

	Administration *AdministrativeInformation `json:"administration,omitempty"`

	Id string `json:"id" validate:"regexp=^([\\\\x09\\\\x0a\\\\x0d\\\\x20-\\\\ud7ff\\\\ue000-\\\\ufffd]|\\\\ud800[\\\\udc00-\\\\udfff]|[\\\\ud801-\\\\udbfe][\\\\udc00-\\\\udfff]|\\\\udbff[\\\\udc00-\\\\udfff])*$"`

	Kind ModellingKind `json:"kind,omitempty"`

	SemanticId *Reference `json:"semanticId,omitempty"`

	SupplementalSemanticIds []Reference `json:"supplementalSemanticIds,omitempty"`

	Qualifiers []Qualifier `json:"qualifiers,omitempty"`

	EmbeddedDataSpecifications []EmbeddedDataSpecification `json:"embeddedDataSpecifications,omitempty"`

	SubmodelElements []SubmodelElement `json:"submodelElements,omitempty"`
}

// UnmarshalJSON implements custom unmarshaling for Submodel to handle polymorphic SubmodelElements
func (s *Submodel) UnmarshalJSON(data []byte) error {
	type Alias Submodel
	aux := &struct {
		SubmodelElements []json.RawMessage `json:"submodelElements,omitempty"`
		*Alias
	}{
		Alias: (*Alias)(s),
	}
	if err := json.Unmarshal(data, aux); err != nil {
		return err
	}
	s.SubmodelElements = make([]SubmodelElement, len(aux.SubmodelElements))
	for i, raw := range aux.SubmodelElements {
		elem, err := UnmarshalSubmodelElement(raw)
		if err != nil {
			return err
		}
		s.SubmodelElements[i] = elem
	}
	return nil
}

// AssertSubmodelRequired checks if the required fields are not zero-ed
func AssertSubmodelRequired(obj Submodel) error {
	elements := map[string]interface{}{
		"modelType": obj.ModelType,
		"id":        obj.Id,
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
	if obj.DisplayName != nil {
		for _, el := range *obj.DisplayName {
			if err := AssertLangStringNameTypeRequired(el); err != nil {
				return err
			}
		}
	}
	if obj.Description != nil {
		for _, el := range *obj.Description {
			if err := AssertLangStringTextTypeRequired(el); err != nil {
				return err
			}
		}
	}
	if obj.Administration != nil {
		if err := AssertAdministrativeInformationRequired(*obj.Administration); err != nil {
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
	return nil
}

// AssertSubmodelConstraints checks if the values respects the defined constraints
func AssertSubmodelConstraints(obj Submodel) error {
	for _, el := range obj.Extensions {
		if err := AssertExtensionConstraints(el); err != nil {
			return err
		}
	}
	if err := AssertReferableAllOfIdShortConstraints(obj.IdShort); err != nil {
		return err
	}
	if obj.DisplayName != nil {
		for _, el := range *obj.DisplayName {
			if err := AssertLangStringNameTypeConstraints(el); err != nil {
				return err
			}
		}
	}
	if obj.Description != nil {
		for _, el := range *obj.Description {
			if err := AssertLangStringTextTypeConstraints(el); err != nil {
				return err
			}
		}
	}
	if obj.Administration != nil {
		if err := AssertAdministrativeInformationConstraints(*obj.Administration); err != nil {
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
	return nil
}
