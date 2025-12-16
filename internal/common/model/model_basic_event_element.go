/*******************************************************************************
* Copyright (C) 2025 the Eclipse BaSyx Authors and Fraunhofer IESE
*
* Permission is hereby granted, free of charge, to any person obtaining
* a copy of this software and associated documentation files (the
* "Software"), to deal in the Software without restriction, including
* without limitation the rights to use, copy, modify, merge, publish,
* distribute, sublicense, and/or sell copies of the Software, and to
* permit persons to whom the Software is furnished to do so, subject to
* the following conditions:
*
* The above copyright notice and this permission notice shall be
* included in all copies or substantial portions of the Software.
*
* THE SOFTWARE IS PROVIDED "AS IS", WITHOUT WARRANTY OF ANY KIND,
* EXPRESS OR IMPLIED, INCLUDING BUT NOT LIMITED TO THE WARRANTIES OF
* MERCHANTABILITY, FITNESS FOR A PARTICULAR PURPOSE AND
* NONINFRINGEMENT. IN NO EVENT SHALL THE AUTHORS OR COPYRIGHT HOLDERS BE
* LIABLE FOR ANY CLAIM, DAMAGES OR OTHER LIABILITY, WHETHER IN AN ACTION
* OF CONTRACT, TORT OR OTHERWISE, ARISING FROM, OUT OF OR IN CONNECTION
* WITH THE SOFTWARE OR THE USE OR OTHER DEALINGS IN THE SOFTWARE.
*
* SPDX-License-Identifier: MIT
******************************************************************************/

/*
 * DotAAS Part 2 | HTTP/REST | Submodel Repository Service Specification
 *
 * The entire Submodel Repository Service Specification as part of the [Specification of the Asset Administration Shell: Part 2](https://industrialdigitaltwin.org/en/content-hub/aasspecifications).   Copyright: Industrial Digital Twin Association (IDTA) 2025
 *
 * API version: V3.1.1_SSP-001
 * Contact: info@idtwin.org
 */
//nolint:all
package model

type BasicEventElement struct {
	Extensions []Extension `json:"extensions,omitempty"`

	Category string `json:"category,omitempty" validate:"regexp=^([\\\\x09\\\\x0a\\\\x0d\\\\x20-\\\\ud7ff\\\\ue000-\\\\ufffd]|\\\\ud800[\\\\udc00-\\\\udfff]|[\\\\ud801-\\\\udbfe][\\\\udc00-\\\\udfff]|\\\\udbff[\\\\udc00-\\\\udfff])*$"`

	//nolint:all
	IdShort string `json:"idShort,omitempty"`

	DisplayName []LangStringNameType `json:"displayName,omitempty"`

	Description []LangStringTextType `json:"description,omitempty"`

	ModelType string `json:"modelType" validate:"regexp=^BasicEventElement$"`

	SemanticID *Reference `json:"semanticId,omitempty"`

	//nolint:all
	SupplementalSemanticIds []Reference `json:"supplementalSemanticIds,omitempty"`

	Qualifiers []Qualifier `json:"qualifiers,omitempty"`

	EmbeddedDataSpecifications []EmbeddedDataSpecification `json:"embeddedDataSpecifications,omitempty"`

	Observed *Reference `json:"observed"`

	Direction Direction `json:"direction"`

	State StateOfEvent `json:"state"`

	MessageTopic string `json:"messageTopic,omitempty" validate:"regexp=^([\\\\x09\\\\x0a\\\\x0d\\\\x20-\\\\ud7ff\\\\ue000-\\\\ufffd]|\\\\ud800[\\\\udc00-\\\\udfff]|[\\\\ud801-\\\\udbfe][\\\\udc00-\\\\udfff]|\\\\udbff[\\\\udc00-\\\\udfff])*$"`

	MessageBroker *Reference `json:"messageBroker,omitempty"`

	LastUpdate string `json:"lastUpdate,omitempty" validate:"regexp=^-?(([1-9][0-9][0-9][0-9]+)|(0[0-9][0-9][0-9]))-((0[1-9])|(1[0-2]))-((0[1-9])|([12][0-9])|(3[01]))T(((([01][0-9])|(2[0-3])):[0-5][0-9]:([0-5][0-9])(\\\\.[0-9]+)?)|24:00:00(\\\\.0+)?)(Z|\\\\+00:00|-00:00)$"`

	MinInterval string `json:"minInterval,omitempty" validate:"regexp=^-?P((([0-9]+Y([0-9]+M)?([0-9]+D)?|([0-9]+M)([0-9]+D)?|([0-9]+D))(T(([0-9]+H)([0-9]+M)?([0-9]+(\\\\.[0-9]+)?S)?|([0-9]+M)([0-9]+(\\\\.[0-9]+)?S)?|([0-9]+(\\\\.[0-9]+)?S)))?)|(T(([0-9]+H)([0-9]+M)?([0-9]+(\\\\.[0-9]+)?S)?|([0-9]+M)([0-9]+(\\\\.[0-9]+)?S)?|([0-9]+(\\\\.[0-9]+)?S))))$"`

	MaxInterval string `json:"maxInterval,omitempty" validate:"regexp=^-?P((([0-9]+Y([0-9]+M)?([0-9]+D)?|([0-9]+M)([0-9]+D)?|([0-9]+D))(T(([0-9]+H)([0-9]+M)?([0-9]+(\\\\.[0-9]+)?S)?|([0-9]+M)([0-9]+(\\\\.[0-9]+)?S)?|([0-9]+(\\\\.[0-9]+)?S)))?)|(T(([0-9]+H)([0-9]+M)?([0-9]+(\\\\.[0-9]+)?S)?|([0-9]+M)([0-9]+(\\\\.[0-9]+)?S)?|([0-9]+(\\\\.[0-9]+)?S))))$"`
}

