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
// Author: Aaron Zielstorff ( Fraunhofer IESE )

package dppapi

import (
	"testing"
	"time"

	"github.com/FriedJannik/aas-go-sdk/types"
	"github.com/FriedJannik/aas-go-sdk/verification"
)

func TestBuildMetadataSubmodelConformsToIDTA02099(t *testing.T) {
	lastUpdate := time.Date(2026, time.July, 9, 10, 11, 12, 123000000, time.UTC)
	//nolint:gosec // The values describe a standards-conformance fixture, not credentials.
	header := dppHeader{
		DigitalProductPassportID: "https://example.org/dpps/1",
		UniqueProductIdentifier:  "https://example.org/products/1",
		Granularity:              "Item",
		DppSchemaVersion:         "EN18223:v1.0",
		DppStatus:                "Active",
		LastUpdate:               lastUpdate,
		EconomicOperatorID:       "operator-1",
		FacilityID:               "facility-1",
		ContentSpecificationIDs:  []string{"urn:example:specification:one", "urn:example:specification:two"},
	}

	metadata := buildMetadataSubmodel(header.DigitalProductPassportID, header)
	expectedProperties := map[string]struct {
		semanticID string
		valueType  types.DataTypeDefXSD
	}{
		headerDigitalProductPassportID: {dppIDSemanticID, types.DataTypeDefXSDString},
		headerUniqueProductIdentifier:  {dppProductIDSemanticID, types.DataTypeDefXSDString},
		headerGranularity:              {dppGranularitySemanticID, types.DataTypeDefXSDString},
		headerDppSchemaVersion:         {dppSchemaVersionSemanticID, types.DataTypeDefXSDString},
		headerDppStatus:                {dppStatusSemanticID, types.DataTypeDefXSDString},
		headerLastUpdate:               {dppLastUpdateSemanticID, types.DataTypeDefXSDDateTime},
		headerEconomicOperatorID:       {dppEconomicOperatorIDSemanticID, types.DataTypeDefXSDString},
		headerFacilityID:               {dppFacilityIDSemanticID, types.DataTypeDefXSDString},
	}
	for idShort, expected := range expectedProperties {
		element := metadataElementByIDShort(t, metadata, idShort)
		property, ok := element.(*types.Property)
		if !ok {
			t.Fatalf("metadata element %s has type %T, want Property", idShort, element)
		}
		if got := referenceToString(property.SemanticID()); got != expected.semanticID {
			t.Fatalf("metadata element %s semantic ID = %q, want %q", idShort, got, expected.semanticID)
		}
		if property.ValueType() != expected.valueType {
			t.Fatalf("metadata element %s value type = %v, want %v", idShort, property.ValueType(), expected.valueType)
		}
	}

	assertSupplementalSemanticID(t, metadataElementByIDShort(t, metadata, headerGranularity), dppGranularitySupplementalSemanticID)
	assertSupplementalSemanticID(t, metadataElementByIDShort(t, metadata, headerLastUpdate), dppAdministrativeUpdateSupplementalSemanticID)
	assertMetadataContentSpecificationIDs(t, metadata)
	assertSubmodelTimestamp(t, metadata, lastUpdate)
	assertValidSubmodel(t, metadata)
}

func assertMetadataContentSpecificationIDs(t *testing.T, metadata types.ISubmodel) {
	t.Helper()
	element := metadataElementByIDShort(t, metadata, headerContentSpecificationIDs)
	list, ok := element.(*types.SubmodelElementList)
	if !ok {
		t.Fatalf("metadata element %s has type %T, want SubmodelElementList", headerContentSpecificationIDs, element)
	}
	if got := referenceToString(list.SemanticID()); got != dppContentSpecificationIDsSemanticID {
		t.Fatalf("contentSpecificationIds semantic ID = %q", got)
	}
	if list.OrderRelevant() == nil || *list.OrderRelevant() {
		t.Fatalf("contentSpecificationIds orderRelevant = %#v, want false", list.OrderRelevant())
	}
	if list.TypeValueListElement() != types.AASSubmodelElementsProperty {
		t.Fatalf("contentSpecificationIds element type = %v", list.TypeValueListElement())
	}
	if list.ValueTypeListElement() == nil || *list.ValueTypeListElement() != types.DataTypeDefXSDString {
		t.Fatalf("contentSpecificationIds value type = %#v", list.ValueTypeListElement())
	}
	if got := referenceToString(list.SemanticIDListElement()); got != dppContentSpecificationIDSemanticID {
		t.Fatalf("contentSpecificationIds list element semantic ID = %q", got)
	}
	for index, item := range list.Value() {
		if got := referenceToString(item.SemanticID()); got != dppContentSpecificationIDSemanticID {
			t.Fatalf("contentSpecificationIds item %d semantic ID = %q", index, got)
		}
	}
}

func metadataElementByIDShort(t *testing.T, metadata types.ISubmodel, idShort string) types.ISubmodelElement {
	t.Helper()
	for _, element := range metadata.SubmodelElements() {
		if element.IDShort() != nil && *element.IDShort() == idShort {
			return element
		}
	}
	t.Fatalf("metadata element %s not found", idShort)
	return nil
}

func assertSupplementalSemanticID(t *testing.T, element types.ISubmodelElement, expected string) {
	t.Helper()
	if len(element.SupplementalSemanticIDs()) != 1 || referenceToString(element.SupplementalSemanticIDs()[0]) != expected {
		t.Fatalf("element supplemental semantic IDs = %#v, want %q", element.SupplementalSemanticIDs(), expected)
	}
}

func assertSubmodelTimestamp(t *testing.T, submodel types.ISubmodel, expected time.Time) {
	t.Helper()
	if submodel.Administration() == nil || submodel.Administration().CreatedAt() == nil || submodel.Administration().UpdatedAt() == nil {
		t.Fatalf("submodel administration = %#v, want createdAt and updatedAt", submodel.Administration())
	}
	want := expected.UTC().Format(time.RFC3339Nano)
	if *submodel.Administration().CreatedAt() != want || *submodel.Administration().UpdatedAt() != want {
		t.Fatalf("submodel timestamps = (%q, %q), want %q", *submodel.Administration().CreatedAt(), *submodel.Administration().UpdatedAt(), want)
	}
}

func assertValidSubmodel(t *testing.T, submodel types.ISubmodel) {
	t.Helper()
	verificationErrors := make([]string, 0)
	verification.VerifySubmodel(submodel, func(err *verification.VerificationError) bool {
		verificationErrors = append(verificationErrors, err.Error())
		return false
	})
	if len(verificationErrors) != 0 {
		t.Fatalf("VerifySubmodel() errors = %#v", verificationErrors)
	}
}
