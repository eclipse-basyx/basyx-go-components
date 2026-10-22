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

package aol

import (
	"context"
	"encoding/json"
	"sync"
	"testing"
	"time"

	"github.com/eclipse-basyx/basyx-go-components/internal/common"
	"github.com/stretchr/testify/require"
)

type memoryStore struct {
	mu    sync.Mutex
	plans map[string][]byte
}

func (s *memoryStore) Create(_ context.Context, p *Plan) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	p.Version = 1
	raw, _ := json.Marshal(planDocument{Plan: storedPlan(*p)})
	s.plans[p.ID] = raw
	return nil
}
func (s *memoryStore) Get(_ context.Context, id string) (*Plan, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	var doc planDocument
	if err := json.Unmarshal(s.plans[id], &doc); err != nil {
		return nil, err
	}
	p := Plan(doc.Plan)
	p.ApprovalHash = doc.ApprovalHash
	p.ApprovalAction = doc.ApprovalAction
	p.ApprovalExpires = doc.ApprovalExpires
	return &p, nil
}
func (s *memoryStore) Update(_ context.Context, p *Plan, version int64) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	var current planDocument
	_ = json.Unmarshal(s.plans[p.ID], &current)
	if current.Plan.Version != version {
		return Failure("AOL-PLAN-CONFLICT", "conflict", "Reload.")
	}
	p.Version = version + 1
	p.Updated = time.Now()
	raw, _ := encodePlan(p)
	s.plans[p.ID] = []byte(raw)
	return nil
}
func (s *memoryStore) Receipts(_ context.Context, _ string) ([]Step, error) { return nil, nil }
func TestApprovalIsBoundToSubjectActionPayloadAndConsumedOnce(t *testing.T) {
	store := &memoryStore{plans: map[string][]byte{}}
	operations := Operations{Store: store, TTL: time.Minute}
	ctx := ContextWithIdentity(t.Context(), Identity{Subject: "alice", Issuer: "issuer"})
	p := &Plan{ID: "plan", Subject: "alice", Issuer: "issuer", Hash: "payload", Status: "planned", Expires: time.Now().Add(time.Minute)}
	p.Hash, _ = common.CanonicalJSONHash(p.Draft)
	require.NoError(t, store.Create(ctx, p))
	token, err := operations.Approve(ctx, "plan", "apply")
	require.NoError(t, err)
	require.NotEmpty(t, token)
	other := ContextWithIdentity(t.Context(), Identity{Subject: "bob", Issuer: "issuer"})
	_, err = operations.consume(other, "plan", token, "apply")
	require.Error(t, err)
	_, err = operations.consume(ctx, "plan", token, "rollback")
	require.Error(t, err)
	_, err = operations.consume(ctx, "plan", "forged", "apply")
	require.Error(t, err)
	_, err = operations.consume(ctx, "plan", token, "apply")
	require.NoError(t, err)
	_, err = operations.consume(ctx, "plan", token, "apply")
	require.Error(t, err)
}
func TestExpiredApprovalCannotExecute(t *testing.T) {
	store := &memoryStore{plans: map[string][]byte{}}
	ctx := ContextWithIdentity(t.Context(), Identity{Subject: "alice", Issuer: "issuer"})
	p := &Plan{ID: "expired", Subject: "alice", Issuer: "issuer", Status: "planned", Expires: time.Now().Add(-time.Minute)}
	require.NoError(t, store.Create(ctx, p))
	_, err := (Operations{Store: store, TTL: time.Minute}).Approve(ctx, "expired", "apply")
	require.Error(t, err)
}
