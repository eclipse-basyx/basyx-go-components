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
 * The entire Submodel Repository Service Specification as part of the [Specification of the Asset Administration Shell: Part 2](http://industrialdigitaltwin.org/en/content-hub).   Publisher: Industrial Digital Twin Association (IDTA) 2023
 *
 * API version: V3.0.3_SSP-001
 * Contact: info@idtwin.org
 */

package model

// AdministrativeInformation struct represents administrative metadata for an Asset Administration Shell (AAS).
type AdministrativeInformation struct {
	EmbeddedDataSpecifications []EmbeddedDataSpecification `json:"embeddedDataSpecifications,omitempty"`

	Version string `json:"version,omitempty" validate:"regexp=^([0-9]|[1-9][0-9]*)$"`

	Revision string `json:"revision,omitempty" validate:"regexp=^([0-9]|[1-9][0-9]*)$"`

	Creator *Reference `json:"creator,omitempty"`

	TemplateID string `json:"templateId,omitempty" validate:"regexp=^([\\\\x09\\\\x0a\\\\x0d\\\\x20-\\\\ud7ff\\\\ue000-\\\\ufffd]|\\\\ud800[\\\\udc00-\\\\udfff]|[\\\\ud801-\\\\udbfe][\\\\udc00-\\\\udfff]|\\\\udbff[\\\\udc00-\\\\udfff])*$"`
}

// AssertAdministrativeInformationRequired checks if the required fields are not zero-ed
func AssertAdministrativeInformationRequired(obj AdministrativeInformation) error {
	for _, el := range obj.EmbeddedDataSpecifications {
		if err := AssertEmbeddedDataSpecificationRequired(el); err != nil {
			return err
		}
	}
	if obj.Creator != nil {
		if err := AssertReferenceRequired(*obj.Creator); err != nil {
			return err
		}
	}
	return nil
}

// AssertAdministrativeInformationConstraints checks if the values respects the defined constraints
func AssertAdministrativeInformationConstraints(obj AdministrativeInformation) error {
	for _, el := range obj.EmbeddedDataSpecifications {
		if err := AssertEmbeddedDataSpecificationConstraints(el); err != nil {
			return err
		}
	}
	if obj.Creator != nil {
		if err := AssertReferenceConstraints(*obj.Creator); err != nil {
			return err
		}
	}
	return nil
}
