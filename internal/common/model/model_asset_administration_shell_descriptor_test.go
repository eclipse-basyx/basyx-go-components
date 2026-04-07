package model

import (
	"testing"
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
