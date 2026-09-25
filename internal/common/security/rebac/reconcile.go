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
	"hash/fnv"
	"math"
	"slices"
	"strings"

	"github.com/doug-martin/goqu/v9"
	"github.com/doug-martin/goqu/v9/exp"
)

// ReconcileReport summarizes one reconciliation run.
type ReconcileReport struct {
	OrphanGrants      int `json:"orphanGrants"`
	OrphanLinks       int `json:"orphanLinks"`
	OrphanInvitations int `json:"orphanInvitations"`
	MissingTuples     int `json:"missingTuples"`
	UnexpectedTuples  int `json:"unexpectedTuples"`
}

// storedTupleTypes are the object types BaSyx owns in OpenFGA. Group
// memberships are only ever sent as contextual tuples, so stored ones are drift.
var storedTupleTypes = []string{TypeGroup, TypeRepository, TypeAAS, TypeSubmodel, TypeElement, TypeConceptDescription}

func advisoryKey(name string) int64 {
	hash := fnv.New64a()
	_, _ = hash.Write([]byte(name))
	return int64(hash.Sum64() & math.MaxInt64) //nolint:gosec // masked to the positive int64 range
}

// ReconcileOrphans removes desired state of resources that were deleted or
// unreferenced while ReBAC was disabled. Their tuple deletions go through the
// outbox; removed links engage the barrier until applied.
func (c *Coordinator) ReconcileOrphans(ctx context.Context) (ReconcileReport, error) {
	report := ReconcileReport{}
	err := c.inReconcileTx(ctx, func(tx *sql.Tx) error {
		grants, err := deleteOrphanGrants(ctx, tx)
		if err != nil {
			return err
		}
		links, err := DeleteSubmodelLinks(ctx, tx, goqu.L("NOT EXISTS (?)", submodelReferenceExists("rebac_submodel_link")))
		if err != nil {
			return err
		}
		invitations, err := deleteOrphanInvitations(ctx, tx)
		if err != nil {
			return err
		}
		report.OrphanGrants, report.OrphanLinks, report.OrphanInvitations = len(grants), len(links), invitations
		return c.enqueueOrphanRemovals(ctx, tx, grants, links)
	})
	return report, err
}

func (c *Coordinator) inReconcileTx(ctx context.Context, fn func(tx *sql.Tx) error) (err error) {
	tx, err := c.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("REBAC-RECONCILE-BEGIN: %w", err)
	}
	defer func() {
		if err != nil {
			_ = tx.Rollback()
		}
	}()
	lock := dialect.Select(goqu.Func("pg_advisory_xact_lock", advisoryKey("basyx-rebac-reconcile:"+c.scope))).Prepared(true)
	if _, err = execDataset(ctx, tx, "REBAC-RECONCILE-LOCK", lock); err != nil {
		return err
	}
	if err = fn(tx); err != nil {
		return err
	}
	if err = tx.Commit(); err != nil {
		return fmt.Errorf("REBAC-RECONCILE-COMMIT: %w", err)
	}
	c.projector.Notify()
	return nil
}

func (c *Coordinator) enqueueOrphanRemovals(ctx context.Context, tx *sql.Tx, grants []Grant, links []SubmodelLink) error {
	operations := make([]OutboxOperation, 0, len(grants))
	for _, grant := range grants {
		operations = append(operations, OutboxOperation{Delete: true, Tuple: grant.Tuple()})
	}
	if _, err := EnqueueOutbox(ctx, tx, c.scope, operations, OutboxPending); err != nil {
		return err
	}
	revocations := make([]OutboxOperation, 0, len(links))
	for _, link := range links {
		revocations = append(revocations, OutboxOperation{Delete: true, Tuple: link.Tuple()})
	}
	_, err := EnqueueOutbox(ctx, tx, c.scope, revocations, OutboxRevocation)
	return err
}

// resourceExists matches rows of table whose auth_uuid equals uuidColumn.
func resourceExists(table string, uuidColumn exp.IdentifierExpression) *goqu.SelectDataset {
	alias := "existing_" + table
	return dialect.From(goqu.T(table).As(alias)).Select(goqu.L("1")).
		Where(goqu.I(alias + ".auth_uuid").Eq(uuidColumn))
}

func orphanResourceCondition(typeColumn exp.IdentifierExpression, uuidColumn exp.IdentifierExpression) exp.Expression {
	return goqu.Or(
		goqu.And(typeColumn.Eq(TypeAAS), goqu.L("NOT EXISTS (?)", resourceExists("aas", uuidColumn))),
		goqu.And(typeColumn.In(TypeSubmodel, TypeElement), goqu.L("NOT EXISTS (?)", resourceExists("submodel", uuidColumn))),
		goqu.And(typeColumn.Eq(TypeConceptDescription), goqu.L("NOT EXISTS (?)", resourceExists("concept_description", uuidColumn))),
	)
}

