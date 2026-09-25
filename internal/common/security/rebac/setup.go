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
	"time"

	"github.com/eclipse-basyx/basyx-go-components/internal/common"
	auth "github.com/eclipse-basyx/basyx-go-components/internal/common/security"
)

// Runtime is the running ReBAC integration of one service.
type Runtime struct {
	Coordinator *Coordinator
	Projector   *Projector
	Activation  Activation
}

// Extensions returns the security extensions that install ReBAC. A nil
// runtime returns the zero value, which keeps ABAC-only behavior.
func (r *Runtime) Extensions() auth.SecurityExtensions {
	if r == nil {
		return auth.SecurityExtensions{}
	}
	return auth.SecurityExtensions{ReBAC: r.Coordinator}
}

// Setup starts ReBAC for a service when rebac.enabled is set. It verifies the
// scope binding and the pinned model, removes desired state of resources
// deleted while ReBAC was disabled, repairs drift and starts the projector.
// Any failure aborts startup; ReBAC decisions are enabled only afterwards.
func Setup(ctx context.Context, cfg *common.Config, db *sql.DB, serviceName string) (*Runtime, error) {
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
	activation, err := resolveActivation(ctx, cfg.ReBAC, db, serviceName)
	if err != nil {
		return nil, err
	}
	client, err := NewOpenFGAClient(cfg.ReBAC.OpenFGA, activation.StoreID, activation.ModelID)
	if err != nil {
		return nil, err
	}
	if err = verifyModel(ctx, client, activation); err != nil {
		return nil, err
	}
	if err = EnsureScopeState(ctx, db, cfg.ReBAC.Scope); err != nil {
		return nil, err
	}
	projector := NewProjector(db, client, cfg.ReBAC.Scope)
	coordinator := NewCoordinator(CoordinatorOptions{
		DB: db, Client: client, Projector: projector, Config: cfg.ReBAC, Administrators: administrators,
	})
	go projector.Run(ctx)
	if err = coordinator.reconcileOnStartup(ctx); err != nil {
		return nil, err
	}
	coordinator.MarkReady()
	slog.InfoContext(ctx, "ReBAC enabled",
		"rebac.scope", activation.Scope, "rebac.store_id", activation.StoreID, "rebac.model_id", activation.ModelID)
	return &Runtime{Coordinator: coordinator, Projector: projector, Activation: activation}, nil
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

// resolveActivation returns the store and model the database is bound to.
// An unbound database is bound to explicitly configured IDs.
func resolveActivation(ctx context.Context, cfg common.ReBACConfig, db *sql.DB, serviceName string) (Activation, error) {
	activation, bound, err := ReadActivation(ctx, db)
	if err != nil {
		return Activation{}, err
	}
	if !bound {
		if cfg.OpenFGA.StoreID == "" || cfg.OpenFGA.AuthorizationModelID == "" {
			return Activation{}, fmt.Errorf("REBAC-SETUP-UNBOUND no model is provisioned for scope %q; run basyxconfigurationservice with rebac.provisionModel=true or configure rebac.openfga.storeId and authorizationModelId", cfg.Scope)
		}
		hash, hashErr := EmbeddedModelHash()
		if hashErr != nil {
			return Activation{}, hashErr
		}
		if err = InsertActivation(ctx, db, Activation{
			Scope: cfg.Scope, StoreID: cfg.OpenFGA.StoreID, ModelID: cfg.OpenFGA.AuthorizationModelID,
			ModelHash: hash, ActivatedBy: "service:" + serviceName,
		}); err != nil {
			return Activation{}, err
		}
		if activation, _, err = ReadActivation(ctx, db); err != nil {
			return Activation{}, err
		}
	}
	return activation, checkActivation(cfg, activation)
}

func checkActivation(cfg common.ReBACConfig, activation Activation) error {
	if activation.Scope != cfg.Scope {
		return fmt.Errorf("REBAC-SETUP-SCOPEMISMATCH database is bound to scope %q, not %q", activation.Scope, cfg.Scope)
	}
	if cfg.OpenFGA.StoreID != "" && cfg.OpenFGA.StoreID != activation.StoreID {
		return fmt.Errorf("REBAC-SETUP-STOREMISMATCH scope %q is bound to store %q", activation.Scope, activation.StoreID)
	}
	if cfg.OpenFGA.AuthorizationModelID != "" && cfg.OpenFGA.AuthorizationModelID != activation.ModelID {
		return fmt.Errorf("REBAC-SETUP-MODELMISMATCH scope %q is bound to model %q", activation.Scope, activation.ModelID)
	}
	return nil
}

// verifyModel ensures the pinned model exists and equals the model of this
// release, so the service never evaluates relations it does not understand.
func verifyModel(ctx context.Context, client Client, activation Activation) error {
	stored, err := client.ReadModel(ctx, activation.ModelID)
	if err != nil {
		return fmt.Errorf("REBAC-SETUP-MODELUNREACHABLE: %w", err)
	}
	storedHash, err := ModelContentHash(stored)
	if err != nil {
		return err
	}
	embeddedHash, err := EmbeddedModelHash()
	if err != nil {
		return err
	}
	if storedHash != embeddedHash || activation.ModelHash != embeddedHash {
		return fmt.Errorf("REBAC-SETUP-MODELHASH model %q does not match the model of this release", activation.ModelID)
	}
	return nil
}

func (c *Coordinator) reconcileOnStartup(ctx context.Context) error {
	enabledAt := time.Now()
	orphans, err := c.ReconcileOrphans(ctx)
	if err != nil {
		return err
	}
	drift, err := c.RepairDrift(ctx)
	if err != nil {
		return err
	}
	if err = MarkScopeReconciled(ctx, c.db, c.scope, enabledAt); err != nil {
		return err
	}
	slog.InfoContext(ctx, "ReBAC startup reconciliation completed",
		"rebac.orphan_grants", orphans.OrphanGrants, "rebac.orphan_links", orphans.OrphanLinks,
		"rebac.orphan_invitations", orphans.OrphanInvitations,
		"rebac.missing_tuples", drift.MissingTuples, "rebac.unexpected_tuples", drift.UnexpectedTuples)
	return nil
}
