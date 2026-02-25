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

	securitydocu "github.com/eclipse-basyx/basyx-go-components/docu/security"
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

// SwaggerUIConfig holds configuration for Swagger UI endpoint setup
type SwaggerUIConfig struct {
	Title       string         // Title shown in browser tab
	SpecURL     string         // URL to the OpenAPI spec (e.g., "/api-docs/openapi.yaml")
	UIPath      string         // Path where Swagger UI will be served (e.g., "/swagger")
	SpecPath    string         // Path where spec will be served (e.g., "/api-docs/openapi.yaml")
	SpecContent []byte         // The OpenAPI spec content
	ServerURL   string         // Server URL to use in OpenAPI spec (e.g., "http://localhost:5004/api")
	BasePath    string         // Base path for redirect to Swagger UI (e.g., "/" or "/api")
	Contact     *ContactConfig // Contact information to inject into OpenAPI spec
}

// ContactConfig holds contact information for OpenAPI spec
type ContactConfig struct {
	Name  string // Contact name
	Email string // Contact email
	URL   string // Contact URL
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

	// Serve the OpenAPI spec
	r.Get(cfg.SpecPath, func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/yaml")
		_, _ = w.Write(specContent)
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
	fullSharedRulesSpecPath := contextPath + path.Join(path.Dir(specPath), "openapi_rules_management.yaml")

	// Base path for redirect (context path or "/" if no context path)
	basePath := contextPath
	if basePath == "" {
		basePath = "/"
	}

	// Build contact config if provided
	var contact *ContactConfig
	if serverConfig != nil && (serverConfig.Swagger.ContactName != "" || serverConfig.Swagger.ContactEmail != "" || serverConfig.Swagger.ContactURL != "") {
		contact = &ContactConfig{
			Name:  serverConfig.Swagger.ContactName,
			Email: serverConfig.Swagger.ContactEmail,
			URL:   serverConfig.Swagger.ContactURL,
		}
	}

	AddSwaggerUI(r, SwaggerUIConfig{
		Title:       title,
		SpecURL:     fullSpecPath,
		UIPath:      fullUIPath,
		SpecPath:    fullSpecPath,
		SpecContent: content,
		ServerURL:   serverURL,
		BasePath:    basePath,
		Contact:     contact,
	})

	if sharedRulesSpec, sharedErr := securitydocu.OpenAPIRulesManagementYAML(); sharedErr == nil {
		r.Get(fullSharedRulesSpecPath, func(w http.ResponseWriter, _ *http.Request) {
			w.Header().Set("Content-Type", "application/yaml")
			_, _ = w.Write(sharedRulesSpec)
		})
		log.Printf("📄 Shared OpenAPI rules spec available at %s", fullSharedRulesSpecPath)
	} else {
		log.Printf("Warning: failed to load shared OpenAPI rules spec: %v", sharedErr)
	}

	return nil
}
