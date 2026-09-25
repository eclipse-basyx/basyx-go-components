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

package rebac

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"github.com/doug-martin/goqu/v9"
	"github.com/eclipse-basyx/basyx-go-components/internal/common"
	"github.com/eclipse-basyx/basyx-go-components/internal/common/events"
)

var mutationKinds = map[string]string{"discovery_history": "discovery", "package_history": "package", "aas_history": "aas", "submodel_history": "submodel", "concept_description_history": "concept_description", "descriptor_history": "aas_descriptor", "submodel_descriptor_history": "submodel_descriptor"}

// HandleMutation joins authorization state to the resource writer's transaction.
func (r *Runtime) HandleMutation(ctx context.Context, tx *sql.Tx, mutation events.Mutation) (err error) {
	defer func() { err = mutationError(err) }()
	kind, covered := mutationKinds[mutation.Table]
	if !covered {
		return nil
	}
	actor, ok := LifecycleActorFromContext(ctx)
	if !ok {
		return fmt.Errorf("REBAC-MUTATION-IDENTITY validated initiating context is required")
	}
	if err := r.State.CheckMutationRevision(ctx, tx, actor.Revision); err != nil {
		return err
	}
	if r.ValidateMutation == nil {
		return fmt.Errorf("REBAC-MUTATION-VALIDATOR resource authorization is required")
	}
	if err := r.ValidateMutation(ctx, tx, mutation, kind); err != nil {
		return err
	}
	creator := actor.User
	if r.Creator != nil {
		creator = r.Creator(ctx, kind, mutation.Identifier, creator)
	}
	writes, deletes, err := r.mutationRelationships(ctx, tx, mutation, kind, creator)
	if err != nil {
		return err
	}
	revision, err := r.State.QueueChanged(ctx, tx, writes, deletes)
	if err != nil {
		return err
	}
	if r.AfterMutation != nil {
		if err = r.AfterMutation(ctx, tx, kind, mutation.Identifier, mutation.Deleted); err != nil {
			return err
		}
	}
	if r.AuditMutation != nil {
		return r.AuditMutation(ctx, tx, kind, mutation.Identifier, revision)
	}
	return nil
}

func (r *Runtime) mutationRelationships(ctx context.Context, tx *sql.Tx, mutation events.Mutation, kind, user string) ([]Tuple, []Tuple, error) {
	if mutation.Deleted {
		return r.retireMutationResource(ctx, tx, kind, mutation.Identifier)
	}

	rootCreator := user
	if mutation.ChangeType != "Created" {
		rootCreator = ""
	}
	resource, writes, err := r.State.EnsureResource(ctx, tx, kind, mutation.Identifier, "", rootCreator)
	if err != nil {
		return nil, nil, err
	}
	if kind == "aas_descriptor" {
		children, deletes, err := r.SyncEmbeddedDescriptors(ctx, tx, resource, user, false)
		return append(writes, children...), deletes, err
	}
	if kind != "submodel" {
		return writes, nil, nil
	}
	children, deletes, err := r.SyncElements(ctx, tx, resource, user)
	return append(writes, children...), deletes, err
}

// QueueChanged avoids a barrier for ordinary value changes with unchanged relationships.
func (s *StateStore) QueueChanged(ctx context.Context, tx *sql.Tx, writes, deletes []Tuple) (int64, error) {
	objects := map[string]bool{}
	for _, t := range writes {
		objects[t.Object] = true
	}
	for _, t := range deletes {
		objects[t.Object] = true
	}
	existing := []Tuple{}
	for object := range objects {
		tuples, err := s.Relationships(ctx, tx, object, false)
		if err != nil {
			return 0, err
		}
		existing = append(existing, tuples...)
	}
	writes = tupleDifference(writes, existing)
	absent := tupleDifference(deletes, existing)
	deletes = tupleDifference(deletes, absent)
	if len(writes)+len(deletes) == 0 {
		state, err := s.Lock(ctx, tx, false)
		return state.Desired, err
	}
	return s.Enqueue(ctx, tx, writes, deletes)
}

// SavePrincipal retains displayable identity mapping without saving token contents.
func (s *StateStore) SavePrincipal(ctx context.Context, tx *sql.Tx, p Principal) error {
	subject, err := p.Object()
	if err != nil {
		return err
	}
	return executeState(ctx, tx, stateDialect.Insert("rebac_principal").Rows(goqu.Record{"scope": s.Scope, "subject": subject, "kind": p.Kind, "issuer": p.Issuer, "identifier": p.ID}).OnConflict(goqu.DoNothing()))
}

func mutationError(err error) error {
	if err == nil {
		return nil
	}
	if errors.Is(err, ErrForbidden) {
		return common.NewErrDenied(err.Error())
	}
	return common.NewErrServiceUnavailable(err.Error())
}

func (r *Runtime) retireMutationResource(ctx context.Context, tx *sql.Tx, kind, id string) ([]Tuple, []Tuple, error) {
	resource, err := r.State.Resource(ctx, tx, kind, id)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil, nil
	}
	if err != nil {
		return nil, nil, err
	}
	deletes, err := r.State.RetireResource(ctx, tx, resource)
	if err != nil {
		return nil, nil, err
	}
	switch kind {
	case "aas_descriptor":
		_, children, err := r.SyncEmbeddedDescriptors(ctx, tx, resource, "", true)
		return nil, append(deletes, children...), err
	case "submodel":
		children, err := r.retireAbsentElements(ctx, tx, id, nil)
		return nil, append(deletes, children...), err
	default:
		return nil, deletes, nil
	}
}
