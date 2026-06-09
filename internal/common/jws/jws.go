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
	"crypto/rsa"
	"crypto/x509"
	"encoding/pem"
	"fmt"
	"os"

	jose "gopkg.in/go-jose/go-jose.v2"
)

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

// SignPayload returns a compact RS256 JWS over payload.
//
// Parameters:
//   - privateKey: RSA private key used for RS256 signing.
//   - payload: Canonical payload bytes to sign.
//
// Returns:
//   - string: Compact serialized JWS.
//   - error: Error when the key is nil, the signer cannot be created, or
//     serialization fails.
func SignPayload(privateKey *rsa.PrivateKey, payload []byte) (string, error) {
	if privateKey == nil {
		return "", fmt.Errorf("JWS-SIGN-NILKEY private key must not be nil")
	}
	signer, err := jose.NewSigner(jose.SigningKey{Algorithm: jose.RS256, Key: privateKey}, nil)
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