func deleteOrphanGrants(ctx context.Context, tx *sql.Tx) ([]Grant, error) {
	ds := dialect.Delete(grantTable).
		Where(orphanResourceCondition(goqu.I(grantTable+".object_type"), goqu.I(grantTable+".object_uuid"))).
		Returning(grantColumns()...).Prepared(true)
	query, args, err := ds.ToSQL()
	if err != nil {
		return nil, fmt.Errorf("REBAC-DELETEORPHANGRANTS-BUILDQ: %w", err)
	}
	rows, err := tx.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("REBAC-DELETEORPHANGRANTS-EXECQ: %w", err)
	}
	return scanGrants(rows)
}

func deleteOrphanInvitations(ctx context.Context, tx *sql.Tx) (int, error) {
	ds := dialect.Delete(invitationTable).
		Where(orphanResourceCondition(goqu.I(invitationTable+".object_type"), goqu.I(invitationTable+".object_uuid"))).
		Prepared(true)
	result, err := execDataset(ctx, tx, "REBAC-DELETEORPHANINVITATIONS", ds)
	if err != nil {
		return 0, err
	}
	affected, err := result.RowsAffected()
	return int(affected), err
}

// submodelReferenceExists matches when the AAS of linkTable still references
// the Submodel of linkTable.
func submodelReferenceExists(linkTable string) *goqu.SelectDataset {
	return dialect.From(goqu.T("aas").As("ref_aas")).
		InnerJoin(goqu.T("aas_submodel_reference").As("ref"), goqu.On(goqu.I("ref.aas_id").Eq(goqu.I("ref_aas.id")))).
		InnerJoin(goqu.T("aas_submodel_reference_key").As("ref_key"), goqu.On(goqu.I("ref_key.reference_id").Eq(goqu.I("ref.id")))).
		InnerJoin(goqu.T("submodel").As("ref_sm"), goqu.On(goqu.I("ref_sm.submodel_identifier").Eq(goqu.I("ref_key.value")))).
		Select(goqu.L("1")).
		Where(
			goqu.I("ref_aas.auth_uuid").Eq(goqu.I(linkTable+".aas_uuid")),
			goqu.I("ref_sm.auth_uuid").Eq(goqu.I(linkTable+".submodel_uuid")),
		)
}

// SubmodelReferenced reports whether the AAS references the Submodel.
func SubmodelReferenced(ctx context.Context, q Queryer, aasUUID string, submodelUUID string) (bool, error) {
	ds := dialect.From(goqu.T("aas").As("ref_aas")).
		InnerJoin(goqu.T("aas_submodel_reference").As("ref"), goqu.On(goqu.I("ref.aas_id").Eq(goqu.I("ref_aas.id")))).
		InnerJoin(goqu.T("aas_submodel_reference_key").As("ref_key"), goqu.On(goqu.I("ref_key.reference_id").Eq(goqu.I("ref.id")))).
		InnerJoin(goqu.T("submodel").As("ref_sm"), goqu.On(goqu.I("ref_sm.submodel_identifier").Eq(goqu.I("ref_key.value")))).
		Select(goqu.L("1")).
		Where(
			goqu.I("ref_aas.auth_uuid").Eq(goqu.L("?::uuid", aasUUID)),
			goqu.I("ref_sm.auth_uuid").Eq(goqu.L("?::uuid", submodelUUID)),
		).Limit(1).Prepared(true)
	var marker int
	return queryRowDataset(ctx, q, "REBAC-SUBMODELREFERENCED", ds, &marker)
}

// RepairDrift compares the desired state with the tuples stored in OpenFGA
// and enqueues writes for missing and deletes for unexpected tuples. Each
// repair is re-validated under the object's revision lock, so a concurrent
// grant change can never be overwritten.
func (c *Coordinator) RepairDrift(ctx context.Context) (ReconcileReport, error) {
	desired, err := c.desiredTuples(ctx)
	if err != nil {
		return ReconcileReport{}, err
	}
	stored, err := c.storedTuples(ctx)
	if err != nil {
		return ReconcileReport{}, err
	}
	missing, unexpected := diffTuples(desired, stored)
	report := ReconcileReport{MissingTuples: len(missing), UnexpectedTuples: len(unexpected)}
	if len(missing) == 0 && len(unexpected) == 0 {
		return report, nil
	}
	err = c.inReconcileTx(ctx, func(tx *sql.Tx) error {
		return c.enqueueRepairs(ctx, tx, missing, unexpected)
	})
	return report, err
}

