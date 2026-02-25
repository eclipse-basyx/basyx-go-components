package securitydocu

import (
	"embed"
	"io/fs"
)

//go:embed openapi_rules_management.yaml
var openAPIAssets embed.FS

// OpenAPIRulesManagementYAML returns the shared OpenAPI fragment for the ABAC rules management endpoints.
func OpenAPIRulesManagementYAML() ([]byte, error) {
	return fs.ReadFile(openAPIAssets, "openapi_rules_management.yaml")
}
