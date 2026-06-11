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

// Package jws provides utilities for handling JSON Web Signatures (JWS).
package jws

import (
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/base64"
	"encoding/hex"
	"encoding/pem"
	"fmt"
	"os"
	"strings"
	"time"

	jose "gopkg.in/go-jose/go-jose.v2"
)

// SigningOptions configures optional protected-header values for compact JWS
// responses created by this package.
//
// The signer always writes the mandatory RS256 algorithm header and the IDTA
// response metadata headers generated at signing time: "typ" with value "JWS",
// "sigT" with the UTC signature timestamp, and "sid" with a random signature
// identifier. SigningOptions only contains values that callers can provide from
// runtime configuration.
type SigningOptions struct {
	// CertificateChain contains DER encoded X.509 certificates as base64
	// strings, ordered from signer certificate to issuer certificates, for the
	// JWS "x5c" protected header. Leave it empty when no certificate chain
	// should be embedded in signed responses.
	CertificateChain []string
}

// LoadPrivateKey reads and parses an RSA private key from a PEM file.
//
// PKCS#8 keys are tried first and PKCS#1 RSA keys are accepted as a fallback.
//
// Parameters:
//   - path: Filesystem path to the PEM encoded private key.
//
// Returns:
//   - *rsa.PrivateKey: Parsed RSA private key.
//   - error: Error when the file cannot be read, decoded, or parsed as RSA.
func LoadPrivateKey(path string) (*rsa.PrivateKey, error) {
	//nolint:all // Ignore linter warnings for this function as it deals with cryptographic key loading.
	keyData, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("failed to read private key file: %w", err)
	}

	block, _ := pem.Decode(keyData)
	if block == nil {
		return nil, fmt.Errorf("failed to decode PEM block")
	}

	// Assume PKCS#8 format - if this wont work, PKCS#1 will be tried next
	key, err := x509.ParsePKCS8PrivateKey(block.Bytes)
	if err != nil {
		// PKCS#1 Fallback
		rsaKey, err := x509.ParsePKCS1PrivateKey(block.Bytes)
		if err != nil {
			return nil, fmt.Errorf("failed to parse private key: %w", err)
		}
		return rsaKey, nil
	}

	rsaKey, ok := key.(*rsa.PrivateKey)
	if !ok {
		return nil, fmt.Errorf("private key is not RSA")
	}
	return rsaKey, nil
}

// LoadPublicKey reads and parses an RSA public key from a PEM file.
//
// SubjectPublicKeyInfo and PKCS#1 RSA public keys are accepted so operators can
// use the public half of the existing manifest signing key material.
//
// Parameters:
//   - path: Filesystem path to the PEM encoded public key.
//
// Returns:
//   - *rsa.PublicKey: Parsed RSA public key.
//   - error: Error when the file cannot be read, decoded, or parsed as RSA.
func LoadPublicKey(path string) (*rsa.PublicKey, error) {
	// #nosec G304 -- the PEM path is an explicit operator configuration value.
	keyData, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("failed to read public key file: %w", err)
	}

	block, _ := pem.Decode(keyData)
	if block == nil {
		return nil, fmt.Errorf("failed to decode PEM block")
	}

	key, err := x509.ParsePKIXPublicKey(block.Bytes)
	if err == nil {
		rsaKey, ok := key.(*rsa.PublicKey)
		if !ok {
			return nil, fmt.Errorf("public key is not RSA")
		}
		return rsaKey, nil
	}

	rsaKey, pkcs1Err := x509.ParsePKCS1PublicKey(block.Bytes)
	if pkcs1Err != nil {
		return nil, fmt.Errorf("failed to parse public key: %w", pkcs1Err)
	}
	return rsaKey, nil
}

// LoadCertificateChain reads a PEM encoded X.509 certificate chain and converts
// it into the value format required by the JWS "x5c" protected header.
//
// The input file may contain one or more PEM blocks. Blocks with type
// "CERTIFICATE" are parsed as X.509 certificates, validated syntactically, and
// returned as standard-base64 encoded DER bytes. Non-certificate PEM blocks are
// ignored. Certificate order is preserved, so the configured file should list
// certificates in the order expected by JWS consumers, typically leaf signer
// certificate first and issuer certificates afterwards.
//
// Parameters:
//   - path: Filesystem path to a PEM file containing one or more
//     "CERTIFICATE" blocks.
//
// Returns:
//   - []string: Base64 encoded DER certificates suitable for the JWS "x5c"
//     protected header.
//   - error: Error when the file cannot be read, no certificate block is found,
//     or any certificate block cannot be parsed as X.509.
func LoadCertificateChain(path string) ([]string, error) {
	// #nosec G304 -- the PEM path is an explicit operator configuration value.
	certData, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("JWS-CERTCHAIN-READ failed to read certificate chain file: %w", err)
	}

	var chain []string
	remaining := certData
	for {
		block, rest := pem.Decode(remaining)
		if block == nil {
			break
		}
		remaining = rest
		if block.Type != "CERTIFICATE" {
			continue
		}
		if _, err = x509.ParseCertificate(block.Bytes); err != nil {
			return nil, fmt.Errorf("JWS-CERTCHAIN-PARSECERT failed to parse certificate: %w", err)
		}
		chain = append(chain, base64.StdEncoding.EncodeToString(block.Bytes))
	}
	if len(chain) == 0 {
		return nil, fmt.Errorf("JWS-CERTCHAIN-DECODE failed to decode certificate chain PEM")
	}
	return chain, nil
}

