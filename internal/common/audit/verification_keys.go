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

package audit

import (
	"bytes"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/x509"
	"encoding/hex"
	"encoding/pem"
	"fmt"
	"os"
)

func publicKeyIdentifier(key *rsa.PublicKey) string {
	digest := sha256.Sum256(x509.MarshalPKCS1PublicKey(key))
	return hex.EncodeToString(digest[:])
}

func retainedVerificationKeys(path string, current *rsa.PublicKey) (map[string]*rsa.PublicKey, error) {
	keys := map[string]*rsa.PublicKey{}
	if current != nil {
		keys[publicKeyIdentifier(current)] = current
	}
	if path == "" {
		return keys, nil
	}
	data, err := os.ReadFile(path) // #nosec G304 -- path is the operator-configured verification key bundle.
	if err != nil {
		return nil, fmt.Errorf("AUDIT-VERIFYKEYS-READ: %w", err)
	}
	for len(bytes.TrimSpace(data)) > 0 {
		block, rest := pem.Decode(data)
		if block == nil {
			return nil, fmt.Errorf("AUDIT-VERIFYKEYS-PEM invalid public key bundle")
		}
		data = rest
		key, err := parseVerificationKey(block)
		if err != nil {
			return nil, err
		}
		keys[publicKeyIdentifier(key)] = key
	}
	return keys, nil
}

func parseVerificationKey(block *pem.Block) (*rsa.PublicKey, error) {
	if block.Type == "RSA PUBLIC KEY" {
		key, err := x509.ParsePKCS1PublicKey(block.Bytes)
		if err != nil {
			return nil, fmt.Errorf("AUDIT-VERIFYKEYS-PKCS1: %w", err)
		}
		return key, nil
	}
	if block.Type != "PUBLIC KEY" {
		return nil, fmt.Errorf("AUDIT-VERIFYKEYS-TYPE RSA public keys required")
	}
	parsed, err := x509.ParsePKIXPublicKey(block.Bytes)
	if err != nil {
		return nil, fmt.Errorf("AUDIT-VERIFYKEYS-PKIX: %w", err)
	}
	key, ok := parsed.(*rsa.PublicKey)
	if !ok {
		return nil, fmt.Errorf("AUDIT-VERIFYKEYS-ALGORITHM RSA public keys required")
	}
	return key, nil
}
