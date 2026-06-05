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
	"PutSubmodelDescriptorByIdThroughSuperpath":       {},
	"PutSubmodelElementByPathAasRepository":           {},
	"PutSubmodelElementByPathSubmodelRepo":            {},
	"PutThumbnailAasRepository":                       {},
}

var exemptMutationOperations = map[string]struct{}{
	"DeleteAllAssetLinksById":                         {},
	"DeleteSubmodelDescriptorById":                    {},
	"InvokeOperationAasRepository":                    {},
	"InvokeOperationAsync":                            {},
	"InvokeOperationAsyncAasRepository":               {},
	"InvokeOperationAsyncValueOnly":                   {},
	"InvokeOperationAsyncValueOnlyAasRepository":      {},
	"InvokeOperationSubmodelRepo":                     {},
	"InvokeOperationValueOnly":                        {},
	"InvokeOperationValueOnlyAasRepository":           {},
	"PostAllAssetLinksById":                           {},
	"PostSubmodelDescriptor":                          {},
	"PutSubmodelDescriptorById":                       {},
	"QueryAssetAdministrationShellDescriptors":        {},
	"QueryAssetAdministrationShells":                  {},
	"QueryConceptDescriptions":                        {},
	"QuerySubmodels":                                  {},
	"SearchAllAssetAdministrationShellIdsByAssetLink": {},
}

type mutationCoverageContextKey struct{}

// MutationCoverage describes the classification applied to an HTTP mutation.
type MutationCoverage struct {
	Method    string
	Pattern   string
	Versioned bool
}

// MutationCoverageGuard rejects unclassified HTTP mutations while history is active.
type MutationCoverageGuard struct {
	mu      sync.RWMutex
	covered map[string][]string
	exempt  map[string][]string
	routes  chi.Routes
}

// NewMutationCoverageGuard creates an empty versioning route classifier.
func NewMutationCoverageGuard(routes ...chi.Routes) *MutationCoverageGuard {
	guard := &MutationCoverageGuard{
		covered: make(map[string][]string),
		exempt:  make(map[string][]string),
	}
	if len(routes) > 0 {
		guard.routes = routes[0]
	}
	return guard
}

// ClassifyRoute registers the versioning policy for a generated API route.
func (g *MutationCoverageGuard) ClassifyRoute(operation string, method string, pattern string) {
	if !isMutationMethod(method) {
		return
	}
	if _, ok := versionedMutationOperations[operation]; ok {
		g.Cover(method, pattern)
		return
	}
	if _, ok := exemptMutationOperations[operation]; ok {
		g.Exempt(method, pattern)
	}
}

// Cover marks a mutation route as snapshot-producing.
func (g *MutationCoverageGuard) Cover(method string, pattern string) {
	g.addRoute(g.covered, method, pattern)
}

// Exempt marks a mutation route as deliberately not snapshot-producing.
func (g *MutationCoverageGuard) Exempt(method string, pattern string) {
	g.addRoute(g.exempt, method, pattern)
}

// Middleware rejects mutation requests that have no explicit versioning policy.
func (g *MutationCoverageGuard) Middleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if ActiveConfig().Mode == ModeOff || !isMutationMethod(r.Method) {
			next.ServeHTTP(w, r)
			return
		}
		requestPath := requestMutationPath(r)
		if pattern, ok := g.match(g.covered, r.Method, requestPath); ok {
			next.ServeHTTP(w, r.WithContext(contextWithMutationCoverage(r.Context(), r.Method, pattern, true)))
			return
		}
		if pattern, ok := g.match(g.exempt, r.Method, requestPath); ok {
			next.ServeHTTP(w, r.WithContext(contextWithMutationCoverage(r.Context(), r.Method, pattern, false)))
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
func MutationCoverageFromContext(ctx context.Context) (MutationCoverage, bool) {
	if ctx == nil {
		return MutationCoverage{}, false
	}
	coverage, ok := ctx.Value(mutationCoverageContextKey{}).(MutationCoverage)
	return coverage, ok
}

func (g *MutationCoverageGuard) addRoute(routes map[string][]string, method string, pattern string) {
	method = strings.ToUpper(strings.TrimSpace(method))
	pattern = normalizeMutationPath(pattern)
	if !isMutationMethod(method) || pattern == "" {
		return
	}
	g.mu.Lock()
	defer g.mu.Unlock()
	for _, current := range routes[method] {
		if current == pattern {
			return
		}
	}
	routes[method] = append(routes[method], pattern)
}

func (g *MutationCoverageGuard) match(routes map[string][]string, method string, path string) (string, bool) {
	g.mu.RLock()
	defer g.mu.RUnlock()
	for _, pattern := range routes[strings.ToUpper(strings.TrimSpace(method))] {
		if mutationPathMatches(pattern, path) {
			return pattern, true
		}
	}
	return "", false
}

func (g *MutationCoverageGuard) hasMatchingHandler(method string, path string) bool {
	if g.routes == nil {
		return true
	}
	return g.routes.Match(chi.NewRouteContext(), method, path)
}

func contextWithMutationCoverage(ctx context.Context, method string, pattern string, versioned bool) context.Context {
	return context.WithValue(ctx, mutationCoverageContextKey{}, MutationCoverage{
		Method:    strings.ToUpper(strings.TrimSpace(method)),
		Pattern:   normalizeMutationPath(pattern),
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
	if len(patternParts) != len(pathParts) {
		return false
	}
	for index, patternPart := range patternParts {
		if strings.HasPrefix(patternPart, "{") && strings.HasSuffix(patternPart, "}") {
			continue
		}
		if patternPart != pathParts[index] {
			return false
		}
	}
	return true
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
