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
 * DotAAS Part 1 | Metamodel | Schemas
 *
 * The schemas implementing the [Specification of the Asset Administration Shell: Part 1](https://industrialdigitaltwin.org/en/content-hub/aasspecifications).   Copyright: Industrial Digital Twin Association (IDTA) 2025
 *
 * API version: V3.1.1
 * Contact: info@idtwin.org
 */
//nolint:all
package model

// EventPayload type of EventPayload
type EventPayload struct {
	Source *Reference `json:"source"`

	SourceSemanticID *Reference `json:"sourceSemanticId,omitempty"`

	ObservableReference *Reference `json:"observableReference"`

	ObservableSemanticID *Reference `json:"observableSemanticId,omitempty"`

	Topic string `json:"topic,omitempty" validate:"regexp=^([\\\\x09\\\\x0a\\\\x0d\\\\x20-\\\\ud7ff\\\\ue000-\\\\ufffd]|\\\\ud800[\\\\udc00-\\\\udfff]|[\\\\ud801-\\\\udbfe][\\\\udc00-\\\\udfff]|\\\\udbff[\\\\udc00-\\\\udfff])*$"`

	SubjectID *Reference `json:"subjectId,omitempty"`

	TimeStamp string `json:"timeStamp" validate:"regexp=^-?(([1-9][0-9][0-9][0-9]+)|(0[0-9][0-9][0-9]))-((0[1-9])|(1[0-2]))-((0[1-9])|([12][0-9])|(3[01]))T(((([01][0-9])|(2[0-3])):[0-5][0-9]:([0-5][0-9])(\\\\.[0-9]+)?)|24:00:00(\\\\.0+)?)(Z|\\\\+00:00|-00:00)$"`

	Payload string `json:"payload,omitempty"`
}

// AssertEventPayloadRequired checks if the required fields are not zero-ed
func AssertEventPayloadRequired(obj EventPayload) error {
	elements := map[string]interface{}{
		"source":              obj.Source,
		"observableReference": obj.ObservableReference,
		"timeStamp":           obj.TimeStamp,
	}
	for name, el := range elements {
		if isZero := IsZeroValue(el); isZero {
			return &RequiredError{Field: name}
		}
	}

	if obj.Source != nil {
		if err := AssertReferenceRequired(*obj.Source); err != nil {
			return err
		}
	}
	if obj.ObservableReference != nil {
		if err := AssertReferenceRequired(*obj.ObservableReference); err != nil {
			return err
		}
	}
	if obj.ObservableReference != nil {
		if err := AssertReferenceRequired(*obj.ObservableReference); err != nil {
			return err
		}
	}
	if obj.ObservableSemanticID != nil {
		if err := AssertReferenceRequired(*obj.ObservableSemanticID); err != nil {
			return err
		}
	}
	if obj.SubjectID != nil {
		if err := AssertReferenceRequired(*obj.SubjectID); err != nil {
			return err
		}
	}
	return nil
}

// AssertEventPayloadConstraints checks if the values respects the defined constraints
func AssertEventPayloadConstraints(obj EventPayload) error {
	if obj.Source != nil {
		if err := AssertReferenceConstraints(*obj.Source); err != nil {
			return err
		}
	}
	if obj.SourceSemanticID != nil {
		if err := AssertReferenceConstraints(*obj.SourceSemanticID); err != nil {
			return err
		}
	}
	if obj.ObservableReference != nil {
		if err := AssertReferenceConstraints(*obj.ObservableReference); err != nil {
			return err
		}
	}
	if obj.ObservableSemanticID != nil {
		if err := AssertReferenceConstraints(*obj.ObservableSemanticID); err != nil {
			return err
		}
	}
	if obj.Topic != "" {
		if err := AssertReferenceConstraints(*obj.SubjectID); err != nil {
			return err
		}
	}
	return nil
}
