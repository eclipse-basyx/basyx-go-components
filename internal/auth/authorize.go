package auth

import (
	"context"
	"errors"
	"log"
	"net/http"
	"time"

	"github.com/eclipse-basyx/basyx-go-components/internal/common"
	openapi "github.com/eclipse-basyx/basyx-go-components/pkg/discoveryapi"
)

type ABACSettings struct {
	Enabled             bool
	TenantClaim         string
	EditorRole          string
	ClientRolesAudience string
	RealmAdminRole      string
	Model               *AccessModel
}

type Resource struct {
	Type   string
	Tenant string
	Attrs  map[string]any
}

type Input struct {
	Subject  Claims
	Action   string
	Resource Resource
	Env      Env
}

type Env struct {
	UTCNow time.Time
}

const (
	filterKey ctxKey = "queryFilter"
)

type ResolveResource func(r *http.Request) (Resource, error)

func ABACMiddleware(settings ABACSettings, resolver ResolveResource) func(http.Handler) http.Handler {

	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if !settings.Enabled {
				next.ServeHTTP(w, r)
				return
			}

			claims := FromContext(r)
			if claims == nil {
				http.Error(w, "missing claims context", http.StatusUnauthorized)
				return
			}

			if settings.Model != nil {
				ok, reason, qf := settings.Model.AuthorizeWithFilter(EvalInput{
					Method:    r.Method,
					Path:      r.URL.Path,
					Claims:    claims,
					IssuedUTC: time.Now().UTC(),
				})
				if !ok {
					log.Printf("❌ ABAC(model): %s", reason)

					resp := common.NewErrorResponse(errors.New("access denied"), http.StatusForbidden, "Middleware", "Rules", "Denied")
					openapi.EncodeJSONResponse(resp.Body, &resp.Code, w)
					return
				}

				ctx := r.Context()
				if qf != nil {
					ctx = context.WithValue(ctx, filterKey, qf)
				}
				next.ServeHTTP(w, r.WithContext(ctx))
				return
			}

			http.Error(w, "resource resolution failed", http.StatusForbidden)
		})
	}
}

func FromFilter(r *http.Request) *QueryFilter {
	if v := r.Context().Value(filterKey); v != nil {
		if f, ok := v.(*QueryFilter); ok {
			return f
		}
	}
	return nil
}

func FromFilterCtx(ctx context.Context) *QueryFilter {
	if v := ctx.Value(filterKey); v != nil {
		if f, ok := v.(*QueryFilter); ok {
			return f
		}
	}
	return nil
}

func HasRealmRole(claims Claims, role string) bool {
	ra, ok := claims["realm_access"].(map[string]any)
	if !ok {
		return false
	}
	roles, ok := ra["roles"].([]any)
	if !ok {
		return false
	}
	for _, r := range roles {
		if s, ok := r.(string); ok && s == role {
			return true
		}
	}
	return false
}

func HasClientRole(claims Claims, clientID, role string) bool {
	resAcc, ok := claims["resource_access"].(map[string]any)
	if !ok {
		return false
	}
	client, ok := resAcc[clientID].(map[string]any)
	if !ok {
		return false
	}
	roles, ok := client["roles"].([]any)
	if !ok {
		return false
	}
	for _, r := range roles {
		if s, ok := r.(string); ok && s == role {
			return true
		}
	}
	return false
}

func SimpleResolver(tenantHeaderOrQuery string) ResolveResource {
	return func(r *http.Request) (Resource, error) {
		t := r.URL.Query().Get(tenantHeaderOrQuery)
		if t == "" {
			t = r.Header.Get(tenantHeaderOrQuery)
		}
		if t == "" {
			return Resource{}, errors.New("tenant not found in request")
		}
		return Resource{
			Type:   "AAS",
			Tenant: t,
			Attrs:  map[string]any{},
		}, nil
	}
}
