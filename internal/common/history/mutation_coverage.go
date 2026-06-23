/*******************************************************************************
* Copyright (C) 2026 the Eclipse BaSyx Authors and Fraunhofer IESE
*
* Permission is hereby granted, free of charge, to any person obtaining
* a copy of this software and associated documentation files (the
* "Software"), to deal in the Software without restriction, including
* without limitation the rights to use, copy, modify, merge, publish,
* distribute, sublicense, and/or sell copies of the Software, and to
* permit persons to whom the Software is furnished to do so, subject to
* the following conditions:
*
* The above copyright notice and this permission notice shall be
* included in all copies or substantial portions of the Software.
*
* THE SOFTWARE IS PROVIDED "AS IS", WITHOUT WARRANTY OF ANY KIND,
* EXPRESS OR IMPLIED, INCLUDING BUT NOT LIMITED TO THE WARRANTIES OF
* MERCHANTABILITY, FITNESS FOR A PARTICULAR PURPOSE AND
* NONINFRINGEMENT. IN NO EVENT SHALL THE AUTHORS OR COPYRIGHT HOLDERS BE
* LIABLE FOR ANY CLAIM, DAMAGES OR OTHER LIABILITY, WHETHER IN AN ACTION
* OF CONTRACT, TORT OR OTHERWISE, ARISING FROM, OUT OF OR IN CONNECTION
* WITH THE SOFTWARE OR THE USE OR OTHER DEALINGS IN THE SOFTWARE.
*
* SPDX-License-Identifier: MIT
******************************************************************************/

package history

import (
	"context"
	"net/http"
	"strings"
	"sync"

	"github.com/go-chi/chi/v5"
)

const unclassifiedMutationError = "HISTORY-COVERAGE-UNCLASSIFIED mutation endpoint is not classified for versioning"

var versionedMutationOperations = map[string]struct{}{
	"DeleteAssetAdministrationShellById":              {},
	"DeleteAssetAdministrationShellDescriptorById":    {},
	"DeleteConceptDescriptionById":                    {},
	"DeleteFileByPathAasRepository":                   {},
	"DeleteFileByPathSubmodelRepo":                    {},
	"DeleteSubmodelById":                              {},
	"DeleteSubmodelByIdAasRepository":                 {},
	"DeleteSubmodelByIdSigned":                        {},
	"DeleteSubmodelDescriptorById":                    {},
	"DeleteSubmodelDescriptorByIdThroughSuperpath":    {},
	"DeleteSubmodelElementByPathAasRepository":        {},
	"DeleteSubmodelElementByPathSubmodelRepo":         {},
	"DeleteSubmodelReferenceAasRepository":            {},
	"DeleteThumbnailAasRepository":                    {},
	"PatchSubmodelAasRepository":                      {},
	"PatchSubmodelByIDMetadata":                       {},
	"PatchSubmodelByIDValueOnly":                      {},
	"PatchSubmodelById":                               {},
	"PatchSubmodelByIdMetadataAasRepository":          {},
	"PatchSubmodelByIdSigned":                         {},
	"PatchSubmodelByIdValueOnlyAasRepository":         {},
	"PatchSubmodelElementByPathMetadataSubmodelRepo":  {},
	"PatchSubmodelElementByPathSubmodelRepo":          {},
	"PatchSubmodelElementByPathValueOnlySubmodelRepo": {},
	"PatchSubmodelElementValueByPathAasRepository":    {},
	"PatchSubmodelElementValueByPathMetadata":         {},
	"PatchSubmodelElementValueByPathValueOnly":        {},
	"PostAssetAdministrationShell":                    {},
	"PostAssetAdministrationShellDescriptor":          {},
	"PostConceptDescription":                          {},
	"PostSubmodel":                                    {},
	"PostSubmodelDescriptor":                          {},
	"PostSubmodelDescriptorThroughSuperpath":          {},
	"PostSubmodelElementAasRepository":                {},
	"PostSubmodelElementByPathAasRepository":          {},
	"PostSubmodelElementByPathSubmodelRepo":           {},
	"PostSubmodelElementSubmodelRepo":                 {},
	"PostSubmodelReferenceAasRepository":              {},
	"PutAssetAdministrationShellById":                 {},
	"PutAssetAdministrationShellDescriptorById":       {},
	"PutAssetInformationAasRepository":                {},
	"PutConceptDescriptionById":                       {},
	"PutFileByPathAasRepository":                      {},
	"PutFileByPathSubmodelRepo":                       {},
	"PutSubmodelById":                                 {},
	"PutSubmodelByIdAasRepository":                    {},
	"PutSubmodelByIdSigned":                           {},
	"PutSubmodelDescriptorById":                       {},
	"PutSubmodelDescriptorByIdThroughSuperpath":       {},
	"PutSubmodelElementByPathAasRepository":           {},
	"PutSubmodelElementByPathSubmodelRepo":            {},
	"PutThumbnailAasRepository":                       {},
}

