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
	"context"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/base64"
	"encoding/json"
	"errors"
	"math/big"
	"net/http"
	"net/http/httptest"
	"regexp"
	"strings"
	"testing"
	"time"

	sqlmock "github.com/DATA-DOG/go-sqlmock"
	"github.com/eclipse-basyx/basyx-go-components/internal/common"
	"github.com/stretchr/testify/require"
	jose "gopkg.in/go-jose/go-jose.v2"
)

func contextWithABACDisabled(t *testing.T) context.Context {
	t.Helper()

	cfg := &common.Config{}
	var cfgCtx context.Context
	handler := common.ConfigMiddleware(cfg)(http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
		cfgCtx = r.Context()
	}))
	handler.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, "/", nil))

	require.NotNil(t, cfgCtx)
	return cfgCtx
}

func TestGetSignedSubmodelWithoutPrivateKeyReturnsError(t *testing.T) {
	t.Parallel()

	sut := &SubmodelDatabase{}

	jws, err := sut.GetSignedSubmodel(contextWithABACDisabled(t), "sm")
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
	mock.ExpectQuery(`SELECT .*`).WillReturnError(errors.New("lookup failed"))

	jws, err := sut.GetSignedSubmodel(contextWithABACDisabled(t), "sm")
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
	certificateChain := []string{newTestCertificate(t, privateKey)}
	sut.SetJWSCertificateChain(certificateChain)
	setupSignedSubmodelHappyPathExpectations(mock, "sm-signed")

	jwsCompact, err := sut.GetSignedSubmodel(contextWithABACDisabled(t), "sm-signed")
	require.NoError(t, err)
	require.NotEmpty(t, jwsCompact)
	require.Equal(t, 2, strings.Count(jwsCompact, "."))

	signed, err := jose.ParseSigned(jwsCompact)
	require.NoError(t, err)
	payload, err := signed.Verify(&privateKey.PublicKey)
	require.NoError(t, err)
	requireCanonicalJSONPayload(t, payload)
	requireIDTAProtectedHeaders(t, jwsCompact, certificateChain)

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

	jwsCompact, err := sut.GetSignedSubmodelValueOnly(contextWithABACDisabled(t), "sm-value-only")
	require.NoError(t, err)
	require.NotEmpty(t, jwsCompact)
	require.Equal(t, 2, strings.Count(jwsCompact, "."))

	signed, err := jose.ParseSigned(jwsCompact)
	require.NoError(t, err)
	payload, err := signed.Verify(&privateKey.PublicKey)
	require.NoError(t, err)
	requireCanonicalJSONPayload(t, payload)
	requireIDTAProtectedHeaders(t, jwsCompact, nil)

	payloadString := string(payload)
	require.NotContains(t, payloadString, "submodelIdentifier")
	require.Equal(t, "{}", payloadString)
	require.NoError(t, mock.ExpectationsWereMet())
}

func requireIDTAProtectedHeaders(t *testing.T, jwsCompact string, certificateChain []string) {
	t.Helper()

	header := parseProtectedHeader(t, jwsCompact)
	require.Equal(t, "RS256", header["alg"])
	require.Equal(t, "JWS", header["typ"])
	require.Regexp(t, regexp.MustCompile(`^[0-9a-f]{8}-[0-9a-f]{4}-4[0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$`), header["sid"])

	signatureTime, ok := header["sigT"].(string)
	require.True(t, ok)
	_, err := time.Parse(time.RFC3339, signatureTime)
	require.NoError(t, err)

	if len(certificateChain) > 0 {
		require.Equal(t, []any{certificateChain[0]}, header["x5c"])
	} else {
		require.NotContains(t, header, "x5c")
	}
}

func newTestCertificate(t *testing.T, privateKey *rsa.PrivateKey) string {
	t.Helper()

	template := &x509.Certificate{
		SerialNumber: big.NewInt(1),
		Subject:      pkix.Name{CommonName: "signed-test"},
		NotBefore:    time.Now().Add(-time.Hour),
		NotAfter:     time.Now().Add(time.Hour),
	}
	der, err := x509.CreateCertificate(rand.Reader, template, template, &privateKey.PublicKey, privateKey)
	require.NoError(t, err)
	return base64.StdEncoding.EncodeToString(der)
}

func parseProtectedHeader(t *testing.T, jwsCompact string) map[string]any {
	t.Helper()

	parts := strings.Split(jwsCompact, ".")
	require.Len(t, parts, 3)
	headerBytes, err := base64.RawURLEncoding.DecodeString(parts[0])
	require.NoError(t, err)

	var header map[string]any
	require.NoError(t, json.Unmarshal(headerBytes, &header))
	return header
}

func requireCanonicalJSONPayload(t *testing.T, payload []byte) {
	t.Helper()

	canonical, err := common.CanonicalJSON(payload)
	require.NoError(t, err)
	require.Equal(t, string(canonical), string(payload))
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
