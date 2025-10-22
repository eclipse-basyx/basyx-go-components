package auth

import (
	"fmt"
	"os"

	"github.com/xeipuuv/gojsonschema"
)

func ValidatePolicyWithSchema(schemaPath, policyPath string) error {
	schemaBytes, err := os.ReadFile(schemaPath)
	if err != nil {
		return fmt.Errorf("read schema: %w", err)
	}
	policyBytes, err := os.ReadFile(policyPath)
	if err != nil {
		return fmt.Errorf("read policy: %w", err)
	}

	schemaLoader := gojsonschema.NewBytesLoader(schemaBytes)
	docLoader := gojsonschema.NewBytesLoader(policyBytes)

	// Compile the schema first (more robust than Validate with two loaders)
	sl := gojsonschema.NewSchemaLoader()
	compiled, err := sl.Compile(schemaLoader)
	if err != nil {
		return fmt.Errorf("compile schema: %w", err)
	}

	result, err := compiled.Validate(docLoader)
	if err != nil {
		return fmt.Errorf("schema validation error: %w", err)
	}
	if !result.Valid() {
		errStr := "policy invalid:\n"
		for _, e := range result.Errors() {
			errStr += "- " + e.String() + "\n"
		}
		return fmt.Errorf(errStr)
	}
	return nil
}
