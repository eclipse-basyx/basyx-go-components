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
	"fmt"
	"time"

	"github.com/doug-martin/goqu/v9"
	"github.com/eclipse-basyx/basyx-go-components/internal/common/events"
)

// Runtime owns one pinned authorization projection for a deployment scope.
type Runtime struct {
	State            *StateStore
	Client           *Client
	ValidateMutation func(context.Context, *sql.Tx, events.Mutation, string) error
	AuditMutation    func(context.Context, *sql.Tx, string, string, int64) error
	AfterMutation    func(context.Context, *sql.Tx, string, string, bool) error
	Creator          func(context.Context, string, string, string) string
}

// NewRuntime validates the pinned model and the shared configuration before use.
func NewRuntime(ctx context.Context, db *sql.DB, scope, fingerprint string, cfg Config) (*Runtime, error) {
	client, err := NewClient(cfg)
	if err != nil {
		return nil, err
	}
	if err = client.ReadModel(ctx); err != nil {
		return nil, err
	}
	state := &StateStore{DB: db, Scope: scope}
	if err = state.Initialize(ctx, fingerprint); err != nil {
		return nil, err
	}
	return &Runtime{State: state, Client: client}, nil
}

// Run retries durable projection work until service shutdown.
func (r *Runtime) Run(ctx context.Context, report func(error)) {
	go r.State.MonitorProjection(ctx, r.Client, report)
	r.State.RunProjection(ctx, r.Client, 100*time.Millisecond, report)
}

// Ready rejects traffic while any relationship revision is incomplete.
func (r *Runtime) Ready(ctx context.Context) error {
	return r.State.WithApplied(ctx, func(*sql.Tx) error { return nil })
}

// Resources returns a stable candidate batch without any ABAC prefilter.
func (s *StateStore) Resources(ctx context.Context, tx *sql.Tx, kind, after string, limit uint) ([]StoredResource, error) {
	if limit == 0 || limit > 1000 {
		return nil, fmt.Errorf("REBAC-STATE-LIMIT invalid candidate batch size")
	}
	query := stateDialect.From("rebac_resource").Select("resource_uuid", "kind", "identifier").Where(goqu.Ex{"scope": s.Scope, "deleted_at": nil}).Where(goqu.C("resource_uuid").Cast("text").Gt(after)).Order(goqu.C("resource_uuid").Cast("text").Asc()).Limit(limit)
	if kind != "" {
		query = query.Where(goqu.C("kind").Eq(kind))
	}
	statement, args, err := query.Prepared(true).ToSQL()
	if err != nil {
		return nil, fmt.Errorf("REBAC-STATE-LISTSQL: %w", err)
	}
	rows, err := tx.QueryContext(ctx, statement, args...)
	if err != nil {
		return nil, fmt.Errorf("REBAC-STATE-LIST: %w", err)
	}
	defer func() { _ = rows.Close() }()
	result := []StoredResource{}
	for rows.Next() {
		var item StoredResource
		if err = rows.Scan(&item.UUID, &item.Kind, &item.Identifier); err != nil {
			return nil, fmt.Errorf("REBAC-STATE-LISTSCAN: %w", err)
		}
		result = append(result, item)
	}
	return result, rows.Err()
}

// ResourcesByObjectKeys resolves OpenFGA object identifiers without scanning
// the complete resource catalog. Callers must supply keys for one object type.
func (s *StateStore) ResourcesByObjectKeys(ctx context.Context, tx *sql.Tx, kind string, objectKeys []string) ([]StoredResource, error) {
	if len(objectKeys) == 0 {
		return []StoredResource{}, nil
	}
	result := make([]StoredResource, 0, len(objectKeys))
	for start := 0; start < len(objectKeys); start += 1000 {
		end := min(start+1000, len(objectKeys))
		query := stateDialect.From("rebac_resource").
			Select("resource_uuid", "kind", "identifier").
			Where(goqu.Ex{"scope": s.Scope, "kind": kind, "deleted_at": nil}).
			Where(goqu.C("object_key").In(objectKeys[start:end])).
			Order(goqu.C("identifier").Asc())
		statement, args, err := query.Prepared(true).ToSQL()
		if err != nil {
			return nil, fmt.Errorf("REBAC-STATE-OBJECTKEYSSQL: %w", err)
		}
		rows, err := tx.QueryContext(ctx, statement, args...)
		if err != nil {
			return nil, fmt.Errorf("REBAC-STATE-OBJECTKEYS: %w", err)
		}
		for rows.Next() {
			var resource StoredResource
			if err = rows.Scan(&resource.UUID, &resource.Kind, &resource.Identifier); err != nil {
				_ = rows.Close()
				return nil, fmt.Errorf("REBAC-STATE-OBJECTKEYSSCAN: %w", err)
			}
			result = append(result, resource)
		}
		if err = rows.Err(); err != nil {
			_ = rows.Close()
			return nil, fmt.Errorf("REBAC-STATE-OBJECTKEYSROWS: %w", err)
		}
		_ = rows.Close()
	}
	return result, nil
}

// CheckResources batches concrete resource checks while the caller holds the barrier.
func (r *Runtime) CheckResources(ctx context.Context, actor GrantActor, resources []StoredResource, relation string) ([]bool, error) {
	allowed := make([]bool, len(resources))
	for start := 0; start < len(resources); start += 50 {
		end := min(start+50, len(resources))
		checks := make([]BatchCheckRequest, 0, end-start)
		for i := start; i < end; i++ {
			object, err := ResourceObject(r.State.Scope, resources[i].Kind, resources[i].UUID)
			if err != nil {
				return nil, err
			}
			checks = append(checks, BatchCheckRequest{CorrelationID: fmt.Sprint(i), Tuple: Tuple{User: actor.User, Relation: relation, Object: object}, ContextualTuples: actor.Groups})
		}
		results, err := r.Client.BatchCheck(ctx, checks)
		if err != nil {
			return nil, err
		}
		for i, result := range results {
			allowed[start+i] = result.Allowed
		}
	}
	return allowed, nil
}
