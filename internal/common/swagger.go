// Package common provides shared utilities for BaSyx Go components.
package common

import (
	"bytes"
	"embed"
	"fmt"
	"html/template"
	"io/fs"
	"log"
	"net/http"
	"path"
	"regexp"
	"strings"

	"github.com/go-chi/chi/v5"
)

// SwaggerUIHTML is the HTML template for Swagger UI
const SwaggerUIHTML = `<!DOCTYPE html>
<html lang="en">
<head>
    <meta charset="UTF-8">
    <meta name="viewport" content="width=device-width, initial-scale=1.0">
    <title>{{.Title}} - Swagger UI</title>
    <link rel="stylesheet" type="text/css" href="https://unpkg.com/swagger-ui-dist@5/swagger-ui.css">
    <style>
        html { box-sizing: border-box; overflow-y: scroll; }
        *, *:before, *:after { box-sizing: inherit; }
        body { margin: 0; background: #fafafa; }
    </style>
</head>
<body>
    <div id="swagger-ui"></div>
    <script src="https://unpkg.com/swagger-ui-dist@5/swagger-ui-bundle.js"></script>
    <script src="https://unpkg.com/swagger-ui-dist@5/swagger-ui-standalone-preset.js"></script>
    <script>
        window.onload = function() {
            window.ui = SwaggerUIBundle({
                url: "{{.SpecURL}}",
                dom_id: '#swagger-ui',
                deepLinking: true,
                presets: [
                    SwaggerUIBundle.presets.apis,
                    SwaggerUIStandalonePreset
                ],
                plugins: [
                    SwaggerUIBundle.plugins.DownloadUrl
                ],
                layout: "StandaloneLayout"
            });
        };
    </script>
</body>
</html>`

//go:embed swagger_part2_schemas/V3.1.1/openapi.yaml swagger_part2_schemas/V3.2.0/openapi.yaml
var part2SchemasFS embed.FS

// SwaggerUIConfig holds configuration for Swagger UI endpoint setup
type SwaggerUIConfig struct {
	Title                 string         // Title shown in browser tab
	SpecURL               string         // URL to the OpenAPI spec (e.g., "/api-docs/openapi.yaml")
	UIPath                string         // Path where Swagger UI will be served (e.g., "/swagger")
	SpecPath              string         // Path where spec will be served (e.g., "/api-docs/openapi.yaml")
	SpecContent           []byte         // The OpenAPI spec content
	ServerURL             string         // Server URL to use in OpenAPI spec (e.g., "http://localhost:5004/api")
	BasePath              string         // Base path for redirect to Swagger UI (e.g., "/" or "/api")
	Contact               *ContactConfig // Contact information to inject into OpenAPI spec
	IncludeVerifyEndpoint *bool          // nil/default=true, false disables /verify injection in OpenAPI spec
	IncludeABACManagement *bool          // nil/default=false, true injects ABAC management API paths
}

// ContactConfig holds contact information for OpenAPI spec
type ContactConfig struct {
	Name  string // Contact name
	Email string // Contact email
	URL   string // Contact URL
}

var openAPIVersionRegex = regexp.MustCompile(`(?m)^\s*version:\s*V([0-9]+\.[0-9]+\.[0-9]+)`)
var verifyPathRegex = regexp.MustCompile(`(?m)^\s*/verify:\s*$`)
var abacManagementPathRegex = regexp.MustCompile(`(?m)^\s*/security/abac/policy-versions:\s*$`)
var pathsSectionRegex = regexp.MustCompile(`(?m)^paths:\s*(?:\r?\n)`)

func detectPart2SchemaVersion(specContent []byte) string {
	matches := openAPIVersionRegex.FindSubmatch(specContent)
	if len(matches) < 2 {
		return "V3.1.1"
	}
	return "V" + string(matches[1])
}

func localizePart2SchemaReferences(specContent []byte, specPath string) []byte {
	version := detectPart2SchemaVersion(specContent)
	localSchemaURL := path.Clean(path.Dir(specPath) + "/part2-schemas/" + version + "/openapi.yaml")

	rewritten := specContent
	remotePrefixes := []string{
		"https://api.swaggerhub.com/domains/Plattform_i40/Part2-API-Schemas/V3.1.1",
		"https://api.swaggerhub.com/domains/Plattform_i40/Part2-API-Schemas/V3.2.0",
	}
	for _, prefix := range remotePrefixes {
		rewritten = []byte(strings.ReplaceAll(string(rewritten), prefix, localSchemaURL))
	}

	relativePrefixes := []string{
		"../Part2-API-Schemas/openapi.yaml",
		"../Part2-API-Schemas/V3.1.1/openapi.yaml",
		"../Part2-API-Schemas/V3.2.0/openapi.yaml",
	}
	for _, prefix := range relativePrefixes {
		rewritten = []byte(strings.ReplaceAll(string(rewritten), prefix, localSchemaURL))
	}

	return rewritten
}

