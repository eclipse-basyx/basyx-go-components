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
// Author: Jannik Fried (Fraunhofer IESE)

package persistence

import (
	"crypto/rand"
	"crypto/rsa"
	"errors"
	"strings"
	"testing"

	sqlmock "github.com/DATA-DOG/go-sqlmock"
	"github.com/stretchr/testify/require"
	jose "gopkg.in/go-jose/go-jose.v2"
)

func TestGetSignedSubmodelWithoutPrivateKeyReturnsError(t *testing.T) {
	t.Parallel()

	sut := &SubmodelDatabase{}

	jws, err := sut.GetSignedSubmodel("sm", false)
	require.Error(t, err)
	require.Empty(t, jws)
	require.Contains(t, err.Error(), "private key not loaded")
}

func TestGetSignedSubmodelPropagatesSubmodelLookupError(t *testing.T) {
	t.Parallel()

	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer func() {
		_ = db.Close()
	}()

	mock.MatchExpectationsInOrder(false)

	privateKey, err := rsa.GenerateKey(rand.Reader, 2048)
	require.NoError(t, err)

	sut := &SubmodelDatabase{db: db, privateKey: privateKey}
	mock.ExpectQuery(`SELECT .*`).WillReturnError(errors.New("lookup failed"))

	jws, err := sut.GetSignedSubmodel("sm", false)
	require.Error(t, err)
	require.Empty(t, jws)
	require.Contains(t, err.Error(), "lookup failed")
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestGetSignedSubmodelSignsFullRepresentation(t *testing.T) {
	t.Parallel()

	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer func() {
		_ = db.Close()
	}()

	mock.MatchExpectationsInOrder(false)

	privateKey, err := rsa.GenerateKey(rand.Reader, 2048)
	require.NoError(t, err)

	sut := &SubmodelDatabase{db: db, privateKey: privateKey}
	setupSignedSubmodelHappyPathExpectations(mock, "sm-signed")

	jwsCompact, err := sut.GetSignedSubmodel("sm-signed", false)
	require.NoError(t, err)
	require.NotEmpty(t, jwsCompact)
	require.Equal(t, 2, strings.Count(jwsCompact, "."))

	signed, err := jose.ParseSigned(jwsCompact)
	require.NoError(t, err)
	payload, err := signed.Verify(&privateKey.PublicKey)
	require.NoError(t, err)

	payloadString := string(payload)
	require.Contains(t, payloadString, "submodelElements")
	require.Contains(t, payloadString, "sm-signed")
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestGetSignedSubmodelSignsValueOnlyRepresentation(t *testing.T) {
	t.Parallel()

	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer func() {
		_ = db.Close()
	}()

	mock.MatchExpectationsInOrder(false)

	privateKey, err := rsa.GenerateKey(rand.Reader, 2048)
	require.NoError(t, err)

	sut := &SubmodelDatabase{db: db, privateKey: privateKey}
	setupSignedSubmodelHappyPathExpectations(mock, "sm-value-only")

	jwsCompact, err := sut.GetSignedSubmodel("sm-value-only", true)
	require.NoError(t, err)
	require.NotEmpty(t, jwsCompact)
	require.Equal(t, 2, strings.Count(jwsCompact, "."))

	signed, err := jose.ParseSigned(jwsCompact)
	require.NoError(t, err)
	payload, err := signed.Verify(&privateKey.PublicKey)
	require.NoError(t, err)

	payloadString := string(payload)
	require.NotContains(t, payloadString, "submodelIdentifier")
	require.Equal(t, "{}", payloadString)
	require.NoError(t, mock.ExpectationsWereMet())
}

func setupSignedSubmodelHappyPathExpectations(mock sqlmock.Sqlmock, submodelIdentifier string) {
	mock.ExpectQuery(`SELECT .*FROM "submodel" INNER JOIN "submodel_payload"`).
		WillReturnRows(sqlmock.NewRows([]string{
			"submodel_identifier",
			"id_short",
			"category",
			"kind",
			"description",
			"display_name",
			"administrative_information",
			"embedded_data_specification",
			"supplemental_semantic_ids",
			"extensions",
			"qualifiers",
			"semantic_id",
		}).AddRow(
			submodelIdentifier,
			"idShort",
			nil,
			nil,
			nil,
			nil,
			nil,
			nil,
			nil,
			nil,
			nil,
			nil,
		))

	mock.ExpectQuery(`SELECT .*"id".*FROM "submodel"`).
		WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow(1))

	mock.ExpectQuery(`SELECT .*"sme"\."idshort_path".*FROM "submodel_element" AS "sme"`).
		WillReturnRows(sqlmock.NewRows([]string{"id", "path"}))
}
