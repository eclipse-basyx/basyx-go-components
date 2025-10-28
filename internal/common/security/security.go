package auth

import (
	"context"
	"os"

	"github.com/eclipse-basyx/basyx-go-components/internal/common"
	"github.com/go-chi/chi/v5"
)

func SetupSecurity(ctx context.Context, cfg *common.Config, r chi.Router) error {
	if !cfg.ABAC.Enabled {
		return nil
	}

	oidc, err := NewOIDC(ctx, OIDCSettings{
		Issuer:         cfg.OIDC.Issuer,
		Audience:       cfg.OIDC.Audience,
		AllowAnonymous: true,
	})
	if err != nil {
		return err
	}

	var model *AccessModel
	if cfg.ABAC.ModelPath != "" {
		data, err := os.ReadFile(cfg.ABAC.ModelPath)
		if err != nil {
			return err
		}
		m, err := ParseAccessModel(data)
		if err != nil {
			return err
		}
		model = m
	}

	abacSettings := ABACSettings{
		Enabled:             cfg.ABAC.Enabled,
		ClientRolesAudience: cfg.ABAC.ClientRolesAudience,
		Model:               model,
	}

	// ✅ Apply both middlewares to the router
	r.Use(
		oidc.Middleware,
		ABACMiddleware(abacSettings),
	)

	return nil
}
