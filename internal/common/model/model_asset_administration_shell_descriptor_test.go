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
	"encoding/json"
	"strings"
	"testing"

	"github.com/FriedJannik/aas-go-sdk/types"
)

func TestAssertAssetAdministrationShellDescriptorConstraints_RejectsNullByteAssetType(t *testing.T) {
	obj := AssetAdministrationShellDescriptor{
		Id:        "375c1f38-0ada-4fe3-8614-6eef35e5cf3f",
		AssetType: "AssetType \u0000",
	}

	err := AssertAssetAdministrationShellDescriptorConstraints(obj)
	if err == nil {
		t.Fatal("expected constraint validation error for assetType with null byte")
	}
	if err.Error() != `must match "^([\\x09\\x0a\\x0d\\x20-\\ud7ff\\ue000-\\ufffd]|\\ud800[\\udc00-\\udfff]|[\\ud801-\\udbfe][\\udc00-\\udfff]|\\udbff[\\udc00-\\udfff])*$"` {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestAssertAssetAdministrationShellDescriptorConstraints_RejectsEmptySpecificAssetIDReferenceKeys(t *testing.T) {
	specificAssetID := types.NewSpecificAssetID("manufacturerId", "value")
	specificAssetID.SetSemanticID(types.NewReference(types.ReferenceTypesModelReference, []types.IKey{}))

	obj := AssetAdministrationShellDescriptor{
		Id:               "375c1f38-0ada-4fe3-8614-6eef35e5cf3f",
		SpecificAssetIds: []types.ISpecificAssetID{specificAssetID},
	}

	err := AssertAssetAdministrationShellDescriptorConstraints(obj)
	if err == nil {
		t.Fatal("expected constraint validation error for empty semanticId keys")
	}
	if !strings.Contains(err.Error(), "COMMON-REFCONSTRAINTS-EMPTYKEYS") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestAssetAdministrationShellDescriptorUnmarshalPermissiveStillRejectsEmptySpecificAssetIDReferenceKeysInConstraints(t *testing.T) {
	setVerificationMode(t, "permissive")
	t.Cleanup(func() {
		setVerificationMode(t, "off")
	})

	payload := `{
		"id": "375c1f38-0ada-4fe3-8614-6eef35e5cf3f",
		"specificAssetIds": [{
			"name": "manufacturerId",
			"value": "value",
			"semanticId": {"type": "ModelReference", "keys": []}
		}]
	}`

	var descriptor AssetAdministrationShellDescriptor
	if err := json.Unmarshal([]byte(payload), &descriptor); err != nil {
		t.Fatalf("unmarshal failed: %v", err)
	}

	err := AssertAssetAdministrationShellDescriptorConstraints(descriptor)
	if err == nil {
		t.Fatal("expected constraint validation error for empty semanticId keys")
	}
	if !strings.Contains(err.Error(), "specificAssetIds.semanticId.keys must not be empty") {
		t.Fatalf("unexpected error: %v", err)
	}
}
