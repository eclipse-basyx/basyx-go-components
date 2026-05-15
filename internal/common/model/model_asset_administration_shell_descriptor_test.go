package model

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/aas-core-works/aas-core3.1-golang/types"
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