// Getters
//
//nolint:all
func (a BasicEventElement) GetExtensions() []Extension {
	return a.Extensions
}

//nolint:all
func (a BasicEventElement) GetIdShort() string {
	return a.IdShort
}

//nolint:all
func (a BasicEventElement) GetCategory() string {
	return a.Category
}

//nolint:all
func (a BasicEventElement) GetDisplayName() []LangStringNameType {
	return a.DisplayName
}

//nolint:all
func (a BasicEventElement) GetDescription() []LangStringTextType {
	return a.Description
}

//nolint:all
func (a BasicEventElement) GetModelType() string {
	return a.ModelType
}

//nolint:all
func (a BasicEventElement) GetSemanticID() *Reference {
	return a.SemanticID
}

//nolint:all
func (a BasicEventElement) GetSupplementalSemanticIds() []Reference {
	return a.SupplementalSemanticIds
}

//nolint:all
func (a BasicEventElement) GetQualifiers() []Qualifier {
	return a.Qualifiers
}

//nolint:all
func (a BasicEventElement) GetEmbeddedDataSpecifications() []EmbeddedDataSpecification {
	return a.EmbeddedDataSpecifications
}

// Setters

//nolint:all
func (a *BasicEventElement) SetModelType(v string) {
	a.ModelType = v
}

//nolint:all
func (a *BasicEventElement) SetExtensions(v []Extension) {
	a.Extensions = v
}

//nolint:all
func (a *BasicEventElement) SetIdShort(v string) {
	a.IdShort = v
}

//nolint:all
func (a *BasicEventElement) SetCategory(v string) {
	a.Category = v
}

//nolint:all
func (a *BasicEventElement) SetDisplayName(v []LangStringNameType) {
	a.DisplayName = v
}

//nolint:all
func (a *BasicEventElement) SetDescription(v []LangStringTextType) {
	a.Description = v
}

//nolint:all
func (a *BasicEventElement) SetSemanticID(v *Reference) {
	a.SemanticID = v
}

//nolint:all
func (a *BasicEventElement) SetSupplementalSemanticIds(v []Reference) {
	a.SupplementalSemanticIds = v
}

//nolint:all
func (a *BasicEventElement) SetQualifiers(v []Qualifier) {
	a.Qualifiers = v
}

//nolint:all
func (a *BasicEventElement) SetEmbeddedDataSpecifications(v []EmbeddedDataSpecification) {
	a.EmbeddedDataSpecifications = v
}

// AssertBasicEventElementRequired checks if the required fields are not zero-ed
func AssertBasicEventElementRequired(obj BasicEventElement) error {
	elements := map[string]interface{}{
		"modelType": obj.ModelType,
		"observed":  obj.Observed,
		"direction": obj.Direction,
		"state":     obj.State,
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
	if obj.SemanticID != nil {
		if err := AssertReferenceRequired(*obj.SemanticID); err != nil {
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
	if obj.Observed != nil {
		if err := AssertReferenceRequired(*obj.Observed); err != nil {
			return err
		}
	}
	if obj.MessageBroker != nil {
		if err := AssertReferenceRequired(*obj.MessageBroker); err != nil {
			return err
		}
	}
	return nil
}

// AssertBasicEventElementConstraints checks if the values respects the defined constraints
func AssertBasicEventElementConstraints(obj BasicEventElement) error {
	for _, el := range obj.Extensions {
		if err := AssertExtensionConstraints(el); err != nil {
			return err
		}
	}
	if err := AssertStringConstraints(obj.IdShort); err != nil {
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
	if obj.Observed != nil {
		if err := AssertReferenceConstraints(*obj.Observed); err != nil {
			return err
		}
	}
	if obj.MessageBroker != nil {
		if err := AssertReferenceConstraints(*obj.MessageBroker); err != nil {
			return err
		}
	}
	return nil
}