func injectVerifyEndpoint(specContent []byte) []byte {
	if verifyPathRegex.Match(specContent) {
		return specContent
	}

	verifyPath := "" +
		"  /verify:\n" +
		"    post:\n" +
		"      tags:\n" +
		"        - Verification API\n" +
		"      summary: Verifies AAS payload against the AAS meta model\n" +
		"      operationId: VerifyPayload\n" +
		"      requestBody:\n" +
		"        required: true\n" +
		"        content:\n" +
		"          application/json:\n" +
		"            schema:\n" +
		"              type: object\n" +
		"          application/xml:\n" +
		"            schema:\n" +
		"              type: string\n" +
		"          application/aasx+xml:\n" +
		"            schema:\n" +
		"              type: string\n" +
		"              format: binary\n" +
		"          application/aasx+json:\n" +
		"            schema:\n" +
		"              type: string\n" +
		"              format: binary\n" +
		"          multipart/form-data:\n" +
		"            schema:\n" +
		"              type: object\n" +
		"              oneOf:\n" +
		"                - required:\n" +
		"                    - file\n" +
		"                - required:\n" +
		"                    - payload\n" +
		"              properties:\n" +
		"                file:\n" +
		"                  type: string\n" +
		"                  format: binary\n" +
		"                payload:\n" +
		"                  type: string\n" +
		"      responses:\n" +
		"        '200':\n" +
		"          description: Verification result\n" +
		"          content:\n" +
		"            application/json:\n" +
		"              schema:\n" +
		"                type: object\n" +
		"                properties:\n" +
		"                  valid:\n" +
		"                    type: boolean\n" +
		"                  format:\n" +
		"                    type: string\n" +
		"                  assetAdministrationShellCount:\n" +
		"                    type: integer\n" +
		"                  submodelCount:\n" +
		"                    type: integer\n" +
		"                  conceptDescriptionCount:\n" +
		"                    type: integer\n" +
		"                  messages:\n" +
		"                    type: array\n" +
		"                    items:\n" +
		"                      type: string\n" +
		"        '400':\n" +
		"          description: Invalid payload or unsupported format\n" +
		"        '413':\n" +
		"          description: Payload exceeds configured size limit\n" +
		"        '500':\n" +
		"          description: Internal server error while generating response\n"

	pathLoc := pathsSectionRegex.FindIndex(specContent)
	if pathLoc != nil {
		injected := make([]byte, 0, len(specContent)+len(verifyPath))
		injected = append(injected, specContent[:pathLoc[1]]...)
		injected = append(injected, verifyPath...)
		injected = append(injected, specContent[pathLoc[1]:]...)
		return injected
	}

	appended := make([]byte, 0, len(specContent)+len(verifyPath)+8)
	appended = append(appended, specContent...)
	if len(specContent) > 0 && specContent[len(specContent)-1] != '\n' {
		appended = append(appended, '\n')
	}
	appended = append(appended, []byte("paths:\n")...)
	appended = append(appended, verifyPath...)
	return appended
}

