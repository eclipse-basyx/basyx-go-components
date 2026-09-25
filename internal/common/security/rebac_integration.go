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

package auth

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"github.com/doug-martin/goqu/v9"

	"github.com/eclipse-basyx/basyx-go-components/internal/common"
	"github.com/eclipse-basyx/basyx-go-components/internal/common/events"
	"github.com/eclipse-basyx/basyx-go-components/internal/common/rebac"
)

// ContextWithReBACEmbeddedSource marks the exact embedding generated from a local Submodel.
func ContextWithReBACEmbeddedSource(ctx context.Context, aasID, submodelID string) context.Context {
	return ContextWithReBACDerivedTarget(ctx, "submodel", submodelID, "embedded_submodel_descriptor", rebac.EmbeddedDescriptorIdentifier(aasID, submodelID))
}

type rebacDerivedKey struct{}
type rebacDerivedTarget struct{ sourceKind, sourceID, targetKind, targetID string }

// ContextWithReBACDerivedTarget scopes one internal integration write to its authorized source.
func ContextWithReBACDerivedTarget(ctx context.Context, sourceKind, sourceID, targetKind, targetID string) context.Context {
	targets, _ := ctx.Value(rebacDerivedKey{}).([]rebacDerivedTarget)
	copied := append([]rebacDerivedTarget(nil), targets...)
	copied = append(copied, rebacDerivedTarget{sourceKind, sourceID, targetKind, targetID})
	return context.WithValue(ctx, rebacDerivedKey{}, copied)
}

// NotifyReBACMutation joins discovery and package mutations to their existing writer transaction.
func NotifyReBACMutation(ctx context.Context, tx *sql.Tx, kind, id string, deleted bool) error {
	cfg, ok := common.ConfigFromContext(ctx)
	if !ok || !cfg.ReBAC.Enabled {
		return nil
	}
	request, ok := ctx.Value(rebacRequestKey{}).(*rebacRequest)
	if !ok || request.security == nil {
		return fmt.Errorf("REBAC-MUTATION-RUNTIME validated authorization context required")
	}
	change := "Created"
	if deleted {
		change = "Deleted"
	}
	table := kind + "_history"
	if kind == "aas_descriptor" {
		table = "descriptor_history"
	}
	return request.security.runtime.HandleMutation(ctx, tx, events.Mutation{Table: table, Identifier: id, ChangeType: change, Deleted: deleted})
}

func (s *rebacSecurity) authorizeDerivedMutation(ctx context.Context, tx *sql.Tx, kind, id string) (bool, error) {
	targets, _ := ctx.Value(rebacDerivedKey{}).([]rebacDerivedTarget)
	for i := len(targets) - 1; i >= 0; i-- {
		target := targets[i]
		if target.targetKind != kind || target.targetID != id {
			continue
		}
		if err := s.validateDerivedSource(ctx, tx, target); err != nil {
			return false, err
		}
		return true, nil
	}
	return false, nil
}

func (s *rebacSecurity) validateDerivedSource(ctx context.Context, _ *sql.Tx, target rebacDerivedTarget) error {
	request, _ := ctx.Value(rebacRequestKey{}).(*rebacRequest)
	if request == nil {
		return rebac.ErrForbidden
	}
	kind, id := string(request.route.Kind), request.route.Identifier
	if kind == "element" && request.route.Parent != nil {
		kind = "submodel"
		id = request.route.Parent.Identifier
	}
	if target.sourceKind == kind && (id == target.sourceID || request.route.Collection) {
		return nil
	}
	targets, _ := ctx.Value(rebacDerivedKey{}).([]rebacDerivedTarget)
	for _, prior := range targets {
		if prior.targetKind == target.sourceKind && prior.targetID == target.sourceID && prior.sourceKind == kind && (prior.sourceID == id || request.route.Collection) {
			return nil
		}
	}
	return fmt.Errorf("REBAC-INTEGRATION-SOURCE source was not authorized by this request")
}

func (s *rebacSecurity) syncDerivedRelationships(ctx context.Context, tx *sql.Tx, kind, id string, deleted bool) error {
	if kind == "aas" && !deleted {
		service := rebac.InheritanceService{State: s.runtime.State, Checker: s.runtime.Client}
		if _, err := service.RemoveUnreferencedInheritance(ctx, tx, id); err != nil {
			return err
		}
	}
	if deleted {
		return nil
	}
	targets, _ := ctx.Value(rebacDerivedKey{}).([]rebacDerivedTarget)
	for _, target := range targets {
		if err := s.syncDerivedTarget(ctx, tx, target, kind, id); err != nil {
			return err
		}
	}
	return nil
}