// LoadSigningOptions loads optional JWS signing configuration from filesystem
// paths.
//
// An empty or whitespace-only certificateChainPath means no certificate chain is
// configured; in that case the returned SigningOptions has an empty
// CertificateChain and no error. When a path is provided, the file is loaded with
// LoadCertificateChain and the resulting certificates are used for the "x5c"
// protected header.
//
// Parameters:
//   - certificateChainPath: Optional filesystem path to a PEM encoded X.509
//     certificate chain.
//
// Returns:
//   - SigningOptions: Header options to pass to SignPayloadWithOptions.
//   - error: Error when a non-empty certificateChainPath cannot be read or
//     parsed as a certificate chain.
func LoadSigningOptions(certificateChainPath string) (SigningOptions, error) {
	if strings.TrimSpace(certificateChainPath) == "" {
		return SigningOptions{}, nil
	}
	chain, err := LoadCertificateChain(certificateChainPath)
	if err != nil {
		return SigningOptions{}, fmt.Errorf("JWS-SIGNOPTIONS-CERTCHAIN %w", err)
	}
	return SigningOptions{CertificateChain: chain}, nil
}

// SignPayload returns a compact RS256 JWS over payload using the default
// BaSyx/IDTA protected headers.
//
// This is a convenience wrapper around SignPayloadWithOptions with empty
// SigningOptions. The generated compact JWS includes the RS256 algorithm header
// plus dynamic protected headers "typ", "sigT", and "sid". It does not include
// an "x5c" certificate chain header.
//
// Parameters:
//   - privateKey: RSA private key used for RS256 signing.
//   - payload: Payload bytes to sign. Callers that need deterministic payload
//     bytes should canonicalize JSON before calling this function.
//
// Returns:
//   - string: Compact serialized JWS string.
//   - error: Error when privateKey is nil, protected-header generation fails,
//     the signer cannot be created, signing fails, or compact serialization
//     fails.
func SignPayload(privateKey *rsa.PrivateKey, payload []byte) (string, error) {
	return SignPayloadWithOptions(privateKey, payload, SigningOptions{})
}

// SignPayloadWithOptions returns a compact RS256 JWS over payload with
// BaSyx/IDTA protected headers.
//
// The protected header contains:
//   - "alg": "RS256", written by go-jose for the RSA signing key.
//   - "typ": "JWS", identifying the compact response as a JWS.
//   - "sigT": Current UTC signing time formatted as RFC3339.
//   - "sid": A random UUID-style signature identifier generated per signature.
//   - "x5c": Optional certificate chain from options.CertificateChain.
//
// Parameters:
//   - privateKey: RSA private key used for RS256 signing.
//   - payload: Payload bytes to sign. Repository callers pass canonical JSON so
//     verifiers receive stable JSON payload bytes.
//   - options: Optional protected-header configuration, currently the
//     certificate chain for "x5c".
//
// Returns:
//   - string: Compact serialized JWS string.
//   - error: Error when privateKey is nil, protected-header generation fails,
//     the signer cannot be created, signing fails, or compact serialization
//     fails.
func SignPayloadWithOptions(privateKey *rsa.PrivateKey, payload []byte, options SigningOptions) (string, error) {
	if privateKey == nil {
		return "", fmt.Errorf("JWS-SIGN-NILKEY private key must not be nil")
	}
	signerOptions, err := newSignerOptions(options)
	if err != nil {
		return "", err
	}
	signer, err := jose.NewSigner(jose.SigningKey{Algorithm: jose.RS256, Key: privateKey}, signerOptions)
	if err != nil {
		return "", fmt.Errorf("JWS-SIGN-NEWSIGNER %w", err)
	}
	jws, err := signer.Sign(payload)
	if err != nil {
		return "", fmt.Errorf("JWS-SIGN-PAYLOAD %w", err)
	}
	signed, err := jws.CompactSerialize()
	if err != nil {
		return "", fmt.Errorf("JWS-SIGN-SERIALIZE %w", err)
	}
	return signed, nil
}

func newSignerOptions(options SigningOptions) (*jose.SignerOptions, error) {
	sid, err := newSignatureID()
	if err != nil {
		return nil, err
	}

	signerOptions := (&jose.SignerOptions{}).WithType("JWS")
	signerOptions.WithHeader(jose.HeaderKey("sigT"), time.Now().UTC().Format(time.RFC3339))
	signerOptions.WithHeader(jose.HeaderKey("sid"), sid)
	if len(options.CertificateChain) > 0 {
		signerOptions.WithHeader(jose.HeaderKey("x5c"), options.CertificateChain)
	}
	return signerOptions, nil
}

func newSignatureID() (string, error) {
	var id [16]byte
	if _, err := rand.Read(id[:]); err != nil {
		return "", fmt.Errorf("JWS-SIGN-SID %w", err)
	}
	id[6] = (id[6] & 0x0f) | 0x40
	id[8] = (id[8] & 0x3f) | 0x80

	encoded := hex.EncodeToString(id[:])
	return strings.Join([]string{
		encoded[0:8],
		encoded[8:12],
		encoded[12:16],
		encoded[16:20],
		encoded[20:32],
	}, "-"), nil
}
