package model

import (
	"strings"
	"testing"

	"github.com/aas-core-works/aas-core3.1-golang/types"
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