func (s *rebacSecurity) derivedIntegration(target rebacDerivedTarget) (string, bool) {
	switch {
	case target.sourceKind == "submodel" && target.targetKind == "embedded_submodel_descriptor":
		return "aas_registry", s.cfg.General.AASRegistryIntegration
	case target.sourceKind == "aas" && target.targetKind == "aas_descriptor":
		return "aas_registry", s.cfg.General.AASRegistryIntegration
	case target.sourceKind == "submodel" && target.targetKind == "submodel_descriptor":
		return "submodel_registry", s.cfg.General.SubmodelRegistryIntegration
	case target.sourceKind == "aas_descriptor" && target.targetKind == "discovery":
		return "discovery", s.cfg.General.DiscoveryIntegration
	default:
		return "", false
	}
}

func (s *rebacSecurity) persistDerivedRelationship(ctx context.Context, tx *sql.Tx, target rebacDerivedTarget, integration string) error {
	source, err := s.runtime.State.Resource(ctx, tx, target.sourceKind, target.sourceID)
	if errors.Is(err, sql.ErrNoRows) {
		user := ""
		request, _ := ctx.Value(rebacRequestKey{}).(*rebacRequest)
		if request != nil && request.identity.Principal != nil {
			user = request.actor.User
		}
		var initial []rebac.Tuple
		user = s.resourceCreator(ctx, target.sourceKind, target.sourceID, user)
		source, initial, err = s.runtime.State.EnsureResource(ctx, tx, target.sourceKind, target.sourceID, "", user)
		if err == nil {
			_, err = s.runtime.State.QueueChanged(ctx, tx, initial, nil)
		}
	}
	if err != nil {
		return err
	}
	resource, err := s.runtime.State.Resource(ctx, tx, target.targetKind, target.targetID)
	if err != nil {
		return err
	}
	sourceObject, err := rebac.ResourceObject(s.cfg.ReBAC.Scope, source.Kind, source.UUID)
	if err != nil {
		return err
	}
	object, err := rebac.ResourceObject(s.cfg.ReBAC.Scope, resource.Kind, resource.UUID)
	if err != nil {
		return err
	}
	query, args, err := goqu.Dialect("postgres").Insert("rebac_generated").Rows(goqu.Record{"scope": s.cfg.ReBAC.Scope, "source_uuid": source.UUID, "target_uuid": resource.UUID, "integration": integration}).OnConflict(goqu.DoNothing()).Prepared(true).ToSQL()
	if err != nil {
		return fmt.Errorf("REBAC-INTEGRATION-PROVENANCESQL: %w", err)
	}
	if _, err = tx.ExecContext(ctx, query, args...); err != nil {
		return fmt.Errorf("REBAC-INTEGRATION-PROVENANCE: %w", err)
	}
	_, err = s.runtime.State.QueueChanged(ctx, tx, []rebac.Tuple{{User: sourceObject, Relation: "source", Object: object}}, nil)
	return err
}

func (s *rebacSecurity) resourceCreator(ctx context.Context, kind, id, user string) string {
	targets, _ := ctx.Value(rebacDerivedKey{}).([]rebacDerivedTarget)
	for _, target := range targets {
		if target.targetKind == kind && target.targetID == id {
			if _, enabled := s.derivedIntegration(target); enabled {
				return ""
			}
		}
	}
	return user
}

func (s *rebacSecurity) persistEmbeddedSource(ctx context.Context, tx *sql.Tx, target rebacDerivedTarget, aasID string) error {
	if !s.cfg.General.AASRegistryIntegration || target.targetID != rebac.EmbeddedDescriptorIdentifier(aasID, target.sourceID) {
		return nil
	}
	// Remote references have no local authorization source.
	if _, err := s.runtime.State.Resource(ctx, tx, "submodel", target.sourceID); errors.Is(err, sql.ErrNoRows) {
		return nil
	} else if err != nil {
		return err
	}
	if _, err := s.runtime.State.Resource(ctx, tx, target.targetKind, target.targetID); errors.Is(err, sql.ErrNoRows) {
		return nil
	} else if err != nil {
		return err
	}
	return s.persistDerivedRelationship(ctx, tx, target, "aas_registry")
}

func (s *rebacSecurity) syncDerivedTarget(ctx context.Context, tx *sql.Tx, target rebacDerivedTarget, kind, id string) error {
	if target.targetKind == "embedded_submodel_descriptor" && kind == "aas_descriptor" {
		return s.persistEmbeddedSource(ctx, tx, target, id)
	}
	if target.targetKind != kind || target.targetID != id {
		return nil
	}
	integration, enabled := s.derivedIntegration(target)
	if !enabled {
		return nil
	}
	return s.persistDerivedRelationship(ctx, tx, target, integration)
}
