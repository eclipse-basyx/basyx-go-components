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
	"regexp"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
)

func TestGrantTuplesAllowsCreatorOnlyForRepository(t *testing.T) {
	principal := Principal{Kind: "user", Issuer: "https://issuer.example", ID: "alice"}
	if _, err := grantTuples("aas", "aas:machine", []Grant{{Principal: principal, Role: "creator"}}); err == nil {
		t.Fatal("creator grant unexpectedly accepted for an AAS")
	}
	tuples, err := grantTuples("repository", "repository:root", []Grant{{Principal: principal, Role: "creator"}})
	if err != nil || len(tuples) != 1 || tuples[0].Relation != "creator" {
		t.Fatalf("repository creator tuple = %#v, %v", tuples, err)
	}
}

func TestDirectGrantsIncludesRepositoryCreator(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	mock.ExpectBegin()
	tx, err := db.BeginTx(t.Context(), nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = tx.Rollback() })
	mock.ExpectQuery(regexp.QuoteMeta("SELECT \"subject\", \"relation\", \"object\" FROM \"rebac_relationship\"")).WithArgs("repository:root", "direct", "scope", "viewer", "editor", "executor", "owner", "creator").WillReturnRows(sqlmock.NewRows([]string{"subject", "relation", "object"}).AddRow("user:alice", "creator", "repository:root"))

	tuples, err := (&StateStore{Scope: "scope"}).DirectGrants(t.Context(), tx, "repository:root")
	if err != nil || len(tuples) != 1 || tuples[0].Relation != "creator" {
		t.Fatalf("DirectGrants() = %#v, %v", tuples, err)
	}
	if err = mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestSnapshotRestoresPrincipalsAndInheritance(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	state := &StateStore{DB: db, Scope: "scope"}
	resource := StoredResource{UUID: "resource-id", Kind: "repository"}
	object, err := ResourceObject(state.Scope, resource.Kind, resource.UUID)
	if err != nil {
		t.Fatal(err)
	}
	user := Principal{Kind: "user", Issuer: "https://issuer.example", ID: "alice"}
	group := Principal{Kind: "group", Issuer: "https://issuer.example", ID: "operators"}
	userSubject, _ := user.Object()
	groupSubject, _ := group.Object()

	mock.ExpectBegin()
	mock.ExpectQuery(regexp.QuoteMeta("SELECT \"desired_revision\", \"applied_revision\" FROM \"rebac_scope\"")).WithArgs("scope").WillReturnRows(sqlmock.NewRows([]string{"desired_revision", "applied_revision"}).AddRow(4, 4))
	mock.ExpectQuery(regexp.QuoteMeta(`SELECT "resource_uuid", "kind", "identifier" FROM "rebac_resource"`)).WithArgs("", "repository", "scope").WillReturnRows(sqlmock.NewRows([]string{"resource_uuid", "kind", "identifier"}).AddRow("resource-id", "repository", ""))
	mock.ExpectQuery(regexp.QuoteMeta("SELECT \"subject\", \"relation\", \"object\" FROM \"rebac_relationship\"")).WithArgs(object, "direct", "scope", "viewer", "editor", "executor", "owner", "creator").WillReturnRows(sqlmock.NewRows([]string{"subject", "relation", "object"}).AddRow(userSubject, "creator", object).AddRow(groupSubject+"#member", "viewer", object))
	mock.ExpectQuery(regexp.QuoteMeta("SELECT \"subject\", \"kind\", \"issuer\", \"identifier\" FROM \"rebac_principal\"")).WithArgs("scope", userSubject, groupSubject).WillReturnRows(sqlmock.NewRows([]string{"subject", "kind", "issuer", "identifier"}).AddRow(userSubject, user.Kind, user.Issuer, user.ID).AddRow(groupSubject, group.Kind, group.Issuer, group.ID))
	mock.ExpectQuery(regexp.QuoteMeta("SELECT \"subject\", \"relation\", \"object\" FROM \"rebac_relationship\"")).WithArgs(object, "scope", "parent", "aas_parent", "source").WillReturnRows(sqlmock.NewRows([]string{"subject", "relation", "object"}).AddRow("repository:parent", "parent", object))
	mock.ExpectCommit()

	snapshot, err := (&GrantService{State: state}).Snapshot(t.Context(), GrantActor{Administrator: true}, resource)
	if err != nil {
		t.Fatalf("Snapshot() error = %v", err)
	}
	if snapshot.Revision != 4 || snapshot.AppliedRevision != 4 || len(snapshot.Grants) != 2 || len(snapshot.Inheritance) != 1 {
		t.Fatalf("Snapshot() = %+v", snapshot)
	}
	if snapshot.Grants[0] != (Grant{Principal: user, Role: "creator"}) || snapshot.Grants[1] != (Grant{Principal: group, Role: "viewer"}) {
		t.Fatalf("grants = %#v", snapshot.Grants)
	}
	if err = mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestSnapshotRequiresManagementAuthority(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	state := &StateStore{DB: db, Scope: "scope"}
	mock.ExpectBegin()
	mock.ExpectQuery(regexp.QuoteMeta("SELECT \"desired_revision\", \"applied_revision\" FROM \"rebac_scope\"")).WithArgs("scope").WillReturnRows(sqlmock.NewRows([]string{"desired_revision", "applied_revision"}).AddRow(1, 1))
	mock.ExpectQuery(regexp.QuoteMeta(`SELECT "resource_uuid", "kind", "identifier" FROM "rebac_resource"`)).WithArgs("", "repository", "scope").WillReturnRows(sqlmock.NewRows([]string{"resource_uuid", "kind", "identifier"}).AddRow("resource-id", "repository", ""))
	mock.ExpectRollback()

	_, err = (&GrantService{State: state, Checker: fixedChecker{allowed: false}}).Snapshot(t.Context(), GrantActor{User: "user:alice"}, StoredResource{UUID: "resource-id", Kind: "repository"})
	if err != ErrForbidden {
		t.Fatalf("Snapshot() error = %v, want forbidden", err)
	}
	if err = mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}
