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

package auth

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"

	"github.com/coreos/go-oidc/v3/oidc"
)

type accessTokenVerifier struct {
	verifier *oidc.IDTokenVerifier
}

// Verify validates a compact signed JWT access token.
//
// The implementation intentionally uses go-oidc's IDTokenVerifier for cryptographic
// and standard claim validation (issuer, signature/JWK, expiry and optional audience),
// while provider-specific token type indicators (for example Entra's "idtyp" or
// Hydra-specific custom claims) are treated as ordinary claims and can be enforced
// through scope/mapping/policy layers.
func (v *accessTokenVerifier) Verify(ctx context.Context, rawToken string) (Claims, error) {
	if err := validateCompactSignedJWT(rawToken); err != nil {
		return nil, err
	}

	token, err := v.verifier.Verify(ctx, rawToken)
	if err != nil {
		return nil, fmt.Errorf("COMMON-OIDC-VERIFYTOKEN verify token: %w", err)
	}

	var rawClaims json.RawMessage
	if err := token.Claims(&rawClaims); err != nil {
		return nil, fmt.Errorf("COMMON-OIDC-READTOKENCLAIMS read token claims: %w", err)
	}

	decoder := json.NewDecoder(bytes.NewReader(rawClaims))
	decoder.UseNumber()

	var claims Claims
	if err := decoder.Decode(&claims); err != nil {
		return nil, fmt.Errorf("COMMON-OIDC-DECODETOKENCLAIMS decode token claims: %w", err)
	}
	if claims == nil {
		return nil, fmt.Errorf("COMMON-OIDC-DECODETOKENCLAIMS token claims must be a JSON object")
	}
	return claims, nil
}
