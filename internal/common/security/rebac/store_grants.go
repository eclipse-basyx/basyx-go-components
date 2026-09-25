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
	"time"

	"github.com/doug-martin/goqu/v9"
	"github.com/doug-martin/goqu/v9/exp"
)

const grantTable = "rebac_grant"

func grantColumns() []any {
	return []any{
		goqu.C("object_key"), goqu.C("object_type"), goqu.L("COALESCE(object_uuid::text, '')"),
		goqu.L("COALESCE(element_path, '')"), goqu.C("relation"), goqu.C("subject_type"),
		goqu.C("subject_key"), goqu.C("subject_issuer"), goqu.C("subject_name"),
		goqu.C("created_by"), goqu.C("created_at"),
	}
}

func scanGrants(rows *sql.Rows) ([]Grant, error) {
	defer func() { _ = rows.Close() }()
	var grants []Grant
	for rows.Next() {
		var grant Grant
		if err := rows.Scan(
			&grant.ObjectKey, &grant.ObjectType, &grant.ObjectUUID, &grant.ElementPath, &grant.Relation,
			&grant.SubjectType, &grant.SubjectKey, &grant.SubjectIssuer, &grant.SubjectName,
			&grant.CreatedBy, &grant.CreatedAt,
		); err != nil {
			return nil, fmt.Errorf("REBAC-SCANGRANTS-SCAN: %w", err)
		}
		grants = append(grants, grant)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("REBAC-SCANGRANTS-ROWS: %w", err)
	}
	return grants, nil
}

func selectGrants(ctx context.Context, q Queryer, code string, where ...exp.Expression) ([]Grant, error) {
	ds := dialect.From(goqu.T(grantTable)).Select(grantColumns()...).Where(where...).
		Order(goqu.C("object_key").Asc(), goqu.C("relation").Asc(), goqu.C("subject_key").Asc())
	rows, err := queryDataset(ctx, q, code, ds)
	if err != nil {
		return nil, err
	}
	return scanGrants(rows)
}

// ListGrants returns the direct grants of one object.
func ListGrants(ctx context.Context, q Queryer, objectKey string) ([]Grant, error) {
	return selectGrants(ctx, q, "REBAC-LISTGRANTS", goqu.C("object_key").Eq(objectKey))
}

// InsertGrant stores a grant; an identical existing grant is kept.
func InsertGrant(ctx context.Context, tx *sql.Tx, grant Grant) (bool, error) {
	record := goqu.Record{
		"object_key": grant.ObjectKey, "object_type": grant.ObjectType, "relation": grant.Relation,
		"subject_type": grant.SubjectType, "subject_key": grant.SubjectKey, "subject_issuer": grant.SubjectIssuer,
		"subject_name": grant.SubjectName, "created_by": grant.CreatedBy,
	}
	if grant.ObjectUUID != "" {
		record["object_uuid"] = goqu.L("?::uuid", grant.ObjectUUID)
	}
	if grant.ElementPath != "" {
		record["element_path"] = grant.ElementPath
	}
	ds := dialect.Insert(grantTable).Rows(record).OnConflict(goqu.DoNothing()).Prepared(true)
	result, err := execDataset(ctx, tx, "REBAC-INSERTGRANT", ds)
	if err != nil {
		return false, err
	}
	affected, err := result.RowsAffected()
	return affected > 0, err
}

// DeleteGrant removes one grant.
func DeleteGrant(ctx context.Context, tx *sql.Tx, grant Grant) error {
	ds := dialect.Delete(grantTable).Where(
		goqu.C("object_key").Eq(grant.ObjectKey),
		goqu.C("relation").Eq(grant.Relation),
		goqu.C("subject_key").Eq(grant.SubjectKey),
	).Prepared(true)
	_, err := execDataset(ctx, tx, "REBAC-DELETEGRANT", ds)
	return err
}

// DeleteGrantsOfResource removes the grants of an identifiable and, for a
// Submodel, of all its element paths. The deleted grants are returned so
// their tuples can be revoked.
func DeleteGrantsOfResource(ctx context.Context, tx *sql.Tx, authUUID string) ([]Grant, error) {
	ds := dialect.Delete(grantTable).
		Where(goqu.C("object_uuid").Eq(goqu.L("?::uuid", authUUID))).
		Returning(grantColumns()...).Prepared(true)
	query, args, err := ds.ToSQL()
	if err != nil {
		return nil, fmt.Errorf("REBAC-DELETERESOURCEGRANTS-BUILDQ: %w", err)
	}
	rows, err := tx.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("REBAC-DELETERESOURCEGRANTS-EXECQ: %w", err)
	}
	return scanGrants(rows)
}

