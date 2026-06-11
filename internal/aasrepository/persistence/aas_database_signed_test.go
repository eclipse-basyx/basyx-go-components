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
	"context"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/base64"
	"encoding/json"
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

func TestGetSignedAssetAdministrationShellSignsCanonicalRepresentation(t *testing.T) {
	t.Parallel()

	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer func() {
		_ = db.Close()
	}()

	privateKey, err := rsa.GenerateKey(rand.Reader, 2048)
	require.NoError(t, err)

	sut := &AssetAdministrationShellDatabase{db: db, privateKey: privateKey}
	certificateChain := []string{newAASSignedTestCertificate(t, privateKey)}
	sut.SetJWSCertificateChain(certificateChain)
	setupSignedAASHappyPathExpectations(mock, "aas-signed")

	jwsCompact, err := sut.GetSignedAssetAdministrationShell(aasSigningTestContext(t), "aas-signed")
	require.NoError(t, err)
	require.NotEmpty(t, jwsCompact)
	require.Equal(t, 2, strings.Count(jwsCompact, "."))

	signed, err := jose.ParseSigned(jwsCompact)
	require.NoError(t, err)
	payload, err := signed.Verify(&privateKey.PublicKey)
	require.NoError(t, err)
	requireAASSignedCanonicalJSONPayload(t, payload)
	requireAASSignedIDTAProtectedHeaders(t, jwsCompact, certificateChain)

	payloadString := string(payload)
	require.Contains(t, payloadString, "assetInformation")
	require.Contains(t, payloadString, "aas-signed")
	require.NoError(t, mock.ExpectationsWereMet())
}

func setupSignedAASHappyPathExpectations(mock sqlmock.Sqlmock, aasIdentifier string) {
	mock.ExpectQuery(`SELECT .*"id".*FROM "aas"`).
		WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow(int64(1)))

	mock.ExpectQuery(`SELECT .*FROM "aas" AS "aas"`).
		WillReturnRows(sqlmock.NewRows([]string{
			"aas_id",
			"id_short",
			"category",
			"displayname_payload",
			"description_payload",
			"administrative_information_payload",
			"embedded_data_specification_payload",
			"extensions_payload",
			"derived_from_payload",
			"asset_kind",
			"global_asset_id",
			"asset_type",
			"value",
			"content_type",
		}).AddRow(
			aasIdentifier,
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
			nil,
			nil,
		))

	mock.ExpectQuery(`SELECT .*FROM "specific_asset_id"`).
		WillReturnRows(sqlmock.NewRows([]string{"id", "name", "value", "semantic_id_payload"}))

	mock.ExpectQuery(`SELECT .*FROM "aas_submodel_reference" AS "aas_submodel_reference"`).
		WillReturnRows(sqlmock.NewRows([]string{"aas_id", "parent_reference_payload"}))
}

func aasSigningTestContext(t *testing.T) context.Context {
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

func requireAASSignedIDTAProtectedHeaders(t *testing.T, jwsCompact string, certificateChain []string) {
	t.Helper()

	header := parseAASSignedProtectedHeader(t, jwsCompact)
	require.Equal(t, "RS256", header["alg"])
	require.Equal(t, "JWS", header["typ"])
	require.Regexp(t, regexp.MustCompile(`^[0-9a-f]{8}-[0-9a-f]{4}-4[0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$`), header["sid"])

	signatureTime, ok := header["sigT"].(string)
	require.True(t, ok)
	_, err := time.Parse(time.RFC3339, signatureTime)
	require.NoError(t, err)
	require.Equal(t, []any{certificateChain[0]}, header["x5c"])
}

func parseAASSignedProtectedHeader(t *testing.T, jwsCompact string) map[string]any {
	t.Helper()

	parts := strings.Split(jwsCompact, ".")
	require.Len(t, parts, 3)
	headerBytes, err := base64.RawURLEncoding.DecodeString(parts[0])
	require.NoError(t, err)

	var header map[string]any
	require.NoError(t, json.Unmarshal(headerBytes, &header))
	return header
}

func requireAASSignedCanonicalJSONPayload(t *testing.T, payload []byte) {
	t.Helper()

	canonical, err := common.CanonicalJSON(payload)
	require.NoError(t, err)
	require.Equal(t, string(canonical), string(payload))
}

func newAASSignedTestCertificate(t *testing.T, privateKey *rsa.PrivateKey) string {
	t.Helper()

	template := &x509.Certificate{
		SerialNumber: big.NewInt(1),
		Subject:      pkix.Name{CommonName: "aas-signed-test"},
		NotBefore:    time.Now().Add(-time.Hour),
		NotAfter:     time.Now().Add(time.Hour),
	}
	der, err := x509.CreateCertificate(rand.Reader, template, template, &privateKey.PublicKey, privateKey)
	require.NoError(t, err)
	return base64.StdEncoding.EncodeToString(der)
}