const abacManagementPathsYAML = `  /security/abac/active-policy:
    get:
      tags:
        - ABAC Policy Management
      summary: Gets the active ABAC policy version
      operationId: GetActiveABACPolicyVersion
      responses:
        '200':
          description: Active ABAC policy version
          content:
            application/json:
              schema:
                $ref: '#/components/schemas/ABACPolicyVersion'
        '404':
          description: Hidden when the caller is not allowed to inspect ABAC policy management data
        '503':
          description: No active ABAC policy is available
  /security/abac/active-policy/rules:
    get:
      tags:
        - ABAC Policy Management
      summary: Lists materialized rules for the active ABAC policy version
      operationId: ListActiveABACPolicyRules
      responses:
        '200':
          description: Active ABAC policy rules
          content:
            application/json:
              schema:
                type: array
                items:
                  $ref: '#/components/schemas/ABACPolicyRule'
        '404':
          description: Hidden when the caller is not allowed to inspect ABAC policy management data
        '503':
          description: No active ABAC policy is available
  /security/abac/policy-versions:
    get:
      tags:
        - ABAC Policy Management
      summary: Lists ABAC policy versions
      operationId: ListABACPolicyVersions
      responses:
        '200':
          description: ABAC policy versions
          content:
            application/json:
              schema:
                type: array
                items:
                  $ref: '#/components/schemas/ABACPolicyVersion'
    post:
      tags:
        - ABAC Policy Management
      summary: Imports a configured ABAC policy version
      description: Creates a staged policy version by default. When activate=true is supplied, the import and activation are executed atomically and the returned version is active.
      operationId: ImportABACPolicyVersion
      requestBody:
        required: true
        content:
          application/json:
            schema:
              $ref: '#/components/schemas/ABACPolicyImportRequest'
            examples:
              stagedImport:
                summary: Import as staged policy version
                value:
                  source_ref: admin-upload-2026-06-14
                  policy:
                    AllAccessPermissionRules:
                      rules:
                        - ACL:
                            ACCESS: ALLOW
                            RIGHTS: [READ]
                            ATTRIBUTES:
                              - CLAIM: role
                          OBJECTS:
                            - ROUTE: /description
                          FORMULA:
                            $eq:
                              - $attribute:
                                  CLAIM: role
                              - $strVal: admin
              importAndActivate:
                summary: Import and activate atomically
                value:
                  source_ref: emergency-policy
                  activate: true
                  policy:
                    AllAccessPermissionRules:
                      rules:
                        - ACL:
                            ACCESS: ALLOW
                            RIGHTS: [ALL]
                            ATTRIBUTES:
                              - CLAIM: role
                          OBJECTS:
                            - ROUTE: /security/abac
                            - ROUTE: /security/abac/*
                          FORMULA:
                            $eq:
                              - $attribute:
                                  CLAIM: role
                              - $strVal: admin
      responses:
        '201':
          description: Imported ABAC policy version
          content:
            application/json:
              schema:
                $ref: '#/components/schemas/ABACPolicyVersion'
  /security/abac/policy-versions/{versionID}:
    parameters:
      - name: versionID
        in: path
        required: true
        schema:
          type: integer
          format: int64
    get:
      tags:
        - ABAC Policy Management
      summary: Gets one ABAC policy version
      operationId: GetABACPolicyVersion
      responses:
        '200':
          description: ABAC policy version
          content:
            application/json:
              schema:
                $ref: '#/components/schemas/ABACPolicyVersion'
  /security/abac/policy-versions/{versionID}/clone:
    parameters:
      - name: versionID
        in: path
        required: true
        schema:
          type: integer
          format: int64
    post:
      tags:
        - ABAC Policy Management
      summary: Clones a policy version to a staged editable version
      operationId: CloneABACPolicyVersion
      responses:
        '201':
          description: Cloned staged policy version
          content:
            application/json:
              schema:
                $ref: '#/components/schemas/ABACPolicyVersion'
  /security/abac/policy-versions/{versionID}/validate:
    parameters:
      - name: versionID
        in: path
        required: true
        schema:
          type: integer
          format: int64
    post:
      tags:
        - ABAC Policy Management
      summary: Validates and materializes a staged ABAC policy version
      operationId: ValidateABACPolicyVersion
      responses:
        '200':
          description: Validation result
          content:
            application/json:
              schema:
                $ref: '#/components/schemas/ABACValidationResult'
  /security/abac/policy-versions/{versionID}/activate:
    parameters:
      - name: versionID
        in: path
        required: true
        schema:
          type: integer
          format: int64
    post:
      tags:
        - ABAC Policy Management
      summary: Activates a staged ABAC policy version
      operationId: ActivateABACPolicyVersion
      responses:
        '200':
          description: Active ABAC policy version
          content:
            application/json:
              schema:
                $ref: '#/components/schemas/ABACPolicyVersion'
  /security/abac/policy-versions/{versionID}/reject:
    parameters:
      - name: versionID
        in: path
        required: true
        schema:
          type: integer
          format: int64
    post:
      tags:
        - ABAC Policy Management
      summary: Rejects a staged ABAC policy version
      operationId: RejectABACPolicyVersion
      responses:
        '200':
          description: Rejected ABAC policy version
          content:
            application/json:
              schema:
                $ref: '#/components/schemas/ABACPolicyVersion'
  /security/abac/policy-versions/{versionID}/rules:
    parameters:
      - name: versionID
        in: path
        required: true
        schema:
          type: integer
          format: int64
    get:
      tags:
        - ABAC Policy Management
      summary: Lists materialized rules for a policy version
      operationId: ListABACPolicyRules
      responses:
        '200':
          description: ABAC policy rules
          content:
            application/json:
              schema:
                type: array
                items:
                  $ref: '#/components/schemas/ABACPolicyRule'
    post:
      tags:
        - ABAC Policy Management
      summary: Creates one rule in a staged ABAC policy version
      operationId: CreateABACPolicyRule
      requestBody:
        required: true
        content:
          application/json:
            schema:
              oneOf:
                - $ref: '#/components/schemas/ABACRuleMutationRequest'
                - $ref: '#/components/schemas/AccessPermissionRule'
            examples:
              wrappedRule:
                summary: Insert a rule at an explicit position
                value:
                  position: 2
                  rule:
                    ACL:
                      ACCESS: ALLOW
                      RIGHTS: [READ]
                      ATTRIBUTES:
                        - CLAIM: role
                    OBJECTS:
                      - ROUTE: /submodels
                      - ROUTE: /submodels/*
                    FORMULA:
                      $and:
                        - $eq:
                            - $attribute:
                                CLAIM: role
                            - $strVal: editor
                        - $eq:
                            - $field: $sm#id
                            - $strVal: urn:example:submodel:visible
              directRule:
                summary: Append a direct AccessPermissionRule body
                value:
                  ACL:
                    ACCESS: ALLOW
                    RIGHTS: [READ]
                    ATTRIBUTES:
                      - GLOBAL: ANONYMOUS
                  OBJECTS:
                    - ROUTE: /description
                  FORMULA:
                    $boolean: true
      responses:
        '200':
          description: Updated staged policy version
          content:
            application/json:
              schema:
                $ref: '#/components/schemas/ABACPolicyVersion'
  /security/abac/policy-versions/{versionID}/rules/{ruleIndex}:
    parameters:
      - name: versionID
        in: path
        required: true
        schema:
          type: integer
          format: int64
      - name: ruleIndex
        in: path
        required: true
        schema:
          type: integer
          minimum: 1
    get:
      tags:
        - ABAC Policy Management
      summary: Gets one ABAC policy rule
      operationId: GetABACPolicyRule
      responses:
        '200':
          description: ABAC policy rule
          content:
            application/json:
              schema:
                $ref: '#/components/schemas/ABACPolicyRule'
    put:
      tags:
        - ABAC Policy Management
      summary: Replaces one rule in a staged ABAC policy version
      operationId: ReplaceABACPolicyRule
      requestBody:
        required: true
        content:
          application/json:
            schema:
              $ref: '#/components/schemas/AccessPermissionRule'
            example:
              ACL:
                ACCESS: ALLOW
                RIGHTS: [READ]
                ATTRIBUTES:
                  - CLAIM: role
              OBJECTS:
                - ROUTE: /shells
              FORMULA:
                $eq:
                  - $attribute:
                      CLAIM: role
                  - $strVal: viewer
      responses:
        '200':
          description: Updated staged policy version
          content:
            application/json:
              schema:
                $ref: '#/components/schemas/ABACPolicyVersion'
    patch:
      tags:
        - ABAC Policy Management
      summary: Merge-patches one rule in a staged ABAC policy version
      operationId: PatchABACPolicyRule
      requestBody:
        required: true
        content:
          application/json:
            schema:
              type: object
              additionalProperties: true
              description: JSON object merge patch. Null removes fields; this is not RFC 6902.
            example:
              USEFORMULA: null
              FORMULA:
                $boolean: true
      responses:
        '200':
          description: Updated staged policy version
          content:
            application/json:
              schema:
                $ref: '#/components/schemas/ABACPolicyVersion'
    delete:
      tags:
        - ABAC Policy Management
      summary: Deletes one rule from a staged ABAC policy version
      operationId: DeleteABACPolicyRule
      responses:
        '200':
          description: Updated staged policy version
          content:
            application/json:
              schema:
                $ref: '#/components/schemas/ABACPolicyVersion'
  /security/abac/policy-versions/{versionID}/rules/{ruleIndex}/duplicate:
    parameters:
      - name: versionID
        in: path
        required: true
        schema:
          type: integer
          format: int64
      - name: ruleIndex
        in: path
        required: true
        schema:
          type: integer
          minimum: 1
    post:
      tags:
        - ABAC Policy Management
      summary: Duplicates one rule in a staged ABAC policy version
      operationId: DuplicateABACPolicyRule
      responses:
        '200':
          description: Updated staged policy version
          content:
            application/json:
              schema:
                $ref: '#/components/schemas/ABACPolicyVersion'
  /security/abac/policy-versions/{versionID}/rules/{ruleIndex}/move:
    parameters:
      - name: versionID
        in: path
        required: true
        schema:
          type: integer
          format: int64
      - name: ruleIndex
        in: path
        required: true
        schema:
          type: integer
          minimum: 1
    post:
      tags:
        - ABAC Policy Management
      summary: Moves one rule within a staged ABAC policy version
      operationId: MoveABACPolicyRule
      requestBody:
        required: true
        content:
          application/json:
            schema:
              $ref: '#/components/schemas/ABACMoveRuleRequest'
      responses:
        '200':
          description: Updated staged policy version
          content:
            application/json:
              schema:
                $ref: '#/components/schemas/ABACPolicyVersion'
  /security/abac/policy-versions/{versionID}/rules/{ruleIndex}/enabled:
    parameters:
      - name: versionID
        in: path
        required: true
        schema:
          type: integer
          format: int64
      - name: ruleIndex
        in: path
        required: true
        schema:
          type: integer
          minimum: 1
    put:
      tags:
        - ABAC Policy Management
      summary: Enables or disables one rule in a staged ABAC policy version
      operationId: SetABACPolicyRuleEnabled
      requestBody:
        required: true
        content:
          application/json:
            schema:
              $ref: '#/components/schemas/ABACSetRuleEnabledRequest'
      responses:
        '200':
          description: Updated staged policy version
          content:
            application/json:
              schema:
                $ref: '#/components/schemas/ABACPolicyVersion'
  /security/abac/policy-versions/{versionID}/definitions:
    parameters:
      - name: versionID
        in: path
        required: true
        schema:
          type: integer
          format: int64
    get:
      tags:
        - ABAC Policy Management
      summary: Lists reusable ABAC definitions for a policy version
      operationId: ListABACPolicyDefinitions
      responses:
        '200':
          description: ABAC policy definitions grouped by kind
          content:
            application/json:
              schema:
                $ref: '#/components/schemas/ABACPolicyDefinitions'
  /security/abac/policy-versions/{versionID}/definitions/{kind}:
    parameters:
      - name: versionID
        in: path
        required: true
        schema:
          type: integer
          format: int64
      - name: kind
        in: path
        required: true
        schema:
          $ref: '#/components/schemas/ABACDefinitionKind'
    get:
      tags:
        - ABAC Policy Management
      summary: Lists reusable ABAC definitions by kind
      operationId: ListABACPolicyDefinitionsByKind
      responses:
        '200':
          description: ABAC policy definitions for the selected kind
          content:
            application/json:
              schema:
                oneOf:
                  - type: array
                    items:
                      $ref: '#/components/schemas/ABACAttributeDefinition'
                  - type: array
                    items:
                      $ref: '#/components/schemas/ABACACLDefinition'
                  - type: array
                    items:
                      $ref: '#/components/schemas/ABACObjectDefinition'
                  - type: array
                    items:
                      $ref: '#/components/schemas/ABACFormulaDefinition'
    post:
      tags:
        - ABAC Policy Management
      summary: Creates one reusable definition in a staged ABAC policy version
      operationId: CreateABACPolicyDefinition
      requestBody:
        required: true
        content:
          application/json:
            schema:
              $ref: '#/components/schemas/ABACDefinition'
            examples:
              attributes:
                summary: Create DEFATTRIBUTES entry
                value:
                  name: adminClaims
                  attributes:
                    - CLAIM: role
              acl:
                summary: Create DEFACLS entry
                value:
                  name: abacAdmins
                  acl:
                    ACCESS: ALLOW
                    RIGHTS: [READ, CREATE, UPDATE, DELETE]
                    USEATTRIBUTES: adminClaims
              objects:
                summary: Create DEFOBJECTS entry
                value:
                  name: abacManagement
                  objects:
                    - ROUTE: /security/abac/*
              formula:
                summary: Create DEFFORMULAS entry
                value:
                  name: isAdmin
                  formula:
                    $eq:
                      - $attribute:
                          CLAIM: role
                      - $strVal: admin
      responses:
        '200':
          description: Updated staged policy version
          content:
            application/json:
              schema:
                $ref: '#/components/schemas/ABACPolicyVersion'
  /security/abac/policy-versions/{versionID}/definitions/{kind}/{name}:
    parameters:
      - name: versionID
        in: path
        required: true
        schema:
          type: integer
          format: int64
      - name: kind
        in: path
        required: true
        schema:
          $ref: '#/components/schemas/ABACDefinitionKind'
      - name: name
        in: path
        required: true
        schema:
          type: string
    get:
      tags:
        - ABAC Policy Management
      summary: Gets one reusable ABAC definition
      operationId: GetABACPolicyDefinition
      responses:
        '200':
          description: ABAC policy definition
          content:
            application/json:
              schema:
                $ref: '#/components/schemas/ABACDefinition'
    put:
      tags:
        - ABAC Policy Management
      summary: Replaces one reusable definition in a staged ABAC policy version
      description: The definition body name must match the path name. Rename by deleting and recreating the definition so references are revalidated explicitly.
      operationId: ReplaceABACPolicyDefinition
      requestBody:
        required: true
        content:
          application/json:
            schema:
              $ref: '#/components/schemas/ABACDefinition'
            example:
              name: isAdmin
              formula:
                $eq:
                  - $attribute:
                      CLAIM: scope
                  - $strVal: basyx.abac.admin
      responses:
        '200':
          description: Updated staged policy version
          content:
            application/json:
              schema:
                $ref: '#/components/schemas/ABACPolicyVersion'
    patch:
      tags:
        - ABAC Policy Management
      summary: Merge-patches one reusable definition in a staged ABAC policy version
      operationId: PatchABACPolicyDefinition
      requestBody:
        required: true
        content:
          application/json:
            schema:
              type: object
              additionalProperties: true
              description: JSON object merge patch. Null removes fields; this is not RFC 6902. The resulting definition name must still match the path name.
            example:
              formula:
                $boolean: true
      responses:
        '200':
          description: Updated staged policy version
          content:
            application/json:
              schema:
                $ref: '#/components/schemas/ABACPolicyVersion'
    delete:
      tags:
        - ABAC Policy Management
      summary: Deletes one reusable definition from a staged ABAC policy version
      description: Deletion is rejected when rules or other definitions still reference the deleted definition.
      operationId: DeleteABACPolicyDefinition
      responses:
        '200':
          description: Updated staged policy version
          content:
            application/json:
              schema:
                $ref: '#/components/schemas/ABACPolicyVersion'
`