var exemptMutationOperations = map[string]struct{}{
	"DeleteAllAssetLinksById":                         {},
	"InvokeOperationAasRepository":                    {},
	"InvokeOperationAsync":                            {},
	"InvokeOperationAsyncAasRepository":               {},
	"InvokeOperationAsyncValueOnly":                   {},
	"InvokeOperationAsyncValueOnlyAasRepository":      {},
	"InvokeOperationSubmodelRepo":                     {},
	"InvokeOperationValueOnly":                        {},
	"InvokeOperationValueOnlyAasRepository":           {},
	"PostAllAssetLinksById":                           {},
	"QueryAssetAdministrationShellDescriptors":        {},
	"QueryAssetAdministrationShells":                  {},
	"QueryConceptDescriptions":                        {},
	"QuerySubmodels":                                  {},
	"SearchAllAssetAdministrationShellIdsByAssetLink": {},
}

type mutationCoverageContextKey struct{}

// MutationCoverage describes the history classification applied to an HTTP mutation.
//
// Middleware stores this value on the request context after matching a mutation
// route. Versioned identifies routes that must append history; exempt routes are
// intentionally allowed to mutate without creating a history row.
type MutationCoverage struct {
	Method    string
	Pattern   string
	Operation string
	Versioned bool
}

type mutationRoute struct {
	pattern   string
	operation string
}

// MutationCoverageGuard rejects unclassified HTTP mutations while history is active.
//
// Generated API routers register route policies through ClassifyRoute, Cover,
// and Exempt. At runtime, the guard lets reads pass through, allows covered or
// exempt mutations, and fails matching mutation routes that have no explicit
// versioning policy.
type MutationCoverageGuard struct {
	mu      sync.RWMutex
	covered map[string][]mutationRoute
	exempt  map[string][]mutationRoute
	routes  chi.Routes
}

// NewMutationCoverageGuard creates an empty mutation route classifier.
//
// Passing a chi route tree lets Middleware distinguish real unclassified
// mutations from requests that the downstream router will reject as not found or
// method not allowed.
//
// Parameters:
//   - routes: Optional chi route tree used to check whether a request matches a
//     generated handler.
//
// Returns:
//   - *MutationCoverageGuard: Guard ready for route classification and
//     middleware installation.
//
// Example:
//
//	guard := NewMutationCoverageGuard(apiRouter)
//	guard.Cover(http.MethodPut, "/shells/{aasId}")
//	apiRouter.Use(guard.Middleware)
func NewMutationCoverageGuard(routes ...chi.Routes) *MutationCoverageGuard {
	guard := &MutationCoverageGuard{
		covered: make(map[string][]mutationRoute),
		exempt:  make(map[string][]mutationRoute),
	}
	if len(routes) > 0 {
		guard.routes = routes[0]
	}
	return guard
}

// ClassifyRoute registers the history policy for a generated API route.
//
// The operation name is matched against the generated operation allowlists in
// this package. Mutation operations known to change persisted state are covered;
// known operational or delegated mutations are exempt. Non-mutating methods are
// ignored.
//
// Parameters:
//   - operation: Generated OpenAPI operation name.
//   - method: HTTP method for the route.
//   - pattern: Route pattern registered with chi.
//
// Example:
//
//	guard.ClassifyRoute("PutAssetAdministrationShellById", http.MethodPut, "/shells/{aasId}")
func (g *MutationCoverageGuard) ClassifyRoute(operation string, method string, pattern string) {
	if !isMutationMethod(method) {
		return
	}
	if _, ok := versionedMutationOperations[operation]; ok {
		g.cover(method, pattern, operation)
		return
	}
	if _, ok := exemptMutationOperations[operation]; ok {
		g.exemptRoute(method, pattern, operation)
	}
}

// Cover marks a mutation route as history-producing.
//
// Covered routes are expected to append a history row somewhere in their
// handling path when history is active.
//
// Parameters:
//   - method: HTTP mutation method.
//   - pattern: Route pattern to treat as versioned.
//
// Example:
//
//	guard.Cover(http.MethodPost, "/submodels")
func (g *MutationCoverageGuard) Cover(method string, pattern string) {
	g.cover(method, pattern, "")
}

// Exempt marks a mutation route as deliberately not history-producing.
//
// Use exemptions for mutations whose persistence is handled by another service,
// does not affect versioned AAS data, or is intentionally outside history scope.
//
// Parameters:
//   - method: HTTP mutation method.
//   - pattern: Route pattern to allow without a history row.
//
// Example:
//
//	guard.Exempt(http.MethodPost, "/query/submodels")
func (g *MutationCoverageGuard) Exempt(method string, pattern string) {
	g.exemptRoute(method, pattern, "")
}

func (g *MutationCoverageGuard) cover(method string, pattern string, operation string) {
	g.addRoute(g.covered, method, pattern, operation)
}

func (g *MutationCoverageGuard) exemptRoute(method string, pattern string, operation string) {
	g.addRoute(g.exempt, method, pattern, operation)
}

