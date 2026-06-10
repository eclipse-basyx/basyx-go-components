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

const abacManagementPathsYAML = `  /security/abac/policy-versions:
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
                  type: object
    post:
      tags:
        - ABAC Policy Management
      summary: Imports a configured ABAC policy as a staged version
      operationId: ImportABACPolicyVersion
      requestBody:
        required: true
        content:
          application/json:
            schema:
              type: object
      responses:
        '201':
          description: Imported ABAC policy version
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
                type: object
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
                type: object
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
                  type: object
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
              type: object
      responses:
        '200':
          description: Updated staged policy version
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
              type: object
      responses:
        '200':
          description: Updated staged policy version
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
      responses:
        '200':
          description: Updated staged policy version
    delete:
      tags:
        - ABAC Policy Management
      summary: Deletes one rule from a staged ABAC policy version
      operationId: DeleteABACPolicyRule
      responses:
        '200':
          description: Updated staged policy version
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
              type: object
              properties:
                position:
                  type: integer
                  minimum: 1
      responses:
        '200':
          description: Updated staged policy version
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
              type: object
              required:
                - enabled
              properties:
                enabled:
                  type: boolean
      responses:
        '200':
          description: Updated staged policy version
`

func injectABACManagementAPI(specContent []byte) []byte {
	if abacManagementPathRegex.Match(specContent) {
		return specContent
	}
	return injectPathFragment(specContent, abacManagementPathsYAML)
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