const abacManagementSchemasYAML = `    ABACPolicyVersion:
      type: object
      description: Versioned ABAC policy snapshot stored for one service scope.
      properties:
        version_id:
          type: integer
          format: int64
        service_scope:
          type: string
        policy_id:
          type: string
          description: SHA-256 hash of the canonical configured policy JSON.
        status:
          type: string
          enum: [staged, active, superseded, rejected]
        source_type:
          type: string
          enum: [file, api]
        source_ref:
          type: string
        configured_policy_json:
          type: object
          additionalProperties: true
        configured_policy_hash:
          type: string
        raw_policy_hash:
          type: string
        materialized_policy_json:
          type: object
          additionalProperties: true
        materialized_policy_hash:
          type: string
        created_at:
          type: string
          format: date-time
        updated_at:
          type: string
          format: date-time
        activated_at:
          type: string
          format: date-time
        superseded_at:
          type: string
          format: date-time
        artifact_ref:
          type: object
          additionalProperties: true
    ABACPolicyRule:
      type: object
      description: One ordered, materialized ABAC rule row.
      properties:
        rule_id:
          type: integer
          format: int64
        version_id:
          type: integer
          format: int64
        policy_id:
          type: string
        service_scope:
          type: string
        rule_index:
          type: integer
          minimum: 1
        matched_rule_id:
          type: string
          example: rule:1:0123456789abcdef
        configured_rule_json:
          $ref: '#/components/schemas/AccessPermissionRule'
        materialized_rule_json:
          type: object
          additionalProperties: true
        acl_json:
          $ref: '#/components/schemas/ABACACL'
        attributes_json:
          type: array
          items:
            $ref: '#/components/schemas/ABACAttributeItem'
        objects_json:
          type: array
          items:
            $ref: '#/components/schemas/ABACObjectItem'
        formula_json:
          $ref: '#/components/schemas/ABACLogicalExpression'
        filters_json:
          type: array
          items:
            $ref: '#/components/schemas/ABACAccessPermissionRuleFilter'
        access:
          type: string
          enum: [ALLOW, DISABLED]
        rights:
          type: array
          items:
            type: string
        rule_hash:
          type: string
        materialized_rule_hash:
          type: string
        created_at:
          type: string
          format: date-time
    ABACPolicyDefinitions:
      type: object
      description: Reusable ABAC definition sections from the configured policy JSON.
      properties:
        attributes:
          type: array
          items:
            $ref: '#/components/schemas/ABACAttributeDefinition'
        acls:
          type: array
          items:
            $ref: '#/components/schemas/ABACACLDefinition'
        objects:
          type: array
          items:
            $ref: '#/components/schemas/ABACObjectDefinition'
        formulas:
          type: array
          items:
            $ref: '#/components/schemas/ABACFormulaDefinition'
    ABACDefinitionKind:
      type: string
      enum: [attributes, acls, objects, formulas]
      description: Definition kind. The API also accepts the original DEFATTRIBUTES, DEFACLS, DEFOBJECTS, and DEFFORMULAS names.
    ABACDefinition:
      oneOf:
        - $ref: '#/components/schemas/ABACAttributeDefinition'
        - $ref: '#/components/schemas/ABACACLDefinition'
        - $ref: '#/components/schemas/ABACObjectDefinition'
        - $ref: '#/components/schemas/ABACFormulaDefinition'
    ABACAttributeDefinition:
      type: object
      required: [name, attributes]
      properties:
        name:
          type: string
        attributes:
          type: array
          items:
            $ref: '#/components/schemas/ABACAttributeItem'
      example:
        name: adminClaims
        attributes:
          - CLAIM: role
    ABACACLDefinition:
      type: object
      required: [name, acl]
      properties:
        name:
          type: string
        acl:
          $ref: '#/components/schemas/ABACACL'
      example:
        name: abacAdmins
        acl:
          ACCESS: ALLOW
          RIGHTS: [READ, CREATE, UPDATE, DELETE]
          ATTRIBUTES:
            - CLAIM: role
    ABACObjectDefinition:
      type: object
      required: [name]
      description: Reusable object selector. Exactly one of objects or USEOBJECTS must be set.
      properties:
        name:
          type: string
        objects:
          type: array
          items:
            $ref: '#/components/schemas/ABACObjectItem'
        USEOBJECTS:
          type: array
          items:
            type: string
      example:
        name: abacManagement
        objects:
          - ROUTE: /security/abac/*
    ABACFormulaDefinition:
      type: object
      required: [name, formula]
      properties:
        name:
          type: string
        formula:
          $ref: '#/components/schemas/ABACLogicalExpression'
      example:
        name: isAdmin
        formula:
          $eq:
            - $attribute:
                CLAIM: role
            - $strVal: admin
    ABACPolicyImportRequest:
      type: object
      required: [policy]
      properties:
        source_ref:
          type: string
          description: Non-secret source reference for audit metadata.
        activate:
          type: boolean
          description: When false or omitted, import creates a staged version. When true, import and activation are executed atomically and the returned version is active.
        policy:
          type: object
          required: [AllAccessPermissionRules]
          additionalProperties: true
          description: Complete configured ABAC policy JSON accepted by abac.modelPath.
    ABACRuleMutationRequest:
      type: object
      required: [rule]
      properties:
        position:
          type: integer
          minimum: 1
          description: Optional 1-based insert position. Omitted or out of range appends.
        rule:
          $ref: '#/components/schemas/AccessPermissionRule'
    AccessPermissionRule:
      type: object
      description: Configured ABAC access rule. Exactly one of ACL/USEACL, OBJECTS/USEOBJECTS, and FORMULA/USEFORMULA must be set.
      properties:
        ACL:
          $ref: '#/components/schemas/ABACACL'
        USEACL:
          type: string
        OBJECTS:
          type: array
          items:
            $ref: '#/components/schemas/ABACObjectItem'
        USEOBJECTS:
          type: array
          items:
            type: string
        FORMULA:
          $ref: '#/components/schemas/ABACLogicalExpression'
        USEFORMULA:
          type: string
        FILTER:
          $ref: '#/components/schemas/ABACAccessPermissionRuleFilter'
        FILTERLIST:
          type: array
          items:
            $ref: '#/components/schemas/ABACAccessPermissionRuleFilter'
      example:
        ACL:
          ACCESS: ALLOW
          RIGHTS: [READ]
          ATTRIBUTES:
            - CLAIM: role
        OBJECTS:
          - ROUTE: /submodels
          - ROUTE: /submodels/*
        FORMULA:
          $and:
            - $eq:
                - $attribute:
                    CLAIM: role
                - $strVal: editor
            - $eq:
                - $field: $sm#id
                - $strVal: urn:example:submodel:visible
    ABACACL:
      type: object
      required: [ACCESS, RIGHTS]
      properties:
        ACCESS:
          type: string
          enum: [ALLOW, DISABLED]
        RIGHTS:
          type: array
          items:
            type: string
            enum: [CREATE, READ, UPDATE, DELETE, EXECUTE, VIEW, ALL]
        ATTRIBUTES:
          type: array
          items:
            $ref: '#/components/schemas/ABACAttributeItem'
        USEATTRIBUTES:
          type: string
      description: Exactly one of ATTRIBUTES or USEATTRIBUTES must be set.
    ABACAttributeItem:
      type: object
      description: Attribute source used by ACLs and formulas. Exactly one key is allowed.
      oneOf:
        - required: [CLAIM]
        - required: [GLOBAL]
        - required: [REFERENCE]
      properties:
        CLAIM:
          type: string
          example: role
        GLOBAL:
          type: string
          enum: [LOCALNOW, UTCNOW, CLIENTNOW, ANONYMOUS]
        REFERENCE:
          type: string
          example: $sm#id
    ABACObjectItem:
      type: object
      description: Route, fragment, identifiable, referable, or descriptor object selector. Exactly one key is allowed.
      additionalProperties: true
      example:
        ROUTE: /security/abac/*
    ABACAccessPermissionRuleFilter:
      type: object
      required: [FRAGMENT]
      properties:
        FRAGMENT:
          type: string
          example: $sm#submodelElements[]
        MATCH:
          type: boolean
          default: false
        CONDITION:
          $ref: '#/components/schemas/ABACLogicalExpression'
        USEFORMULA:
          type: string
      description: Exactly one of CONDITION or USEFORMULA must be set.
    ABACLogicalExpression:
      type: object
      description: ABAC logical expression tree using Part 4 style operators such as $and, $or, $eq, $attribute, $field, $strVal, and $boolean.
      additionalProperties: true
      example:
        $eq:
          - $attribute:
              CLAIM: role
          - $strVal: admin
    ABACValidationResult:
      type: object
      properties:
        valid:
          type: boolean
        policy_id:
          type: string
        materialized_policy_hash:
          type: string
        error:
          type: string
    ABACMoveRuleRequest:
      type: object
      required: [position]
      properties:
        position:
          type: integer
          minimum: 1
    ABACSetRuleEnabledRequest:
      type: object
      required: [enabled]
      properties:
        enabled:
          type: boolean
`

