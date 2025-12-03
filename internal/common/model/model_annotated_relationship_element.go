/*
 * DotAAS Part 2 | HTTP/REST | Submodel Repository Service Specification
 *
 * The entire Submodel Repository Service Specification as part of the [Specification of the Asset Administration Shell: Part 2](http://industrialdigitaltwin.org/en/content-hub).   Publisher: Industrial Digital Twin Association (IDTA) 2023
 *
 * API version: V3.0.3_SSP-001
 * Contact: info@idtwin.org
 */

package model

import (
	"encoding/json"
	"fmt"

	jsoniter "github.com/json-iterator/go"
)

// AnnotatedRelationshipElement type of SubmodelElement
type AnnotatedRelationshipElement struct {
	Extensions []Extension `json:"extensions,omitempty"`

	Category string `json:"category,omitempty" validate:"regexp=^([\\\\x09\\\\x0a\\\\x0d\\\\x20-\\\\ud7ff\\\\ue000-\\\\ufffd]|\\\\ud800[\\\\udc00-\\\\udfff]|[\\\\ud801-\\\\udbfe][\\\\udc00-\\\\udfff]|\\\\udbff[\\\\udc00-\\\\udfff])*$"`

	//nolint:all
	IdShort string `json:"idShort,omitempty"`

	DisplayName []LangStringNameType `json:"displayName,omitempty"`

	Description []LangStringTextType `json:"description,omitempty"`

	ModelType string `json:"modelType" validate:"regexp=^AnnotatedRelationshipElement$"`

	SemanticID *Reference `json:"semanticId,omitempty"`

	//nolint:all
	SupplementalSemanticIds []Reference `json:"supplementalSemanticIds,omitempty"`

	Qualifiers []Qualifier `json:"qualifiers,omitempty"`

	EmbeddedDataSpecifications []EmbeddedDataSpecification `json:"embeddedDataSpecifications,omitempty"`

	First *Reference `json:"first"`

	Second *Reference `json:"second"`

	Annotations []SubmodelElement `json:"annotations,omitempty"`
}

// Getters
//
//nolint:all
func (a AnnotatedRelationshipElement) GetExtensions() []Extension {
	return a.Extensions
}

//nolint:all
func (a AnnotatedRelationshipElement) GetIdShort() string {
	return a.IdShort
}

//nolint:all
func (a AnnotatedRelationshipElement) GetCategory() string {
	return a.Category
}

//nolint:all
func (a AnnotatedRelationshipElement) GetDisplayName() []LangStringNameType {
	return a.DisplayName
}

//nolint:all
func (a AnnotatedRelationshipElement) GetDescription() []LangStringTextType {
	return a.Description
}

//nolint:all
func (a AnnotatedRelationshipElement) GetModelType() string {
	return a.ModelType
}

//nolint:all
func (a AnnotatedRelationshipElement) GetSemanticID() *Reference {
	return a.SemanticID
}

//nolint:all
func (a AnnotatedRelationshipElement) GetSupplementalSemanticIds() []Reference {
	return a.SupplementalSemanticIds
}

//nolint:all
func (a AnnotatedRelationshipElement) GetQualifiers() []Qualifier {
	return a.Qualifiers
}

//nolint:all
func (a AnnotatedRelationshipElement) GetEmbeddedDataSpecifications() []EmbeddedDataSpecification {
	return a.EmbeddedDataSpecifications
}

//nolint:all
func (a AnnotatedRelationshipElement) GetFirst() *Reference {
	return a.First
}

//nolint:all
func (a AnnotatedRelationshipElement) GetSecond() *Reference {
	return a.Second
}

//nolint:all
func (a AnnotatedRelationshipElement) GetAnnotations() []SubmodelElement {
	return a.Annotations
}

// Setters

//nolint:all
func (a *AnnotatedRelationshipElement) SetModelType(v string) {
	a.ModelType = v
}

//nolint:all
func (a *AnnotatedRelationshipElement) SetExtensions(v []Extension) {
	a.Extensions = v
}

//nolint:all
func (a *AnnotatedRelationshipElement) SetIdShort(v string) {
	a.IdShort = v
}

//nolint:all
func (a *AnnotatedRelationshipElement) SetCategory(v string) {
	a.Category = v
}

//nolint:all
func (a *AnnotatedRelationshipElement) SetDisplayName(v []LangStringNameType) {
	a.DisplayName = v
}

//nolint:all
func (a *AnnotatedRelationshipElement) SetDescription(v []LangStringTextType) {
	a.Description = v
}

//nolint:all
func (a *AnnotatedRelationshipElement) SetSemanticID(v *Reference) {
	a.SemanticID = v
}

//nolint:all
func (a *AnnotatedRelationshipElement) SetSupplementalSemanticIds(v []Reference) {
	a.SupplementalSemanticIds = v
}

//nolint:all
func (a *AnnotatedRelationshipElement) SetQualifiers(v []Qualifier) {
	a.Qualifiers = v
}

//nolint:all
func (a *AnnotatedRelationshipElement) SetEmbeddedDataSpecifications(v []EmbeddedDataSpecification) {
	a.EmbeddedDataSpecifications = v
}

