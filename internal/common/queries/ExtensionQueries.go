/*******************************************************************************
* Copyright (C) 2025 the Eclipse BaSyx Authors and Fraunhofer IESE
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

// Author: Aaron Zielstorff ( Fraunhofer IESE ), Jannik Fried ( Fraunhofer IESE )
package queries

import (
	"github.com/doug-martin/goqu/v9"
	"github.com/doug-martin/goqu/v9/exp"
)

func GetExtensionSubquery(dialect goqu.DialectWrapper, joinTableExtensionColumnName string, joinTable exp.IdentifierExpression, entityIdColumn string, entityIdCondition exp.Expression) *goqu.SelectDataset {
	// Build the jsonb object for semantic ID references
	semanticIdObj := goqu.Func("jsonb_build_object",
		goqu.V("reference_id"), goqu.I("r.id"),
		goqu.V("reference_type"), goqu.I("r.type"),
		goqu.V("key_id"), goqu.I("rk.id"),
		goqu.V("key_type"), goqu.I("rk.type"),
		goqu.V("key_value"), goqu.I("rk.value"),
	)

	extensionSemanticIdSubquery := dialect.From(goqu.T("reference").As("r")).
		Select(goqu.Func("jsonb_agg", goqu.L("?", semanticIdObj))).
		Join(
			goqu.T("extension").As("e"),
			goqu.On(goqu.I("e.semantic_id").Eq(goqu.I("r.id"))),
		).
		Join(
			goqu.T("reference_key").As("rk"),
			goqu.On(goqu.I("r.id").Eq(goqu.I("rk.reference_id"))),
		).
		Where(goqu.I("e.id").Eq(goqu.I("jt." + joinTableExtensionColumnName)))

	// Build the jsonb object for semantic ID referred references
	semanticIdReferredObj := goqu.Func("jsonb_build_object",
		goqu.V("reference_id"), goqu.I("r.id"),
		goqu.V("reference_type"), goqu.I("r.type"),
		goqu.V("parentReference"), goqu.I("r.parentreference"),
		goqu.V("rootReference"), goqu.I("r.rootreference"),
		goqu.V("key_id"), goqu.I("rk.id"),
		goqu.V("key_type"), goqu.I("rk.type"),
		goqu.V("key_value"), goqu.I("rk.value"),
	)

	extensionSemanticIdReferredSubquery := dialect.From(goqu.T("reference").As("ref")).
		Select(goqu.Func("jsonb_agg", goqu.L("?", semanticIdReferredObj))).
		Join(
			goqu.T("reference").As("r"),
			goqu.On(goqu.I("ref.id").Eq(goqu.I("r.rootreference"))),
		).
		Join(
			goqu.T("extension").As("e"),
			goqu.On(goqu.I("e.semantic_id").Eq(goqu.I("ref.id"))),
		).
		LeftJoin(
			goqu.T("reference_key").As("rk"),
			goqu.On(goqu.I("rk.reference_id").Eq(goqu.I("r.id"))),
		).
		Where(
			goqu.I("jt."+joinTableExtensionColumnName).Eq(goqu.I("e.id")),
			goqu.I("r.id").IsNotNull(),
		)

	// Build the jsonb object for refersTo references
	refersToObj := goqu.Func("jsonb_build_object",
		goqu.V("reference_id"), goqu.I("r.id"),
		goqu.V("reference_type"), goqu.I("r.type"),
		goqu.V("key_id"), goqu.I("rk.id"),
		goqu.V("key_type"), goqu.I("rk.type"),
		goqu.V("key_value"), goqu.I("rk.value"),
	)

	extensionRefersToSubquery := dialect.From(goqu.T("reference").As("r")).
		Select(goqu.Func("jsonb_agg", goqu.L("?", refersToObj))).
		Join(
			goqu.T("extension_refers_to").As("ert"),
			goqu.On(goqu.I("ert.extension_id").Eq(goqu.I("jt."+joinTableExtensionColumnName))),
		).
		Join(
			goqu.T("reference_key").As("rk"),
			goqu.On(goqu.I("r.id").Eq(goqu.I("rk.reference_id"))),
		).
		Where(goqu.I("ert.reference_id").Eq(goqu.I("r.id")))

	// Build the jsonb object for refersTo referred references
	refersToReferredObj := goqu.Func("jsonb_build_object",
		goqu.V("reference_id"), goqu.I("ref.id"),
		goqu.V("reference_type"), goqu.I("ref.type"),
		goqu.V("parentReference"), goqu.I("ref.parentreference"),
		goqu.V("rootReference"), goqu.I("ref.rootreference"),
		goqu.V("key_id"), goqu.I("rk.id"),
		goqu.V("key_type"), goqu.I("rk.type"),
		goqu.V("key_value"), goqu.I("rk.value"),
	)

	extensionRefersToReferredSubquery := dialect.From(goqu.T("reference").As("ref")).
		Select(goqu.Func("jsonb_agg", goqu.L("?", refersToReferredObj))).
		Join(
			goqu.T("extension_refers_to").As("ert"),
			goqu.On(goqu.I("ert.extension_id").Eq(goqu.I("jt."+joinTableExtensionColumnName))),
		).
		Join(
			goqu.T("reference_key").As("rk"),
			goqu.On(goqu.I("ref.id").Eq(goqu.I("rk.reference_id"))),
		).
		Where(
			goqu.I("ref.rootreference").Eq(goqu.I("ert.reference_id")),
			goqu.I("ref.id").IsNotNull(),
		)

	// Build the jsonb object for supplemental semantic ID references
	supplementalSemanticIdObj := goqu.Func("jsonb_build_object",
		goqu.V("reference_id"), goqu.I("r.id"),
		goqu.V("reference_type"), goqu.I("r.type"),
		goqu.V("key_id"), goqu.I("rk.id"),
		goqu.V("key_type"), goqu.I("rk.type"),
		goqu.V("key_value"), goqu.I("rk.value"),
	)

	extensionSupplementalSemanticIdSubquery := dialect.From(goqu.T("reference").As("r")).
		Select(goqu.Func("jsonb_agg", goqu.L("?", supplementalSemanticIdObj))).
		Join(
			goqu.T("extension_supplemental_semantic_id").As("essi"),
			goqu.On(goqu.I("essi.extension_id").Eq(goqu.I("jt."+joinTableExtensionColumnName))),
		).
		Join(
			goqu.T("reference_key").As("rk"),
			goqu.On(goqu.I("r.id").Eq(goqu.I("rk.reference_id"))),
		).
		Where(goqu.I("essi.reference_id").Eq(goqu.I("r.id")))

	// Build the jsonb object for supplemental semantic ID referred references
	supplementalSemanticIdReferredObj := goqu.Func("jsonb_build_object",
		goqu.V("reference_id"), goqu.I("ref.id"),
		goqu.V("reference_type"), goqu.I("ref.type"),
		goqu.V("parentReference"), goqu.I("ref.parentreference"),
		goqu.V("rootReference"), goqu.I("ref.rootreference"),
		goqu.V("key_id"), goqu.I("rk.id"),
		goqu.V("key_type"), goqu.I("rk.type"),
		goqu.V("key_value"), goqu.I("rk.value"),
	)

	extensionSupplementalSemanticIdReferredSubquery := dialect.From(goqu.T("reference").As("ref")).
		Select(goqu.Func("jsonb_agg", goqu.L("?", supplementalSemanticIdReferredObj))).
		Join(
			goqu.T("extension_supplemental_semantic_id").As("essi"),
			goqu.On(goqu.I("essi.extension_id").Eq(goqu.I("jt."+joinTableExtensionColumnName))),
		).
		Join(
			goqu.T("reference_key").As("rk"),
			goqu.On(goqu.I("ref.id").Eq(goqu.I("rk.reference_id"))),
		).
		Where(
			goqu.I("ref.rootreference").Eq(goqu.I("essi.reference_id")),
			goqu.I("ref.id").IsNotNull(),
		)

	// Build the main extension jsonb object
	extensionObj := goqu.Func("jsonb_build_object",
		goqu.V("dbId"), goqu.I("jt."+joinTableExtensionColumnName),
		goqu.V("name"), goqu.I("e.name"),
		goqu.V("value_type"), goqu.I("e.value_type"),
		goqu.V("value"), goqu.COALESCE(
			goqu.I("e.value_text"),
			goqu.L("?::text", goqu.I("e.value_num")),
			goqu.L("?::text", goqu.I("e.value_bool")),
			goqu.L("?::text", goqu.I("e.value_time")),
			goqu.L("?::text", goqu.I("e.value_datetime")),
		),
		goqu.V("semanticIdReferenceRows"), extensionSemanticIdSubquery,
		goqu.V("semanticIdReferredReferencesRows"), extensionSemanticIdReferredSubquery,
		goqu.V("refersToReferenceRows"), extensionRefersToSubquery,
		goqu.V("refersToReferredReferencesRows"), extensionRefersToReferredSubquery,
		goqu.V("supplementalSemanticIdReferenceRows"), extensionSupplementalSemanticIdSubquery,
		goqu.V("supplementalSemanticIdReferredReferenceRows"), extensionSupplementalSemanticIdReferredSubquery,
	)

	extensionSubquery := dialect.From(joinTable.As("jt")).
		Select(goqu.Func("jsonb_agg", goqu.L("?", extensionObj))).
		Join(
			goqu.T("extension").As("e"),
			goqu.On(goqu.I("jt."+joinTableExtensionColumnName).Eq(goqu.I("e.id"))),
		).
		Where(goqu.I("jt." + entityIdColumn).Eq(entityIdCondition))
	return extensionSubquery
}
