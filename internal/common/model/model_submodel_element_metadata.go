/*
 * DotAAS Part 2 | HTTP/REST | Submodel Repository Service Specification
 *
 * The entire Submodel Repository Service Specification as part of the [Specification of the Asset Administration Shell: Part 2](http://industrialdigitaltwin.org/en/content-hub).   Publisher: Industrial Digital Twin Association (IDTA) 2023
 *
 * API version: V3.0.3_SSP-001
 * Contact: info@idtwin.org
 */

package model

// SubmodelElementMetadata struct representing metadata of a SubmodelElement.
type SubmodelElementMetadata struct {
	Extensions []Extension `json:"extensions,omitempty"`

	Category string `json:"category,omitempty"`

	//nolint:all
	IdShort string `json:"idShort,omitempty"`

	DisplayName []LangStringNameType `json:"displayName,omitempty"`

	Description []LangStringTextType `json:"description,omitempty"`

	ModelType ModelType `json:"modelType"`

	EmbeddedDataSpecifications []EmbeddedDataSpecification `json:"embeddedDataSpecifications,omitempty"`

	SemanticID *Reference `json:"semanticId,omitempty"`

	//nolint:all
	SupplementalSemanticIds []Reference `json:"supplementalSemanticIds,omitempty"`

	Qualifiers []Qualifier `json:"qualifiers,omitempty"`

	Kind ModellingKind `json:"kind,omitempty"`

	Direction Direction `json:"direction,omitempty"`

	State StateOfEvent `json:"state,omitempty"`

	MessageTopic string `json:"messageTopic,omitempty"`

	MessageBroker *Reference `json:"messageBroker,omitempty"`

	LastUpdate string `json:"lastUpdate,omitempty"`

	MinInterval string `json:"minInterval,omitempty"`

	MaxInterval string `json:"maxInterval,omitempty"`

	ValueType DataTypeDefXsd `json:"valueType,omitempty"`

	OrderRelevant bool `json:"orderRelevant,omitempty"`

	//nolint:all
	SemanticIdListElement *Reference `json:"semanticIdListElement,omitempty"`

	TypeValueListElement ModelType `json:"typeValueListElement,omitempty"`

	ValueTypeListElement DataTypeDefXsd `json:"valueTypeListElement,omitempty"`
}

// AssertSubmodelElementMetadataRequired checks if the required fields are not zero-ed
func AssertSubmodelElementMetadataRequired(obj SubmodelElementMetadata) error {
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
	for _, el := range obj.EmbeddedDataSpecifications {
		if err := AssertEmbeddedDataSpecificationRequired(el); err != nil {
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
	if obj.MessageBroker != nil {
		if err := AssertReferenceRequired(*obj.MessageBroker); err != nil {
			return err
		}
	}
	if obj.SemanticIdListElement != nil {
		if err := AssertReferenceRequired(*obj.SemanticIdListElement); err != nil {
			return err
		}
	}
	return nil
}

// AssertSubmodelElementMetadataConstraints checks if the values respects the defined constraints
func AssertSubmodelElementMetadataConstraints(obj SubmodelElementMetadata) error {
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
	for _, el := range obj.EmbeddedDataSpecifications {
		if err := AssertEmbeddedDataSpecificationConstraints(el); err != nil {
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
	if obj.MessageBroker != nil {
		if err := AssertReferenceConstraints(*obj.MessageBroker); err != nil {
			return err
		}
	}
	if obj.SemanticIdListElement != nil {
		if err := AssertReferenceConstraints(*obj.SemanticIdListElement); err != nil {
			return err
		}
	}
	return nil
}
