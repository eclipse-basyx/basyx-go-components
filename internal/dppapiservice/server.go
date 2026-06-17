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
// Author: Jannik Fried ( Fraunhofer IESE ), Aaron Zielstorff ( Fraunhofer IESE )

// Package dppapiservice assembles the Digital Product Passport API HTTP server.
package dppapiservice

import (
	"context"
	"embed"
	"log"
	"net/http"

	aasrepositorydb "github.com/eclipse-basyx/basyx-go-components/internal/aasrepository/persistence"
	"github.com/eclipse-basyx/basyx-go-components/internal/common"
	"github.com/eclipse-basyx/basyx-go-components/internal/common/history"
	auth "github.com/eclipse-basyx/basyx-go-components/internal/common/security"
	submodelrepositorydb "github.com/eclipse-basyx/basyx-go-components/internal/submodelrepository/persistence"
	dppapi "github.com/eclipse-basyx/basyx-go-components/pkg/dppapiservice/go"
	"github.com/go-chi/chi/v5"
)

// NewHTTPHandler assembles the DPP API HTTP handler from configured dependencies.
//
// Parameters:
//   - ctx: Request-independent context used for security setup
//   - cfg: Runtime configuration for routing, security, CORS, and context path
//   - openapiSpec: Embedded OpenAPI specification used for Swagger UI
//   - aasRepo: Asset Administration Shell repository persistence dependency
//   - submodelRepo: Submodel repository persistence dependency
//
// Returns:
//   - http.Handler: Configured root HTTP handler for the DPP API service
//   - error: Setup error if security or router assembly fails
func NewHTTPHandler(ctx context.Context, cfg *common.Config, openapiSpec embed.FS, aasRepo *aasrepositorydb.AssetAdministrationShellDatabase, submodelRepo *submodelrepositorydb.SubmodelDatabase) (http.Handler, error) {
	dppService := dppapi.NewDPPRepositoryService(aasRepo, submodelRepo)
	dppRouter := dppapi.NewDPPRepositoryRouter(dppService)
	contextPath := common.NormalizeBasePath(cfg.Server.ContextPath)

	rootRouter := chi.NewRouter()
	rootRouter.Use(common.ConfigMiddleware(cfg))
	common.AddCors(rootRouter, cfg)
	common.AddHealthEndpoint(rootRouter, cfg)
	if err := common.AddSwaggerUIFromFS(rootRouter, openapiSpec, "openapi.yaml", "Digital Product Passport API", "/swagger", "/api-docs/openapi.yaml", cfg); err != nil {
		log.Printf("Warning: failed to load OpenAPI spec for Swagger UI: %v", err)
	}
	rootRouter.Get(rootRedirectPath(contextPath), func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, swaggerRedirectPath(contextPath), http.StatusFound)
	})

	apiRouter := chi.NewRouter()
	common.ConfigureAPIRouter(apiRouter, "DPPAPIService")
	apiRouter.Use(dppapi.Logger)
	if err := auth.SetupSecurity(ctx, cfg, apiRouter); err != nil {
		return nil, err
	}
	versioningGuard := history.NewMutationCoverageGuard(apiRouter)
	apiRouter.Use(versioningGuard.Middleware)
	apiRouter.Use(history.AuditContextMiddleware(cfg))
	for _, route := range dppRouter.OrderedRoutes() {
		classifyDPPRoute(versioningGuard, route)
		apiRouter.Method(route.Method, route.Pattern, route.HandlerFunc)
	}

	rootRouter.Mount(contextPath, apiRouter)
	return rootRouter, nil
}

// ConfigureHistory applies DPP history settings.
//
// Parameters:
//   - cfg: History configuration values loaded for the DPP API service
func ConfigureHistory(cfg common.HistoryConfig) {
	history.Configure(history.Config{
		Mode:                 cfg.Mode,
		RetentionDays:        cfg.RetentionDays,
		FullSnapshotInterval: cfg.FullSnapshotInterval,
		Immutability:         cfg.Immutability,
		AuditIdentityMode:    cfg.AuditIdentityMode,
	})
}

func classifyDPPRoute(versioningGuard *history.MutationCoverageGuard, route dppapi.Route) {
	switch route.Name {
	case "CreateDPP", "UpdateDPPById", "DeleteDPPById", "UpdateDataElement":
		versioningGuard.Cover(route.Method, route.Pattern)
	case "ReadDPPIdsByProductIds":
		versioningGuard.Exempt(route.Method, route.Pattern)
	default:
		versioningGuard.ClassifyRoute(route.Name, route.Method, route.Pattern)
	}
}

func rootRedirectPath(contextPath string) string {
	if contextPath == "/" {
		return "/"
	}
	return contextPath + "/"
}

func swaggerRedirectPath(contextPath string) string {
	if contextPath == "/" {
		return "/swagger"
	}
	return contextPath + "/swagger"
}
