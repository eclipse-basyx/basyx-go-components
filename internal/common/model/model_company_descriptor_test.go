/*******************************************************************************
* Copyright (C) 2026 the Eclipse BaSyx Authors and Fraunhofer IESE
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

package model

import (
	"strings"
	"testing"

	"github.com/FriedJannik/aas-go-sdk/types"
)

func TestAssertCompanyDescriptorConstraints_RejectsEmptyCreatorReferenceKeys(t *testing.T) {
	descriptor := validCompanyDescriptorForConstraints()
	descriptor.Administration.SetCreator(types.NewReference(types.ReferenceTypesExternalReference, []types.IKey{}))

	err := AssertCompanyDescriptorConstraints(descriptor)
	if err == nil {
		t.Fatal("expected constraint validation error for empty creator reference keys")
	}
	if !strings.Contains(err.Error(), "administration.creator.keys must not be empty") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestAssertCompanyDescriptorConstraints_RejectsEmptyCreatorKeyValue(t *testing.T) {
	descriptor := validCompanyDescriptorForConstraints()
	descriptor.Administration.SetCreator(types.NewReference(
		types.ReferenceTypesExternalReference,
		[]types.IKey{types.NewKey(types.KeyTypesGlobalReference, " ")},
	))

	err := AssertCompanyDescriptorConstraints(descriptor)
	if err == nil {
		t.Fatal("expected constraint validation error for empty creator key value")
	}
	if !strings.Contains(err.Error(), "administration.creator.keys[0].value must not be empty") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestAssertCompanyDescriptorConstraints_RejectsEmptyDescriptionLanguage(t *testing.T) {
	descriptor := validCompanyDescriptorForConstraints()
	descriptor.Description = []types.ILangStringTextType{
		types.NewLangStringTextType("", "A company description"),
	}

	err := AssertCompanyDescriptorConstraints(descriptor)
	if err == nil {
		t.Fatal("expected constraint validation error for empty description language")
	}
	if !strings.Contains(err.Error(), "description[0].language must not be empty") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestAssertCompanyDescriptorConstraints_RejectsEmptyDisplayNameText(t *testing.T) {
	descriptor := validCompanyDescriptorForConstraints()
	descriptor.DisplayName = []types.ILangStringNameType{
		types.NewLangStringNameType("en", " "),
	}

	err := AssertCompanyDescriptorConstraints(descriptor)
	if err == nil {
		t.Fatal("expected constraint validation error for empty display name text")
	}
	if !strings.Contains(err.Error(), "displayName[0].text must not be empty") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func validCompanyDescriptorForConstraints() CompanyDescriptor {
	administration := types.NewAdministrativeInformation()
	administration.SetCreator(types.NewReference(
		types.ReferenceTypesExternalReference,
		[]types.IKey{types.NewKey(types.KeyTypesGlobalReference, "https://iese.fraunhofer.de")},
	))

	return CompanyDescriptor{
		Name:   "Fraunhofer IESE",
		Domain: "iese.fraunhofer.de",
		Endpoints: []Endpoint{
			{
				Interface: "AAS-REGISTRY-3.0",
				ProtocolInformation: ProtocolInformation{
					Href:             "https://demo.digital-twin.host/aas-registry",
					EndpointProtocol: "HTTPS",
				},
			},
		},
		Administration: administration,
		Description: []types.ILangStringTextType{
			types.NewLangStringTextType("en", "A company description"),
		},
		DisplayName: []types.ILangStringNameType{
			types.NewLangStringNameType("en", "Fraunhofer IESE"),
		},
	}
}
