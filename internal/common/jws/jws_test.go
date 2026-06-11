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

package jws

import (
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/base64"
	"encoding/pem"
	"math/big"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestLoadPublicKeyReportsPKCS1FallbackError(t *testing.T) {
	invalidDER := []byte{0x30, 0x00}
	_, pkcs1Err := x509.ParsePKCS1PublicKey(invalidDER)
	require.Error(t, pkcs1Err)
	keyPath := filepath.Join(t.TempDir(), "invalid-public.pem")
	pemData := pem.EncodeToMemory(&pem.Block{Type: "PUBLIC KEY", Bytes: invalidDER})
	require.NoError(t, os.WriteFile(keyPath, pemData, 0o600))

	_, err := LoadPublicKey(keyPath)

	require.ErrorContains(t, err, pkcs1Err.Error())
}

func TestLoadCertificateChainLoadsSingleCertificate(t *testing.T) {
	t.Parallel()

	cert := newTestCertificate(t, "single")
	path := writeCertificateChainFile(t, cert)

	chain, err := LoadCertificateChain(path)

	require.NoError(t, err)
	require.Equal(t, []string{base64.StdEncoding.EncodeToString(cert)}, chain)
}

func TestLoadCertificateChainPreservesCertificateOrder(t *testing.T) {
	t.Parallel()

	first := newTestCertificate(t, "first")
	second := newTestCertificate(t, "second")
	path := writeCertificateChainFile(t, first, second)

	chain, err := LoadCertificateChain(path)

	require.NoError(t, err)
	require.Equal(t, []string{
		base64.StdEncoding.EncodeToString(first),
		base64.StdEncoding.EncodeToString(second),
	}, chain)
}

func TestLoadCertificateChainRejectsMissingCertificateBlock(t *testing.T) {
	t.Parallel()

	path := filepath.Join(t.TempDir(), "empty-chain.pem")
	pemData := pem.EncodeToMemory(&pem.Block{Type: "PUBLIC KEY", Bytes: []byte{0x30, 0x00}})
	require.NoError(t, os.WriteFile(path, pemData, 0o600))

	chain, err := LoadCertificateChain(path)

	require.ErrorContains(t, err, "JWS-CERTCHAIN-DECODE")
	require.Empty(t, chain)
}

func TestLoadCertificateChainRejectsInvalidCertificateBlock(t *testing.T) {
	t.Parallel()

	path := filepath.Join(t.TempDir(), "invalid-chain.pem")
	pemData := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: []byte{0x30, 0x00}})
	require.NoError(t, os.WriteFile(path, pemData, 0o600))

	chain, err := LoadCertificateChain(path)

	require.ErrorContains(t, err, "JWS-CERTCHAIN-PARSECERT")
	require.Empty(t, chain)
}

func newTestCertificate(t *testing.T, commonName string) []byte {
	t.Helper()

	privateKey, err := rsa.GenerateKey(rand.Reader, 2048)
	require.NoError(t, err)
	template := &x509.Certificate{
		SerialNumber: big.NewInt(time.Now().UnixNano()),
		Subject:      pkix.Name{CommonName: commonName},
		NotBefore:    time.Now().Add(-time.Hour),
		NotAfter:     time.Now().Add(time.Hour),
	}
	der, err := x509.CreateCertificate(rand.Reader, template, template, &privateKey.PublicKey, privateKey)
	require.NoError(t, err)
	return der
}

func writeCertificateChainFile(t *testing.T, certificates ...[]byte) string {
	t.Helper()

	path := filepath.Join(t.TempDir(), "chain.pem")
	pemData := make([]byte, 0)
	for _, certificate := range certificates {
		pemData = append(pemData, pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: certificate})...)
	}
	require.NoError(t, os.WriteFile(path, pemData, 0o600))
	return path
}