func (c *Coordinator) desiredTuples(ctx context.Context) (map[Tuple]struct{}, error) {
	desired := make(map[Tuple]struct{})
	grants, err := selectGrants(ctx, c.db, "REBAC-DESIREDTUPLES-GRANTS", goqu.L("TRUE"))
	if err != nil {
		return nil, err
	}
	for _, grant := range grants {
		desired[grant.Tuple()] = struct{}{}
	}
	links, err := selectLinks(ctx, c.db, "REBAC-DESIREDTUPLES-LINKS", goqu.L("TRUE"))
	if err != nil {
		return nil, err
	}
	for _, link := range links {
		desired[link.Tuple()] = struct{}{}
	}
	return desired, nil
}

func (c *Coordinator) storedTuples(ctx context.Context) (map[Tuple]struct{}, error) {
	stored := make(map[Tuple]struct{})
	continuation := ""
	for {
		tuples, next, err := c.client.Read(ctx, "", continuation)
		if err != nil {
			return nil, err
		}
		for _, tuple := range tuples {
			if isStoredTupleType(tuple.Object) {
				stored[tuple] = struct{}{}
			}
		}
		if next == "" {
			return stored, nil
		}
		continuation = next
	}
}

func isStoredTupleType(object string) bool {
	objectType, _, _ := strings.Cut(object, ":")
	return slices.Contains(storedTupleTypes, objectType)
}

func diffTuples(desired map[Tuple]struct{}, stored map[Tuple]struct{}) ([]Tuple, []Tuple) {
	var missing, unexpected []Tuple
	for tuple := range desired {
		if _, ok := stored[tuple]; !ok {
			missing = append(missing, tuple)
		}
	}
	for tuple := range stored {
		if _, ok := desired[tuple]; !ok {
			unexpected = append(unexpected, tuple)
		}
	}
	sortTuples(missing)
	sortTuples(unexpected)
	return missing, unexpected
}

func sortTuples(tuples []Tuple) {
	slices.SortFunc(tuples, func(a Tuple, b Tuple) int {
		return strings.Compare(a.Object+"#"+a.Relation+"@"+a.User, b.Object+"#"+b.Relation+"@"+b.User)
	})
}

func (c *Coordinator) enqueueRepairs(ctx context.Context, tx *sql.Tx, missing []Tuple, unexpected []Tuple) error {
	if err := lockTupleObjects(ctx, tx, append(append([]Tuple{}, missing...), unexpected...)); err != nil {
		return err
	}
	writes, err := repairsStillNeeded(ctx, tx, missing, false)
	if err != nil {
		return err
	}
	deletes, err := repairsStillNeeded(ctx, tx, unexpected, true)
	if err != nil {
		return err
	}
	if _, err = EnqueueOutbox(ctx, tx, c.scope, writes, OutboxPending); err != nil {
		return err
	}
	_, err = EnqueueOutbox(ctx, tx, c.scope, deletes, OutboxRevocation)
	return err
}

// lockTupleObjects takes the revision locks of all objects in sorted order,
// which keeps concurrent repairs deadlock free.
func lockTupleObjects(ctx context.Context, tx *sql.Tx, tuples []Tuple) error {
	objects := make([]string, 0, len(tuples))
	for _, tuple := range tuples {
		objects = append(objects, tuple.Object)
	}
	slices.Sort(objects)
	for _, object := range slices.Compact(objects) {
		if _, err := LockObjectRevision(ctx, tx, object); err != nil {
			return err
		}
	}
	return nil
}

// repairsStillNeeded keeps the tuples whose desired state still differs from
// OpenFGA after locking: missing tuples that are still desired, or
// unexpected tuples that are still not desired.
func repairsStillNeeded(ctx context.Context, tx *sql.Tx, tuples []Tuple, remove bool) ([]OutboxOperation, error) {
	operations := make([]OutboxOperation, 0, len(tuples))
	for _, tuple := range tuples {
		desired, err := tupleDesired(ctx, tx, tuple)
		if err != nil {
			return nil, err
		}
		if desired == remove {
			continue
		}
		operations = append(operations, OutboxOperation{Delete: remove, Tuple: tuple})
	}
	return operations, nil
}

// tupleDesired re-reads the desired state of one tuple inside the repair
// transaction.
func tupleDesired(ctx context.Context, q Queryer, tuple Tuple) (bool, error) {
	if tuple.Relation == RelationLinkedAAS {
		submodelUUID, _ := strings.CutPrefix(tuple.Object, TypeSubmodel+":")
		aasUUID, _ := strings.CutPrefix(tuple.User, TypeAAS+":")
		links, err := selectLinks(ctx, q, "REBAC-TUPLEDESIRED-LINK", goqu.And(
			goqu.L("submodel_uuid::text = ?", submodelUUID), goqu.L("aas_uuid::text = ?", aasUUID),
		))
		return len(links) > 0, err
	}
	grants, err := selectGrants(ctx, q, "REBAC-TUPLEDESIRED-GRANT",
		goqu.C("object_key").Eq(tuple.Object), goqu.C("relation").Eq(tuple.Relation), goqu.C("subject_key").Eq(tuple.User))
	return len(grants) > 0, err
}
