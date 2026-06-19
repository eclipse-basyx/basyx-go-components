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

package descriptors

import (
	"context"
	"fmt"
	"strings"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/FriedJannik/aas-go-sdk/types"
	"github.com/eclipse-basyx/basyx-go-components/internal/common/model/grammar"
	auth "github.com/eclipse-basyx/basyx-go-components/internal/common/security"
)

func TestReadSubmodelDescriptorSupplementalSemanticReferencesAppliesFragmentFilter(t *testing.T) {
	field := grammar.ModelStringPattern("$aasdesc#submodelDescriptors[].supplementalSemanticIds[].keys[].value")
	value := grammar.StandardString("supplementalsemanticIdExample value")
	fragment := grammar.FragmentStringPattern("$aasdesc#submodelDescriptors[].supplementalSemanticIds[]")
	condition := grammar.LogicalExpression{
		Eq: grammar.ComparisonItems{
			{Field: &field},
			{StrVal: &value},
		},
	}
	ctx := auth.WithQueryFilter(context.Background(), &auth.QueryFilter{
		Filters: auth.FragmentFilters{
			fragment: condition,
		},
		FilterMatch: auth.FragmentMatchModes{
			fragment: true,
		},
	})

	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherFunc(func(_ string, actual string) error {
		for _, want := range []string{
			`submodel_descriptor_supplemental_semantic_id_reference`,
			`aasdesc_submodel_descriptor_supplemental_semantic_id_reference_key`,
			`"aasdesc_submodel_descriptor_supplemental_semantic_id_reference_key"."value"`,
			`'supplementalsemanticIdExample value'`,
		} {
			if !strings.Contains(actual, want) {
				return fmt.Errorf("expected SQL to contain %q, got: %s", want, actual)
			}
		}
		return nil
	})))
	if err != nil {
		t.Fatalf("sqlmock.New failed: %v", err)
	}
	defer func() {
		_ = db.Close()
	}()

	mock.ExpectQuery("supplemental reference lookup").
		WillReturnRows(sqlmock.NewRows([]string{
			"owner_id",
			"ref_id",
			"ref_type",
			"key_id",
			"key_type",
			"key_value",
			"parent_reference_payload",
		}))

	out, err := ReadSubmodelDescriptorSupplementalSemanticReferencesByDescriptorIDs(ctx, db, []int64{12})
	if err != nil {
		t.Fatalf("ReadSubmodelDescriptorSupplementalSemanticReferencesByDescriptorIDs returned error: %v", err)
	}
	if refs, ok := out[12]; !ok || refs != nil {
		t.Fatalf("expected descriptor 12 to have nil supplemental references, got %#v", out)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet sqlmock expectations: %v", err)
	}
}

func TestReadRepositorySupplementalSemanticReferencesAppliesFragmentFilter(t *testing.T) {
	tests := []struct {
		name           string
		field          grammar.ModelStringPattern
		fragment       grammar.FragmentStringPattern
		expectedTables []string
		read           func(context.Context, DBQueryer, []int64) (map[int64][]types.IReference, error)
	}{
		{
			name:     "submodel",
			field:    "$sm#supplementalSemanticIds[].keys[].value",
			fragment: "$sm#supplementalSemanticIds[]",
			expectedTables: []string{
				`submodel_supplemental_semantic_id_reference`,
				`sm_supplemental_semantic_id_reference_key`,
			},
			read: ReadSubmodelSupplementalSemanticReferencesBySubmodelIDs,
		},
		{
			name:     "submodel element",
			field:    "$sme#supplementalSemanticIds[].keys[].value",
			fragment: "$sme#supplementalSemanticIds[]",
			expectedTables: []string{
				`submodel_element_supplemental_semantic_id_reference`,
				`sme_supplemental_semantic_id_reference_key`,
			},
			read: ReadSubmodelElementSupplementalSemanticReferencesByElementIDs,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			value := grammar.StandardString("FILTER_VISIBLE")
			condition := grammar.LogicalExpression{
				Eq: grammar.ComparisonItems{
					{Field: &test.field},
					{StrVal: &value},
				},
			}
			ctx := auth.WithQueryFilter(context.Background(), &auth.QueryFilter{
				Filters: auth.FragmentFilters{
					test.fragment: condition,
				},
				FilterMatch: auth.FragmentMatchModes{
					test.fragment: true,
				},
			})

			db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherFunc(func(_ string, actual string) error {
				for _, expectedTable := range test.expectedTables {
					if !strings.Contains(actual, expectedTable) {
						return fmt.Errorf("expected SQL to contain %q, got: %s", expectedTable, actual)
					}
				}
				if !strings.Contains(actual, "FILTER_VISIBLE") {
					return fmt.Errorf("expected SQL to contain filter value, got: %s", actual)
				}
				return nil
			})))
			if err != nil {
				t.Fatalf("sqlmock.New failed: %v", err)
			}
			defer func() {
				_ = db.Close()
			}()

			mock.ExpectQuery("supplemental reference lookup").
				WillReturnRows(sqlmock.NewRows([]string{
					"owner_id",
					"ref_id",
					"ref_type",
					"key_id",
					"key_type",
					"key_value",
					"parent_reference_payload",
				}))

			out, readErr := test.read(ctx, db, []int64{12})
			if readErr != nil {
				t.Fatalf("supplemental reference read returned error: %v", readErr)
			}
			if refs, ok := out[12]; !ok || refs != nil {
				t.Fatalf("expected owner 12 to have nil supplemental references, got %#v", out)
			}
			if expectationErr := mock.ExpectationsWereMet(); expectationErr != nil {
				t.Fatalf("unmet sqlmock expectations: %v", expectationErr)
			}
		})
	}
}
