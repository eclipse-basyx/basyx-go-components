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
	"fmt"

	jsoniter "github.com/json-iterator/go"
)

// SubmodelElement interface representing a SubmodelElement.
type SubmodelElement interface {
	GetModelType() string
	GetIdShort() string
	GetCategory() string
	GetDisplayName() []LangStringNameType
	GetDescription() []LangStringTextType
	GetSemanticID() *Reference
	GetSupplementalSemanticIds() []Reference
	GetQualifiers() []Qualifier
	GetEmbeddedDataSpecifications() []EmbeddedDataSpecification
	GetExtensions() []Extension

	SetModelType(string)
	SetIdShort(string)
	SetCategory(string)
	SetDisplayName([]LangStringNameType)
	SetDescription([]LangStringTextType)
	SetSemanticID(*Reference)
	SetSupplementalSemanticIds([]Reference)
	SetQualifiers([]Qualifier)
	SetEmbeddedDataSpecifications([]EmbeddedDataSpecification)
	SetExtensions([]Extension)
}

// UnmarshalSubmodelElement creates the appropriate concrete SubmodelElement type from JSON
func UnmarshalSubmodelElement(data []byte) (SubmodelElement, error) {
	// First, determine the modelType
	var raw struct {
		ModelType string `json:"modelType"`
	}
	var json = jsoniter.ConfigCompatibleWithStandardLibrary

	if err := json.Unmarshal(data, &raw); err != nil {
		return nil, fmt.Errorf("failed to determine modelType: %w", err)
	}

	// Create the appropriate concrete type based on modelType
	switch raw.ModelType {
	case "Property":
		var prop Property
		if err := json.Unmarshal(data, &prop); err != nil {
			return nil, fmt.Errorf("failed to unmarshal Property: %w", err)
		}
		return &prop, nil
	case "MultiLanguageProperty":
		var mlp MultiLanguageProperty
		if err := json.Unmarshal(data, &mlp); err != nil {
			return nil, fmt.Errorf("failed to unmarshal MultiLanguageProperty: %w", err)
		}
		return &mlp, nil
	case "Range":
		var r Range
		if err := json.Unmarshal(data, &r); err != nil {
			return nil, fmt.Errorf("failed to unmarshal Range: %w", err)
		}
		return &r, nil
	case "File":
		var f File
		if err := json.Unmarshal(data, &f); err != nil {
			return nil, fmt.Errorf("failed to unmarshal File: %w", err)
		}
		return &f, nil
	case "Blob":
		var b Blob
		if err := json.Unmarshal(data, &b); err != nil {
			return nil, fmt.Errorf("failed to unmarshal Blob: %w", err)
		}
		return &b, nil
	case "ReferenceElement":
		var re ReferenceElement
		if err := json.Unmarshal(data, &re); err != nil {
			return nil, fmt.Errorf("failed to unmarshal ReferenceElement: %w", err)
		}
		return &re, nil
	case "RelationshipElement":
		var re RelationshipElement
		if err := json.Unmarshal(data, &re); err != nil {
			return nil, fmt.Errorf("failed to unmarshal RelationshipElement: %w", err)
		}
		return &re, nil
	case "AnnotatedRelationshipElement":
		var are AnnotatedRelationshipElement
		if err := json.Unmarshal(data, &are); err != nil {
			return nil, fmt.Errorf("failed to unmarshal AnnotatedRelationshipElement: %w", err)
		}
		return &are, nil
	case "Entity":
		var e Entity
		if err := json.Unmarshal(data, &e); err != nil {
			return nil, fmt.Errorf("failed to unmarshal Entity: %w", err)
		}
		return &e, nil
	case "Operation":
		var op Operation
		if err := json.Unmarshal(data, &op); err != nil {
			return nil, fmt.Errorf("failed to unmarshal Operation: %w", err)
		}
		return &op, nil
	case "BasicEventElement":
		var bee BasicEventElement
		if err := json.Unmarshal(data, &bee); err != nil {
			return nil, fmt.Errorf("failed to unmarshal BasicEventElement: %w", err)
		}
		return &bee, nil
	case "Capability":
		var capability Capability
		if err := json.Unmarshal(data, &capability); err != nil {
			return nil, fmt.Errorf("failed to unmarshal Capability: %w", err)
		}
		return &capability, nil
	case "SubmodelElementCollection":
		var sec SubmodelElementCollection
		if err := json.Unmarshal(data, &sec); err != nil {
			return nil, fmt.Errorf("failed to unmarshal SubmodelElementCollection: %w", err)
		}
		return &sec, nil
	case "SubmodelElementList":
		var sel SubmodelElementList
		if err := json.Unmarshal(data, &sel); err != nil {
			return nil, fmt.Errorf("failed to unmarshal SubmodelElementList: %w", err)
		}
		return &sel, nil
	default:
		return nil, fmt.Errorf("unsupported modelType: %s (supported types: Property, MultiLanguageProperty, Range, File, Blob, ReferenceElement, RelationshipElement, AnnotatedRelationshipElement, Entity, Operation, BasicEventElement, Capability, SubmodelElementCollection, SubmodelElementList)", raw.ModelType)
	}
}

// AssertSubmodelElementRequired checks if the required fields are not zero-ed
func AssertSubmodelElementRequired(obj SubmodelElement) error {
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
	if err := AssertIdShortRequired(obj.GetIdShort()); err != nil {
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
	if err := AssertReferenceRequired(*obj.GetSemanticID()); err != nil {
		return err
	}
	for _, el := range obj.GetSupplementalSemanticIds() {
		if err := AssertReferenceRequired(el); err != nil {
			return err
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

// AssertSubmodelElementConstraints checks if the values respects the defined constraints
func AssertSubmodelElementConstraints(obj SubmodelElement) error {
	for _, el := range obj.GetExtensions() {
		if err := AssertExtensionConstraints(el); err != nil {
			return err
		}
	}
	if err := AssertstringConstraints(obj.GetIdShort()); err != nil {
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
	if err := AssertReferenceConstraints(*obj.GetSemanticID()); err != nil {
		return err
	}
	for _, el := range obj.GetSupplementalSemanticIds() {
		if err := AssertReferenceConstraints(el); err != nil {
			return err
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
