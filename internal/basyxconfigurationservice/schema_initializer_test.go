package basyxconfigurationservice

import (
	"errors"
	"strings"
	"testing"
)

type mockStep struct {
	description string
	execFn      func(stepIndex int) (int, error)
}

func (m mockStep) Execute(stepIndex int) (int, error) {
	return m.execFn(stepIndex)
}

func (m mockStep) GetDescription(_ int) string {
	return m.description
}

func TestStepRegistryExecuteRunsStepsInOrder(t *testing.T) {
	registry := NewSchemaInitializer()
	seenIndices := make([]int, 0, 2)

	registry.Register(mockStep{
		description: "step-1",
		execFn: func(stepIndex int) (int, error) {
			seenIndices = append(seenIndices, stepIndex)
			return 0, nil
		},
	})
	registry.Register(mockStep{
		description: "step-2",
		execFn: func(stepIndex int) (int, error) {
			seenIndices = append(seenIndices, stepIndex)
			return 0, nil
		},
	})

	if err := registry.Execute(); err != nil {
		t.Fatalf("execute returned error: %v", err)
	}

	if len(seenIndices) != 2 {
		t.Fatalf("expected 2 executed steps, got %d", len(seenIndices))
	}
	if seenIndices[0] != 1 || seenIndices[1] != 2 {
		t.Fatalf("unexpected step indices: %#v", seenIndices)
	}
}

func TestStepRegistryExecuteReturnsWrappedError(t *testing.T) {
	registry := NewSchemaInitializer()
	rootErr := errors.New("boom")

	registry.Register(mockStep{
		description: "failing-step",
		execFn: func(_ int) (int, error) {
			return 17, rootErr
		},
	})

	err := registry.Execute()
	if err == nil {
		t.Fatal("expected error, got nil")
	}

	errMsg := err.Error()
	if !strings.Contains(errMsg, "BASYXCFG-REGISTRY-EXECSTEP") {
		t.Fatalf("expected wrapped error code, got: %s", errMsg)
	}
	if !strings.Contains(errMsg, "status 17") {
		t.Fatalf("expected status code in error, got: %s", errMsg)
	}
	if !strings.Contains(errMsg, "boom") {
		t.Fatalf("expected root cause in error, got: %s", errMsg)
	}
}