//nolint:all
func (a *AnnotatedRelationshipElement) SetFirst(v Reference) {
	a.First = &v
}

//nolint:all
func (a *AnnotatedRelationshipElement) SetSecond(v Reference) {
	a.Second = &v
}

//nolint:all
func (a *AnnotatedRelationshipElement) SetAnnotations(v []SubmodelElement) {
	a.Annotations = v
}

// AssertAnnotatedRelationshipElementRequired checks if the required fields are not zero-ed
func AssertAnnotatedRelationshipElementRequired(obj AnnotatedRelationshipElement) error {
	elements := map[string]interface{}{
		"modelType": obj.ModelType,
		"first":     obj.First,
		"second":    obj.Second,
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
	if obj.First != nil {
		if err := AssertReferenceRequired(*obj.First); err != nil {
			return err
		}
	}
	if obj.Second != nil {
		if err := AssertReferenceRequired(*obj.Second); err != nil {
			return err
		}
	}

	return nil
}

// AssertAnnotatedRelationshipElementConstraints checks if the values respects the defined constraints
func AssertAnnotatedRelationshipElementConstraints(obj AnnotatedRelationshipElement) error {
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
	if obj.First != nil {
		if err := AssertReferenceConstraints(*obj.First); err != nil {
			return err
		}
	}
	if obj.Second != nil {
		if err := AssertReferenceConstraints(*obj.Second); err != nil {
			return err
		}
	}

	return nil
}

// ToValueOnly converts the AnnotatedRelationshipElement to its Value Only representation.
// Returns a map with "first", "second", and optionally "annotations" fields.
// Returns nil if either first or second reference is missing.
//
// Parameters:
//   - referenceSerializer: function to convert Reference to its value-only form
//   - annotationSerializer: function to convert annotation SubmodelElements to value-only form
//
// Example output:
//
//	{
//	  "first": {...},
//	  "second": {...},
//	  "annotations": {...}  // only if annotations exist
//	}
func (a *AnnotatedRelationshipElement) ToValueOnly(
	referenceSerializer func(Reference) interface{},
	annotationSerializer func([]SubmodelElement) interface{},
) interface{} {
	if a.First == nil || a.Second == nil {
		return nil
	}

	result := map[string]interface{}{
		"first":  referenceSerializer(*a.First),
		"second": referenceSerializer(*a.Second),
	}

	if len(a.Annotations) > 0 {
		result["annotations"] = annotationSerializer(a.Annotations)
	}

	return result
}

// UpdateFromValueOnly updates the AnnotatedRelationshipElement from a Value Only representation.
// Expects a map with "first", "second", and optionally "annotations" fields.
//
// Parameters:
//   - value: map containing "first", "second", and optional "annotations" keys
//   - referenceDeserializer: function to convert value-only form to Reference
//   - annotationDeserializer: function to convert value-only form to SubmodelElement slice
//
// Returns an error if:
//   - value is not a map
//   - reference or annotation deserialization fails
func (a *AnnotatedRelationshipElement) UpdateFromValueOnly(
	value interface{},
	referenceDeserializer func(interface{}) (*Reference, error),
	annotationDeserializer func(interface{}) ([]SubmodelElement, error),
) error {
	valueMap, ok := value.(map[string]interface{})
	if !ok {
		return fmt.Errorf("invalid value type for AnnotatedRelationshipElement: expected map, got %T", value)
	}

	if firstVal, ok := valueMap["first"]; ok {
		first, err := referenceDeserializer(firstVal)
		if err != nil {
			return fmt.Errorf("failed to deserialize 'first' reference: %w", err)
		}
		a.First = first
	}

	if secondVal, ok := valueMap["second"]; ok {
		second, err := referenceDeserializer(secondVal)
		if err != nil {
			return fmt.Errorf("failed to deserialize 'second' reference: %w", err)
		}
		a.Second = second
	}

	if annotationsVal, ok := valueMap["annotations"]; ok {
		annotations, err := annotationDeserializer(annotationsVal)
		if err != nil {
			return fmt.Errorf("failed to deserialize annotations: %w", err)
		}
		a.Annotations = annotations
	}

	return nil
}

func (a *AnnotatedRelationshipElement) UnmarshalJSON(data []byte) error {
	// Create a temporary struct with the same fields but Annotations as []json.RawMessage
	type Alias AnnotatedRelationshipElement
	aux := &struct {
		Annotations []json.RawMessage `json:"annotations,omitempty"`
		*Alias
	}{
		Alias: (*Alias)(a),
	}

	// Unmarshal into the temporary struct
	var json = jsoniter.ConfigCompatibleWithStandardLibrary
	if err := json.Unmarshal(data, aux); err != nil {
		return err
	}

	// Now process the Annotations field manually
	if aux.Annotations != nil {
		a.Annotations = make([]SubmodelElement, len(aux.Annotations))
		for i, rawElement := range aux.Annotations {
			element, err := UnmarshalSubmodelElement(rawElement)
			if err != nil {
				return fmt.Errorf("failed to unmarshal element at index %d: %w", i, err)
			}
			a.Annotations[i] = element
		}
	}

	return nil
}
