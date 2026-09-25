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
// Author: Aaron Zielstorff ( Fraunhofer IESE )

package rebac

import (
	"context"
	"database/sql"
	"fmt"
	"log/slog"
	"os"
	"strings"

	"github.com/eclipse-basyx/basyx-go-components/internal/common"
	auth "github.com/eclipse-basyx/basyx-go-components/internal/common/security"
)

// Runtime is the running ReBAC integration of one service.
type Runtime struct {
	Coordinator *Coordinator
}

// Extensions returns the security extensions that install ReBAC. A nil
// runtime returns the zero value, which keeps ABAC-only behavior.
func (r *Runtime) Extensions() auth.SecurityExtensions {
	if r == nil {
		return auth.SecurityExtensions{}
	}
	return auth.SecurityExtensions{ReBAC: r.Coordinator}
}

// Setup starts ReBAC for a service when rebac.enabled is set. It verifies
// that ABAC and OIDC are configured and removes state of resources that
// changed while ReBAC was disabled before enabling decisions.
func Setup(ctx context.Context, cfg *common.Config, db *sql.DB) (*Runtime, error) {
	if cfg == nil || !cfg.ReBAC.Enabled {
		return nil, nil
	}
	if err := validateServiceRequirements(cfg); err != nil {
		return nil, err
	}
	administrators, err := parseAdministrators(cfg.ReBAC.Administrators)
	if err != nil {
		return nil, err
	}
	coordinator := NewCoordinator(db, cfg.ReBAC, administrators)
	report, err := coordinator.ReconcileOrphans(ctx)
	if err != nil {
		return nil, err
	}
	coordinator.MarkReady()
	slog.InfoContext(ctx, "ReBAC enabled",
		"rebac.orphan_grants", report.OrphanGrants, "rebac.orphan_links", report.OrphanLinks,
		"rebac.orphan_invitations", report.OrphanInvitations)
	return &Runtime{Coordinator: coordinator}, nil
}

func validateServiceRequirements(cfg *common.Config) error {
	if !cfg.ABAC.Enabled {
		return fmt.Errorf("REBAC-SETUP-ABACREQUIRED rebac.enabled requires abac.enabled")
	}
	trustlist := strings.TrimSpace(cfg.OIDC.TrustlistPath)
	if trustlist == "" {
		return fmt.Errorf("REBAC-SETUP-OIDCREQUIRED rebac.enabled requires oidc.trustlistPath")
	}
	if _, err := os.Stat(trustlist); err != nil {
		return fmt.Errorf("REBAC-SETUP-OIDCREQUIRED OIDC trustlist not readable: %w", err)
	}
	return nil
}

func parseAdministrators(entries []string) ([]common.ReBACAdministrator, error) {
	administrators := make([]common.ReBACAdministrator, 0, len(entries))
	for _, entry := range entries {
		administrator, err := common.ParseReBACAdministrator(entry)
		if err != nil {
			return nil, err
		}
		administrators = append(administrators, administrator)
	}
	return administrators, nil
}