// ElementGrantPaths returns the element paths of a Submodel with grants of
// any of the given subjects.
func ElementGrantPaths(ctx context.Context, q Queryer, submodelAuthUUID string, subjectKeys []string) ([]string, error) {
	ds := dialect.From(goqu.T(grantTable)).
		SelectDistinct(goqu.C("element_path")).
		Where(
			goqu.C("object_type").Eq(TypeElement),
			goqu.C("object_uuid").Eq(goqu.L("?::uuid", submodelAuthUUID)),
			goqu.C("subject_key").In(subjectKeys),
		).
		Order(goqu.C("element_path").Asc())
	return queryStrings(ctx, q, "REBAC-ELEMENTGRANTPATHS", ds)
}

// CandidateObjects returns the objects of a type that carry direct grants of
// any of the given subjects, bounded by limit.
func CandidateObjects(ctx context.Context, q Queryer, objectType string, subjectKeys []string, limit int) ([]string, error) {
	ds := dialect.From(goqu.T(grantTable)).
		SelectDistinct(goqu.L("object_uuid::text")).
		Where(goqu.C("object_type").Eq(objectType), goqu.C("subject_key").In(subjectKeys)).
		Limit(uint(limit)) //nolint:gosec // limit is a validated positive configuration value
	return queryStrings(ctx, q, "REBAC-CANDIDATEOBJECTS", ds)
}

func queryStrings(ctx context.Context, q Queryer, code string, ds *goqu.SelectDataset) ([]string, error) {
	rows, err := queryDataset(ctx, q, code, ds)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	var values []string
	for rows.Next() {
		var value string
		if err = rows.Scan(&value); err != nil {
			return nil, fmt.Errorf("%s-SCAN: %w", code, err)
		}
		values = append(values, value)
	}
	return values, rows.Err()
}

// CountRelation counts the grants of one relation on an object.
func CountRelation(ctx context.Context, q Queryer, objectKey string, relation string) (int, error) {
	ds := dialect.From(goqu.T(grantTable)).Select(goqu.COUNT(goqu.Star())).
		Where(goqu.C("object_key").Eq(objectKey), goqu.C("relation").Eq(relation)).Prepared(true)
	var count int
	_, err := queryRowDataset(ctx, q, "REBAC-COUNTRELATION", ds, &count)
	return count, err
}

// ObjectRevision returns the grant revision of an object.
func ObjectRevision(ctx context.Context, q Queryer, objectKey string) (int64, error) {
	ds := dialect.From(goqu.T("rebac_object_revision")).Select(goqu.C("revision")).
		Where(goqu.C("object_key").Eq(objectKey)).Prepared(true)
	var revision int64
	_, err := queryRowDataset(ctx, q, "REBAC-OBJECTREVISION", ds, &revision)
	return revision, err
}

// LockObjectRevision locks an object's revision row for the transaction and
// returns the current revision. Concurrent grant changes of the same object
// serialize on this lock, which keeps their outbox order equal to commit order.
func LockObjectRevision(ctx context.Context, tx *sql.Tx, objectKey string) (int64, error) {
	insert := dialect.Insert("rebac_object_revision").
		Rows(goqu.Record{"object_key": objectKey, "revision": 0}).
		OnConflict(goqu.DoNothing()).Prepared(true)
	if _, err := execDataset(ctx, tx, "REBAC-LOCKOBJECTREVISION-ENSURE", insert); err != nil {
		return 0, err
	}
	ds := dialect.From(goqu.T("rebac_object_revision")).Select(goqu.C("revision")).
		Where(goqu.C("object_key").Eq(objectKey)).ForUpdate(exp.Wait).Prepared(true)
	var revision int64
	_, err := queryRowDataset(ctx, tx, "REBAC-LOCKOBJECTREVISION-LOCK", ds, &revision)
	return revision, err
}

// BumpObjectRevision increments the revision of a locked object.
func BumpObjectRevision(ctx context.Context, tx *sql.Tx, objectKey string) (int64, error) {
	ds := dialect.Update("rebac_object_revision").
		Set(goqu.Record{"revision": goqu.L("revision + 1"), "updated_at": goqu.L("clock_timestamp()")}).
		Where(goqu.C("object_key").Eq(objectKey)).
		Returning(goqu.C("revision")).Prepared(true)
	var revision int64
	_, err := queryRowDataset(ctx, tx, "REBAC-BUMPOBJECTREVISION", ds, &revision)
	return revision, err
}

// DeleteObjectRevisions removes the revision rows of deleted objects.
func DeleteObjectRevisions(ctx context.Context, tx *sql.Tx, objectKeys []string) error {
	if len(objectKeys) == 0 {
		return nil
	}
	ds := dialect.Delete("rebac_object_revision").Where(goqu.C("object_key").In(objectKeys)).Prepared(true)
	_, err := execDataset(ctx, tx, "REBAC-DELETEOBJECTREVISIONS", ds)
	return err
}

// SubmodelLink is an approved AAS-to-Submodel inheritance link.
type SubmodelLink struct {
	SubmodelUUID string    `json:"-"`
	AASUUID      string    `json:"-"`
	ApprovedBy   string    `json:"approvedBy"`
	ApprovedAt   time.Time `json:"approvedAt"`
}

