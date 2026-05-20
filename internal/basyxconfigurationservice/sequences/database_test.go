package steps

import (
	"strings"
	"testing"
)

func TestDatabaseConnectionGetDescription(t *testing.T) {
	step := NewDatabaseConnection(&ExecutionContext{}, "")
	description := step.GetDescription(3)
	if description != "[Step 3] Connecting to Database" {
		t.Fatalf("unexpected description: %q", description)
	}
}

func TestDatabaseConnectionExecuteReturnsConfigLoadError(t *testing.T) {
	step := NewDatabaseConnection(&ExecutionContext{}, "/path/that/does/not/exist.yaml")
	statusCode, err := step.Execute(1)
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if statusCode != 1 {
		t.Fatalf("expected status code 1, got %d", statusCode)
	}
	if !strings.Contains(err.Error(), "BASYXCFG-DB-LOADCONFIG") {
		t.Fatalf("unexpected error: %v", err)
	}
}
