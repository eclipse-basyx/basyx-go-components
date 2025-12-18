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
//nolint:all
package model

import "reflect"

func isReferenceEmpty(ref Reference) bool {
	return reflect.DeepEqual(ref, Reference{})
}

// AssertSubmodelElementRequiredFixed is a fixed version of AssertSubmodelElementRequired
// that doesn't validate empty optional references
func AssertSubmodelElementRequiredFixed(obj SubmodelElement) error {
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
	// Only validate non-empty references
	if obj.GetSemanticID() != nil && !isReferenceEmpty(*obj.GetSemanticID()) {
		if err := AssertReferenceRequired(*obj.GetSemanticID()); err != nil {
			return err
		}
	}
	for _, el := range obj.GetSupplementalSemanticIds() {
		if !isReferenceEmpty(el) {
			if err := AssertReferenceRequired(el); err != nil {
				return err
			}
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

// AssertSubmodelElementConstraintsFixed is a fixed version of AssertSubmodelElementConstraints
// that doesn't validate empty optional references
func AssertSubmodelElementConstraintsFixed(obj SubmodelElement) error {
	for _, el := range obj.GetExtensions() {
		if err := AssertExtensionConstraints(el); err != nil {
			return err
		}
	}
	if err := AssertStringConstraints(obj.GetIdShort()); err != nil {
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
	// Only validate constraints on non-empty references
	if obj.GetSemanticID() != nil && !isReferenceEmpty(*obj.GetSemanticID()) {
		if err := AssertReferenceConstraints(*obj.GetSemanticID()); err != nil {
			return err
		}
	}
	for _, el := range obj.GetSupplementalSemanticIds() {
		if !isReferenceEmpty(el) {
			if err := AssertReferenceConstraints(el); err != nil {
				return err
			}
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