func injectABACManagementAPI(specContent []byte) []byte {
	if abacManagementPathRegex.Match(specContent) {
		return specContent
	}
	specContent = injectABACManagementSchemas(specContent)
	return injectPathFragment(specContent, abacManagementPathsYAML)
}

func injectABACManagementSchemas(specContent []byte) []byte {
	if strings.Contains(string(specContent), "    ABACPolicyDefinitions:") {
		return specContent
	}
	return injectComponentSchemas(specContent, abacManagementSchemasYAML)
}

func injectComponentSchemas(specContent []byte, schemas string) []byte {
	lines := strings.SplitAfter(string(specContent), "\n")
	componentsIndex := topLevelLineIndex(lines, "components:")
	if componentsIndex < 0 {
		content := ensureTrailingNewline(string(specContent))
		return []byte(content + "components:\n  schemas:\n" + schemas)
	}

	nextTopLevel := nextTopLevelLineIndex(lines, componentsIndex+1)
	schemasIndex := schemasLineIndex(lines, componentsIndex+1, nextTopLevel)
	if schemasIndex >= 0 {
		return []byte(insertLines(lines, schemasIndex+1, schemas))
	}
	return []byte(insertLines(lines, componentsIndex+1, "  schemas:\n"+schemas))
}

func topLevelLineIndex(lines []string, value string) int {
	for i, line := range lines {
		if strings.TrimSpace(line) == value && strings.TrimLeft(line, " \t") == line {
			return i
		}
	}
	return -1
}