// Middleware rejects mutation requests that have no explicit history policy.
//
// When history is disabled, or when the request is not a mutation method, the
// middleware is transparent. For covered and exempt mutations it records
// MutationCoverage on the request context so downstream handlers and tests can
// inspect the classification.
//
// Parameters:
//   - next: Downstream HTTP handler.
//
// Returns:
//   - http.Handler: Handler that enforces mutation coverage before delegating.
//
// Example:
//
//	apiRouter.Use(guard.Middleware)
func (g *MutationCoverageGuard) Middleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if ActiveConfig().Mode == ModeOff || !isMutationMethod(r.Method) {
			next.ServeHTTP(w, r)
			return
		}
		requestPath := requestMutationPath(r)
		if route, ok := g.match(g.covered, r.Method, requestPath); ok {
			next.ServeHTTP(w, r.WithContext(contextWithMutationCoverage(r.Context(), r.Method, route, true)))
			return
		}
		if route, ok := g.match(g.exempt, r.Method, requestPath); ok {
			next.ServeHTTP(w, r.WithContext(contextWithMutationCoverage(r.Context(), r.Method, route, false)))
			return
		}
		if !g.hasMatchingHandler(r.Method, requestPath) {
			next.ServeHTTP(w, r)
			return
		}
		http.Error(w, unclassifiedMutationError, http.StatusInternalServerError)
	})
}

// MutationCoverageFromContext returns the route classification stored by Middleware.
//
// The boolean result is false when ctx is nil or the request did not pass through
// a covered or exempt mutation route.
//
// Parameters:
//   - ctx: Request context to inspect.
//
// Returns:
//   - MutationCoverage: Stored route classification.
//   - bool: True when coverage metadata was present.
//
// Example:
//
//	coverage, ok := MutationCoverageFromContext(ctx)
//	if ok && coverage.Versioned {
//		return AppendVersionTx(ctx, tx, TableSubmodel, id, ChangeUpdated, snapshot, false)
//	}
func MutationCoverageFromContext(ctx context.Context) (MutationCoverage, bool) {
	if ctx == nil {
		return MutationCoverage{}, false
	}
	coverage, ok := ctx.Value(mutationCoverageContextKey{}).(MutationCoverage)
	return coverage, ok
}

func (g *MutationCoverageGuard) addRoute(routes map[string][]mutationRoute, method string, pattern string, operation string) {
	method = strings.ToUpper(strings.TrimSpace(method))
	pattern = normalizeMutationPath(pattern)
	if !isMutationMethod(method) || pattern == "" {
		return
	}
	g.mu.Lock()
	defer g.mu.Unlock()
	for _, current := range routes[method] {
		if current.pattern == pattern {
			return
		}
	}
	routes[method] = append(routes[method], mutationRoute{pattern: pattern, operation: strings.TrimSpace(operation)})
}

func (g *MutationCoverageGuard) match(routes map[string][]mutationRoute, method string, path string) (mutationRoute, bool) {
	g.mu.RLock()
	defer g.mu.RUnlock()
	for _, route := range routes[strings.ToUpper(strings.TrimSpace(method))] {
		if mutationPathMatches(route.pattern, path) {
			return route, true
		}
	}
	return mutationRoute{}, false
}

func (g *MutationCoverageGuard) hasMatchingHandler(method string, path string) bool {
	if g.routes == nil {
		return true
	}
	return g.routes.Match(chi.NewRouteContext(), method, path)
}

func contextWithMutationCoverage(ctx context.Context, method string, route mutationRoute, versioned bool) context.Context {
	return context.WithValue(ctx, mutationCoverageContextKey{}, MutationCoverage{
		Method:    strings.ToUpper(strings.TrimSpace(method)),
		Pattern:   normalizeMutationPath(route.pattern),
		Operation: strings.TrimSpace(route.operation),
		Versioned: versioned,
	})
}

func isMutationMethod(method string) bool {
	switch strings.ToUpper(strings.TrimSpace(method)) {
	case http.MethodPost, http.MethodPut, http.MethodPatch, http.MethodDelete:
		return true
	default:
		return false
	}
}

func mutationPathMatches(pattern string, path string) bool {
	patternParts := splitMutationPath(pattern)
	pathParts := splitMutationPath(path)
	for index, patternPart := range patternParts {
		if patternPart == "*" {
			return index == len(patternParts)-1 && len(pathParts) >= index
		}
		if index >= len(pathParts) {
			return false
		}
		if strings.HasPrefix(patternPart, "{") && strings.HasSuffix(patternPart, "}") {
			continue
		}
		if patternPart != pathParts[index] {
			return false
		}
	}
	return len(patternParts) == len(pathParts)
}

func requestMutationPath(r *http.Request) string {
	routeContext := chi.RouteContext(r.Context())
	if routeContext != nil && strings.TrimSpace(routeContext.RoutePath) != "" {
		return routeContext.RoutePath
	}
	return r.URL.Path
}

func splitMutationPath(path string) []string {
	path = normalizeMutationPath(path)
	if path == "" {
		return nil
	}
	return strings.Split(strings.Trim(path, "/"), "/")
}

func normalizeMutationPath(path string) string {
	return strings.TrimSpace(strings.TrimRight(path, "/"))
}
