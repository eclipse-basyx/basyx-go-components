/*******************************************************************************
* Copyright (C) 2026 the Eclipse BaSyx Authors and Fraunhofer IESE
*
* SPDX-License-Identifier: MIT
******************************************************************************/

package aasregistryapi

import (
	"net/http"
	"testing"

	"github.com/eclipse-basyx/basyx-go-components/internal/common/model"
)

func TestValidateBulkCreateDescriptorsRejectsDuplicateID(t *testing.T) {
	t.Parallel()

	failure := validateBulkCreateDescriptors([]model.AssetAdministrationShellDescriptor{
		{Id: "urn:example:aas:1"},
		{Id: "urn:example:aas:1"},
	})

	if failure.Index != 1 {
		t.Fatalf("expected duplicate index 1, got %d", failure.Index)
	}
	if failure.StatusCode != http.StatusConflict {
		t.Fatalf("expected status %d, got %d", http.StatusConflict, failure.StatusCode)
	}
}

func TestValidateBulkCreateDescriptorsRejectsMissingID(t *testing.T) {
	t.Parallel()

	failure := validateBulkCreateDescriptors([]model.AssetAdministrationShellDescriptor{
		{Id: "urn:example:aas:1"},
		{Id: "  "},
	})

	if failure.Index != 1 {
		t.Fatalf("expected missing-id index 1, got %d", failure.Index)
	}
	if failure.StatusCode != http.StatusBadRequest {
		t.Fatalf("expected status %d, got %d", http.StatusBadRequest, failure.StatusCode)
	}
}

func TestValidateBulkCreateDescriptorsNormalizesIDs(t *testing.T) {
	t.Parallel()

	descriptors := []model.AssetAdministrationShellDescriptor{
		{Id: " urn:example:aas:1 "},
		{Id: "\turn:example:aas:2\n"},
	}

	failure := validateBulkCreateDescriptors(descriptors)
	if failure.StatusCode != 0 {
		t.Fatalf("expected validation success, got failure: %+v", failure)
	}
	if descriptors[0].Id != "urn:example:aas:1" {
		t.Fatalf("expected first id to be normalized, got %q", descriptors[0].Id)
	}
	if descriptors[1].Id != "urn:example:aas:2" {
		t.Fatalf("expected second id to be normalized, got %q", descriptors[1].Id)
	}
}