func nextTopLevelLineIndex(lines []string, start int) int {
	for i := start; i < len(lines); i++ {
		trimmed := strings.TrimSpace(lines[i])
		if trimmed == "" || strings.HasPrefix(trimmed, "#") {
			continue
		}
		if strings.TrimLeft(lines[i], " \t") == lines[i] {
			return i
		}
	}
	return len(lines)
}

func schemasLineIndex(lines []string, start int, end int) int {
	for i := start; i < end; i++ {
		line := lines[i]
		if strings.TrimSpace(line) == "schemas:" && strings.HasPrefix(line, "  ") {
			return i
		}
	}
	return -1
}

func insertLines(lines []string, index int, value string) string {
	var builder strings.Builder
	for _, line := range lines[:index] {
		_, _ = builder.WriteString(line)
	}
	_, _ = builder.WriteString(value)
	for _, line := range lines[index:] {
		_, _ = builder.WriteString(line)
	}
	return builder.String()
}

func ensureTrailingNewline(value string) string {
	if value == "" || strings.HasSuffix(value, "\n") {
		return value
	}
	return value + "\n"
}

func injectPathFragment(specContent []byte, fragment string) []byte {
	pathLoc := pathsSectionRegex.FindIndex(specContent)
	if pathLoc != nil {
		injected := make([]byte, 0, len(specContent)+len(fragment))
		injected = append(injected, specContent[:pathLoc[1]]...)
		injected = append(injected, []byte(fragment)...)
		injected = append(injected, specContent[pathLoc[1]:]...)
		return injected
	}

	appended := make([]byte, 0, len(specContent)+len(fragment)+8)
	appended = append(appended, specContent...)
	if len(specContent) > 0 && specContent[len(specContent)-1] != '\n' {
		appended = append(appended, '\n')
	}
	appended = append(appended, []byte("paths:\n")...)
	appended = append(appended, []byte(fragment)...)
	return appended
}

