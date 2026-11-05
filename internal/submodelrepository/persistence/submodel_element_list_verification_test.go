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
* MERCHANTABILITY, FITNESS FOR A PARTICULAR PURPOSE AND NONINFRINGEMENT.
* IN NO EVENT SHALL THE AUTHORS OR COPYRIGHT HOLDERS BE LIABLE FOR ANY
* CLAIM, DAMAGES OR OTHER LIABILITY, WHETHER IN AN ACTION OF CONTRACT,
* TORT OR OTHERWISE, ARISING FROM, OUT OF OR IN CONNECTION WITH THE
* SOFTWARE OR THE USE OR OTHER DEALINGS IN THE SOFTWARE.
*
* SPDX-License-Identifier: MIT
******************************************************************************/

package persistence

import (
	"testing"

	sqlmock "github.com/DATA-DOG/go-sqlmock"
	"github.com/FriedJannik/aas-go-sdk/types"
	gen "github.com/eclipse-basyx/basyx-go-components/internal/common/model"
	"github.com/stretchr/testify/require"
)

func TestLockSubmodelForListVerificationLocksSubmodelOnlyForListWrites(t *testing.T) {
	testCases := []struct {
		name             string
		verificationMode gen.VerificationMode
		path             string
		element          types.ISubmodelElement
		expectLock       bool
	}{
		{name: "list member", verificationMode: gen.VerificationModeStrict, path: "Readings[0]", element: types.NewProperty(types.DataTypeDefXSDInt), expectLock: true},
		{name: "list", verificationMode: gen.VerificationModePermissive, path: "Readings", element: types.NewSubmodelElementList(types.AASSubmodelElementsProperty), expectLock: true},
		{name: "other element", verificationMode: gen.VerificationModeStrict, path: "Temperature", element: types.NewProperty(types.DataTypeDefXSDInt)},
		{name: "verification off", verificationMode: gen.VerificationModeOff, path: "Readings[0]", element: types.NewProperty(types.DataTypeDefXSDInt)},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			db, mock, err := sqlmock.New()
			require.NoError(t, err)
			defer func() { _ = db.Close() }()

			mock.ExpectBegin()
			if testCase.expectLock {
				mock.ExpectQuery(`SELECT "id" FROM "submodel" .*FOR UPDATE`).
					WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow(100))
			}
			tx, err := db.Begin()
			require.NoError(t, err)

			sut := &SubmodelDatabase{db: db, verificationMode: testCase.verificationMode}
			require.NoError(t, sut.lockSubmodelForListVerificationTx(contextWithABACDisabled(t), tx, "sm-1", testCase.path, testCase.element))
			require.NoError(t, mock.ExpectationsWereMet())
		})
	}
}

func TestVerifySubmodelElementHidesDetailsFromRestrictedCallers(t *testing.T) {
	t.Parallel()
	sut := &SubmodelDatabase{verificationMode: gen.VerificationModeStrict}
	invalidIDShort := "1invalid"
	element := types.NewProperty(types.DataTypeDefXSDInt)
	element.SetIDShort(&invalidIDShort)

	detailedErr := sut.verifySubmodelElement(element, "SMREPO-TEST-VERIFY", true)
	hiddenErr := sut.verifySubmodelElement(element, "SMREPO-TEST-VERIFY", false)

	require.ErrorContains(t, detailedErr, "ID-short")
	require.ErrorContains(t, hiddenErr, "SMREPO-TEST-VERIFY the resulting submodel element violates metamodel constraints")
	require.NotContains(t, hiddenErr.Error(), "ID-short")
}
