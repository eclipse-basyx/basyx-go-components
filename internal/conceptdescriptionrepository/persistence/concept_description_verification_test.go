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

package persistence

import (
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/FriedJannik/aas-go-sdk/types"
	"github.com/eclipse-basyx/basyx-go-components/internal/common"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestConceptDescriptionStrictVerificationRejectsBeforeTransaction(t *testing.T) {
	for _, violation := range []string{"ID-short", "AASd-005"} {
		t.Run(violation, func(t *testing.T) {
			for _, operation := range []string{"create", "put"} {
				t.Run(operation, func(t *testing.T) {
					testConceptDescriptionRejectsBeforeTransaction(t, violation, operation)
				})
			}
		})
	}
}

func testConceptDescriptionRejectsBeforeTransaction(t *testing.T, violation, operation string) {
	t.Helper()
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	t.Cleanup(func() {
		mock.ExpectClose()
		require.NoError(t, db.Close())
		require.NoError(t, mock.ExpectationsWereMet())
	})
	backend, err := NewConceptDescriptionBackendFromDB(db, "strict")
	require.NoError(t, err)
	cd := types.NewConceptDescription("urn:example:description")
	if violation == "ID-short" {
		idShort := "1Invalid"
		cd.SetIDShort(&idShort)
	} else {
		administration := types.NewAdministrativeInformation()
		revision := "1"
		administration.SetRevision(&revision)
		cd.SetAdministration(administration)
	}
	ctx := contextWithConceptDescriptionConfig(t)
	if operation == "create" {
		err = backend.CreateConceptDescription(ctx, cd)
	} else {
		var replaced bool
		replaced, err = backend.PutConceptDescription(ctx, cd.ID(), cd)
		assert.False(t, replaced)
	}
	require.Error(t, err)
	assert.True(t, common.IsErrBadRequest(err), "expected semantic bad request before any SQL, got %v", err)
	assert.Contains(t, err.Error(), violation)
}