// injectServerURL modifies the OpenAPI spec to use the configured server URL
func injectServerURL(specContent []byte, serverURL string) []byte {
	if serverURL == "" {
		return specContent
	}

	newServers := fmt.Sprintf("servers:\n- url: '%s'\n  description: Auto-configured server\n", serverURL)

	// Replace existing servers section - match from "servers:" to the next top-level key (paths:, etc.)
	// The servers section ends when we hit a line starting with a non-space character that isn't part of the array
	serversRegex := regexp.MustCompile(`(?ms)^servers:\s*\n((?:[ \t]*-[^\n]*\n?|[ \t]+[^\n]*\n?)*)`)

	if serversRegex.Match(specContent) {
		return serversRegex.ReplaceAll(specContent, []byte(newServers))
	}

	// If no servers section exists, add it after info section (before paths)
	pathsRegex := regexp.MustCompile(`(?m)^(paths:)`)
	if pathsRegex.Match(specContent) {
		return pathsRegex.ReplaceAll(specContent, []byte(newServers+"$1"))
	}

	// Fallback: add after openapi version line
	openapiRegex := regexp.MustCompile(`(?m)^(openapi:\s*.+\n)`)
	if openapiRegex.Match(specContent) {
		return openapiRegex.ReplaceAll(specContent, []byte("$1"+newServers))
	}

	// Last resort: prepend servers section
	return append([]byte(newServers), specContent...)
}

// injectContact modifies the OpenAPI spec to use the configured contact information
func injectContact(specContent []byte, contact *ContactConfig) []byte {
	if contact == nil {
		return specContent
	}

	// Build new contact section
	var contactLines []string
	contactLines = append(contactLines, "  contact:")
	if contact.Name != "" {
		contactLines = append(contactLines, fmt.Sprintf("    name: %s", contact.Name))
	}
	if contact.Email != "" {
		contactLines = append(contactLines, fmt.Sprintf("    email: %s", contact.Email))
	}
	if contact.URL != "" {
		contactLines = append(contactLines, fmt.Sprintf("    url: %s", contact.URL))
	}
	newContact := strings.Join(contactLines, "\n") + "\n"

	// Replace existing contact section within info block
	// Match "  contact:" followed by indented lines (more than 2 spaces)
	contactRegex := regexp.MustCompile(`(?m)^  contact:\s*\n((?:    [^\n]*\n?)*)`)

	if contactRegex.Match(specContent) {
		return contactRegex.ReplaceAll(specContent, []byte(newContact))
	}

	// If no contact section exists, add it after info: title line
	titleRegex := regexp.MustCompile(`(?m)^(  title:[^\n]*\n)`)
	if titleRegex.Match(specContent) {
		return titleRegex.ReplaceAll(specContent, []byte("$1"+newContact))
	}

	return specContent
}

