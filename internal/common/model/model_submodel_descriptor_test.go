package model

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/aas-core-works/aas-core3.1-golang/jsonization"
	"github.com/aas-core-works/aas-core3.1-golang/types"
)

func setVerificationMode(t *testing.T, mode string) {
	t.Helper()
	if err := SetVerificationMode(mode); err != nil {
		t.Fatalf("failed to set verification mode %q: %v", mode, err)
	}
}

func TestSubmodelDescriptorUnmarshalRejectsSingularWhenDisabled(t *testing.T) {
	setVerificationMode(t, "off")
	SetSupportsSingularSupplementalSemanticId(false)

	payload := `{
		"endpoints":[{"interface":"IF","protocolInformation":{"href":"http://example.com"}}],
		"id":"submodel-id",
		"supplementalSemanticId":[{"type":"ModelReference","keys":[{"type":"Submodel","value":"x"}]}]
	}`

	var descriptor SubmodelDescriptor
	err := json.Unmarshal([]byte(payload), &descriptor)
	if err == nil {
		t.Fatal("expected error for singular supplementalSemanticId when support is disabled")
	}
	if !strings.Contains(err.Error(), "unknown field: supplementalSemanticId") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestSubmodelDescriptorUnmarshalAcceptsSingularWhenEnabled(t *testing.T) {
	setVerificationMode(t, "off")
	SetSupportsSingularSupplementalSemanticId(true)
	t.Cleanup(func() {
		SetSupportsSingularSupplementalSemanticId(false)
	})

	payload := `{
		"endpoints":[{"interface":"IF","protocolInformation":{"href":"http://example.com"}}],
		"id":"submodel-id",
		"supplementalSemanticId":[{"type":"ModelReference","keys":[{"type":"Submodel","value":"x"}]}]
	}`

	var descriptor SubmodelDescriptor
	if err := json.Unmarshal([]byte(payload), &descriptor); err != nil {
		t.Fatalf("unmarshal failed: %v", err)
	}

	if len(descriptor.SupplementalSemanticId) != 1 {
		t.Fatalf("expected 1 supplemental semantic id, got %d", len(descriptor.SupplementalSemanticId))
	}
}

func TestSubmodelDescriptorToJsonableUsesPluralWhenDisabled(t *testing.T) {
	setVerificationMode(t, "off")
	SetSupportsSingularSupplementalSemanticId(false)

	descriptor := SubmodelDescriptor{
		SupplementalSemanticId: []types.IReference{mustReference(t)},
	}

	jsonable, err := descriptor.ToJsonable()
	if err != nil {
		t.Fatalf("ToJsonable failed: %v", err)
	}

	if _, ok := jsonable[supplementalSemanticIdsKey]; !ok {
		t.Fatalf("expected key %q in output", supplementalSemanticIdsKey)
	}
	if _, ok := jsonable[supplementalSemanticIdSingularKey]; ok {
		t.Fatalf("did not expect key %q in output", supplementalSemanticIdSingularKey)
	}
}

func TestSubmodelDescriptorToJsonableUsesSingularWhenEnabled(t *testing.T) {
	setVerificationMode(t, "off")
	SetSupportsSingularSupplementalSemanticId(true)
	t.Cleanup(func() {
		SetSupportsSingularSupplementalSemanticId(false)
	})

	descriptor := SubmodelDescriptor{
		SupplementalSemanticId: []types.IReference{mustReference(t)},
	}

	jsonable, err := descriptor.ToJsonable()
	if err != nil {
		t.Fatalf("ToJsonable failed: %v", err)
	}

	if _, ok := jsonable[supplementalSemanticIdSingularKey]; !ok {
		t.Fatalf("expected key %q in output", supplementalSemanticIdSingularKey)
	}
	if _, ok := jsonable[supplementalSemanticIdsKey]; ok {
		t.Fatalf("did not expect key %q in output", supplementalSemanticIdsKey)
	}
}

func TestSubmodelDescriptorUnmarshalSkipsSemanticVerificationWhenStrictDisabled(t *testing.T) {
	SetSupportsSingularSupplementalSemanticId(false)
	setVerificationMode(t, "off")
	t.Cleanup(func() {
		setVerificationMode(t, "off")
	})

	payload := `{
		"endpoints":[{"interface":"IF","protocolInformation":{"href":"http://example.com"}}],
		"id":"submodel-id",
		"semanticId":{"type":"ExternalReference","keys":[{"type":"Submodel","value":"semanticIdExample"}]}
	}`

	var descriptor SubmodelDescriptor
	if err := json.Unmarshal([]byte(payload), &descriptor); err != nil {
		t.Fatalf("expected successful unmarshal with strictVerification disabled, got: %v", err)
	}
}

func TestSubmodelDescriptorUnmarshalSkipsSemanticVerificationWhenPermissive(t *testing.T) {
	SetSupportsSingularSupplementalSemanticId(false)
	setVerificationMode(t, "permissive")
	t.Cleanup(func() {
		setVerificationMode(t, "off")
	})

	payload := `{
		"endpoints":[{"interface":"IF","protocolInformation":{"href":"http://example.com"}}],
		"id":"submodel-id",
		"semanticId":{"type":"ExternalReference","keys":[{"type":"Submodel","value":"semanticIdExample"}]}
	}`

	var descriptor SubmodelDescriptor
	if err := json.Unmarshal([]byte(payload), &descriptor); err != nil {
		t.Fatalf("expected successful unmarshal with permissive verification, got: %v", err)
	}
}

func TestSubmodelDescriptorUnmarshalFailsSemanticVerificationWhenStrictEnabled(t *testing.T) {
	SetSupportsSingularSupplementalSemanticId(false)
	setVerificationMode(t, "strict")
	t.Cleanup(func() {
		setVerificationMode(t, "off")
	})

	payload := `{
		"endpoints":[{"interface":"IF","protocolInformation":{"href":"http://example.com"}}],
		"id":"submodel-id",
		"semanticId":{"type":"ExternalReference","keys":[{"type":"Submodel","value":"semanticIdExample"}]}
	}`

	var descriptor SubmodelDescriptor
	err := json.Unmarshal([]byte(payload), &descriptor)
	if err == nil {
		t.Fatal("expected semanticId verification failure with strictVerification enabled")
	}
	if !strings.Contains(err.Error(), "SemanticId verification failed") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestAssertSubmodelDescriptorConstraints_RejectsEmptySemanticIDReferenceKeys(t *testing.T) {
	descriptor := SubmodelDescriptor{
		Id:         "submodel-id",
		SemanticId: types.NewReference(types.ReferenceTypesModelReference, []types.IKey{}),
	}

	err := AssertSubmodelDescriptorConstraints(descriptor)
	if err == nil {
		t.Fatal("expected constraint validation error for empty semanticId keys")
	}
	if !strings.Contains(err.Error(), "semanticId.keys must not be empty") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func mustReference(t *testing.T) types.IReference {
	t.Helper()
	ref, err := jsonization.ReferenceFromJsonable(map[string]any{
		"type": "ModelReference",
		"keys": []any{
			map[string]any{
				"type":  "Submodel",
				"value": "x",
			},
		},
	})
	if err != nil {
		t.Fatalf("failed to build reference: %v", err)
	}
	return ref
}