// Tuple returns the OpenFGA tuple projected from the link.
func (l SubmodelLink) Tuple() Tuple {
	return Tuple{
		User:     ResourceObject(TypeAAS, l.AASUUID),
		Relation: RelationLinkedAAS,
		Object:   ResourceObject(TypeSubmodel, l.SubmodelUUID),
	}
}

func selectLinks(ctx context.Context, q Queryer, code string, where exp.Expression) ([]SubmodelLink, error) {
	ds := dialect.From(goqu.T("rebac_submodel_link")).
		Select(goqu.L("submodel_uuid::text"), goqu.L("aas_uuid::text"), goqu.C("approved_by"), goqu.C("approved_at")).
		Where(where).Order(goqu.C("aas_uuid").Asc())
	rows, err := queryDataset(ctx, q, code, ds)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	var links []SubmodelLink
	for rows.Next() {
		var link SubmodelLink
		if err = rows.Scan(&link.SubmodelUUID, &link.AASUUID, &link.ApprovedBy, &link.ApprovedAt); err != nil {
			return nil, fmt.Errorf("%s-SCAN: %w", code, err)
		}
		links = append(links, link)
	}
	return links, rows.Err()
}

// ListSubmodelLinks returns the approved AAS links of a Submodel.
func ListSubmodelLinks(ctx context.Context, q Queryer, submodelUUID string) ([]SubmodelLink, error) {
	return selectLinks(ctx, q, "REBAC-LISTSUBMODELLINKS", goqu.C("submodel_uuid").Eq(goqu.L("?::uuid", submodelUUID)))
}

// LinkedSubmodels returns Submodels linked to any of the given AAS.
func LinkedSubmodels(ctx context.Context, q Queryer, aasUUIDs []string) ([]string, error) {
	if len(aasUUIDs) == 0 {
		return nil, nil
	}
	literal, err := uuidArrayLiteral(aasUUIDs)
	if err != nil {
		return nil, err
	}
	ds := dialect.From(goqu.T("rebac_submodel_link")).SelectDistinct(goqu.L("submodel_uuid::text")).
		Where(goqu.L("aas_uuid = ANY(?::uuid[])", literal))
	return queryStrings(ctx, q, "REBAC-LINKEDSUBMODELS", ds)
}

// InsertSubmodelLink stores an approved link.
func InsertSubmodelLink(ctx context.Context, tx *sql.Tx, link SubmodelLink) error {
	ds := dialect.Insert("rebac_submodel_link").Rows(goqu.Record{
		"submodel_uuid": goqu.L("?::uuid", link.SubmodelUUID),
		"aas_uuid":      goqu.L("?::uuid", link.AASUUID),
		"approved_by":   link.ApprovedBy,
	}).OnConflict(goqu.DoNothing()).Prepared(true)
	_, err := execDataset(ctx, tx, "REBAC-INSERTSUBMODELLINK", ds)
	return err
}

// DeleteSubmodelLinks deletes links matching where and returns them.
func DeleteSubmodelLinks(ctx context.Context, tx *sql.Tx, where exp.Expression) ([]SubmodelLink, error) {
	ds := dialect.Delete("rebac_submodel_link").Where(where).
		Returning(goqu.L("submodel_uuid::text"), goqu.L("aas_uuid::text"), goqu.C("approved_by"), goqu.C("approved_at")).
		Prepared(true)
	query, args, err := ds.ToSQL()
	if err != nil {
		return nil, fmt.Errorf("REBAC-DELETESUBMODELLINKS-BUILDQ: %w", err)
	}
	rows, err := tx.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("REBAC-DELETESUBMODELLINKS-EXECQ: %w", err)
	}
	defer func() { _ = rows.Close() }()
	var links []SubmodelLink
	for rows.Next() {
		var link SubmodelLink
		if err = rows.Scan(&link.SubmodelUUID, &link.AASUUID, &link.ApprovedBy, &link.ApprovedAt); err != nil {
			return nil, fmt.Errorf("REBAC-DELETESUBMODELLINKS-SCAN: %w", err)
		}
		links = append(links, link)
	}
	return links, rows.Err()
}

// LinkOfSubmodel selects links of one Submodel.
func LinkOfSubmodel(submodelUUID string) exp.Expression {
	return goqu.C("submodel_uuid").Eq(goqu.L("?::uuid", submodelUUID))
}

// LinkOfAAS selects links of one AAS.
func LinkOfAAS(aasUUID string) exp.Expression {
	return goqu.C("aas_uuid").Eq(goqu.L("?::uuid", aasUUID))
}

// LinkBetween selects one link.
func LinkBetween(aasUUID string, submodelUUID string) exp.Expression {
	return goqu.And(LinkOfAAS(aasUUID), LinkOfSubmodel(submodelUUID))
}

const invitationTable = "rebac_invitation"