// AddSwaggerUI adds Swagger UI endpoints to the router
//
// Parameters:
//   - r: Chi router to add endpoints to
//   - cfg: Swagger UI configuration
//
// This adds two endpoints:
//   - cfg.UIPath: Serves the Swagger UI HTML page
//   - cfg.SpecPath: Serves the OpenAPI specification file
func AddSwaggerUI(r *chi.Mux, cfg SwaggerUIConfig) {
	// Inject server URL into spec if configured
	specContent := cfg.SpecContent
	if cfg.ServerURL != "" {
		specContent = injectServerURL(specContent, cfg.ServerURL)
	}

	// Inject contact information if configured
	if cfg.Contact != nil {
		specContent = injectContact(specContent, cfg.Contact)
	}

	// Repoint Part2 schema references to local, bundled schema snapshots so Swagger works offline.
	specContent = localizePart2SchemaReferences(specContent, cfg.SpecPath)

	includeVerifyEndpoint := true
	if cfg.IncludeVerifyEndpoint != nil {
		includeVerifyEndpoint = *cfg.IncludeVerifyEndpoint
	}
	if includeVerifyEndpoint {
		specContent = injectVerifyEndpoint(specContent)
	}
	includeABACManagement := false
	if cfg.IncludeABACManagement != nil {
		includeABACManagement = *cfg.IncludeABACManagement
	}
	if includeABACManagement {
		specContent = injectABACManagementAPI(specContent)
	}

	// Serve the OpenAPI spec
	r.Get(cfg.SpecPath, func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/yaml")
		_, _ = w.Write(specContent)
	})

	part2SchemaPath := path.Clean(path.Dir(cfg.SpecPath) + "/part2-schemas/{version}/openapi.yaml")
	r.Get(part2SchemaPath, func(w http.ResponseWriter, req *http.Request) {
		version := chi.URLParam(req, "version")
		if version == "" {
			http.NotFound(w, req)
			return
		}

		schemaPath := path.Clean("swagger_part2_schemas/" + version + "/openapi.yaml")
		schemaContent, err := fs.ReadFile(part2SchemasFS, schemaPath)
		if err != nil {
			http.NotFound(w, req)
			return
		}

		w.Header().Set("Content-Type", "application/yaml")
		// #nosec G705 -- schemaContent is loaded from trusted embedded files only.
		_, _ = w.Write(schemaContent)
	})

	// Serve Swagger UI
	tmpl := template.Must(template.New("swagger").Parse(SwaggerUIHTML))
	r.Get(cfg.UIPath, func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_ = tmpl.Execute(w, struct {
			Title   string
			SpecURL string
		}{
			Title:   cfg.Title,
			SpecURL: cfg.SpecURL,
		})
	})

	// Add redirect from base path to Swagger UI
	if cfg.BasePath != "" {
		r.Get(cfg.BasePath, func(w http.ResponseWriter, req *http.Request) {
			http.Redirect(w, req, cfg.UIPath, http.StatusFound)
		})
		// Also handle base path with trailing slash
		if !strings.HasSuffix(cfg.BasePath, "/") {
			r.Get(cfg.BasePath+"/", func(w http.ResponseWriter, req *http.Request) {
				http.Redirect(w, req, cfg.UIPath, http.StatusFound)
			})
		}
	}

	log.Printf("📖 Swagger UI available at %s", cfg.UIPath)
	log.Printf("📄 OpenAPI spec available at %s", cfg.SpecPath)
}

// AddSwaggerUIFromFS adds Swagger UI endpoints using an embedded filesystem
//
// Parameters:
//   - r: Chi router to add endpoints to
//   - specFS: Embedded filesystem containing the OpenAPI spec
//   - specFile: Path to the spec file within the embedded FS
//   - title: Title for the Swagger UI page
//   - uiPath: URL path for Swagger UI (e.g., "/swagger")
//   - specPath: URL path for the spec file (e.g., "/api-docs/openapi.yaml")
//   - serverConfig: Server configuration for building the server URL
func AddSwaggerUIFromFS(r *chi.Mux, specFS embed.FS, specFile string, title string, uiPath string, specPath string, serverConfig *Config) error {
	content, err := fs.ReadFile(specFS, specFile)
	if err != nil {
		return err
	}

	// Build server URL and paths from config
	serverURL := ""
	contextPath := ""
	if serverConfig != nil {
		host := serverConfig.Server.Host
		// Use localhost for display if host is 0.0.0.0
		if host == "0.0.0.0" || host == "" {
			host = "localhost"
		}
		serverURL = fmt.Sprintf("http://%s:%d", host, serverConfig.Server.Port)
		if serverConfig.Server.ContextPath != "" {
			// Ensure context path starts with / but doesn't end with /
			contextPath = serverConfig.Server.ContextPath
			if !bytes.HasPrefix([]byte(contextPath), []byte("/")) {
				contextPath = "/" + contextPath
			}
			// Remove trailing slash if present
			contextPath = strings.TrimSuffix(contextPath, "/")
			serverURL += contextPath
		}
	}

	// Prepend context path to UI and spec paths
	fullUIPath := contextPath + uiPath
	fullSpecPath := contextPath + specPath

	// Base path for redirect (context path or "/" if no context path)
	basePath := contextPath
	if basePath == "" {
		basePath = "/"
	}

	// Build contact config if provided
	var contact *ContactConfig
	var includeVerifyEndpoint *bool
	var includeABACManagement *bool
	if serverConfig != nil && (serverConfig.Swagger.ContactName != "" || serverConfig.Swagger.ContactEmail != "" || serverConfig.Swagger.ContactURL != "") {
		contact = &ContactConfig{
			Name:  serverConfig.Swagger.ContactName,
			Email: serverConfig.Swagger.ContactEmail,
			URL:   serverConfig.Swagger.ContactURL,
		}
	}
	if serverConfig != nil {
		includeVerifyEndpoint = &serverConfig.Server.VerificationEndpointAvailable
		abacManagementEnabled := shouldIncludeABACManagement(serverConfig)
		includeABACManagement = &abacManagementEnabled
	}

	AddSwaggerUI(r, SwaggerUIConfig{
		Title:                 title,
		SpecURL:               fullSpecPath,
		UIPath:                fullUIPath,
		SpecPath:              fullSpecPath,
		SpecContent:           content,
		ServerURL:             serverURL,
		BasePath:              basePath,
		Contact:               contact,
		IncludeVerifyEndpoint: includeVerifyEndpoint,
		IncludeABACManagement: includeABACManagement,
	})

	return nil
}

func shouldIncludeABACManagement(serverConfig *Config) bool {
	return serverConfig != nil && serverConfig.ABAC.Enabled && serverConfig.ABAC.ManagementAPI.Enabled
}
