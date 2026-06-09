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
	"encoding/json"
	"errors"

	"github.com/FriedJannik/aas-go-sdk/jsonization"
	"github.com/FriedJannik/aas-go-sdk/types"
	gen "github.com/eclipse-basyx/basyx-go-components/internal/common/model"
	jose "gopkg.in/go-jose/go-jose.v2"
)

// GetSignedSubmodel retrieves and signs a submodel
// while preserving ABAC visibility checks from ctx.
//
// Parameters:
//   - ctx: Context
//   - submodelID: The Submodel ID to fetch
//
// Returns:
//   - string: Signed Payload
//   - error
func (s *SubmodelDatabase) GetSignedSubmodel(ctx context.Context, submodelID string) (string, error) {
	if s.privateKey == nil {
		return "", errors.New("JWS signing not configured: private key not loaded")
	}

	submodel, err := s.GetSubmodelByID(ctx, submodelID, "deep", false)
	if err != nil {
		return "", err
	}

	var payload []byte

	payload, err = getNormalPayload(submodel)
	if err != nil {
		return "", err
	}

	signer, err := jose.NewSigner(jose.SigningKey{Algorithm: jose.RS256, Key: s.privateKey}, nil)
	if err != nil {
		return "", err
	}

	jws, err := signer.Sign(payload)
	if err != nil {
		return "", err
	}

	return jws.CompactSerialize()
}

// GetSignedSubmodelValueOnly returns and signs a submodel in its value-only representation
// while preserving ABAC visibility checks from ctx.
//
// Parameters:
//   - ctx: Context
//   - submodelID: The Submodel ID to fetch
//
// Returns:
//   - string: Signed Payload
//   - error
func (s *SubmodelDatabase) GetSignedSubmodelValueOnly(ctx context.Context, submodelID string) (string, error) {
	if s.privateKey == nil {
		return "", errors.New("JWS signing not configured: private key not loaded")
	}

	submodel, err := s.GetSubmodelByID(ctx, submodelID, "deep", false)
	if err != nil {
		return "", err
	}

	payload, err := getValueOnlyPayload(submodel)
	if err != nil {
		return "", err
	}

	signer, err := jose.NewSigner(jose.SigningKey{Algorithm: jose.RS256, Key: s.privateKey}, nil)
	if err != nil {
		return "", err
	}

	jws, err := signer.Sign(payload)
	if err != nil {
		return "", err
	}

	return jws.CompactSerialize()
}

func getNormalPayload(submodel types.ISubmodel) ([]byte, error) {
	jsonSubmodel, convertErr := jsonization.ToJsonable(submodel)
	if convertErr != nil {
		return nil, convertErr
	}
	payload, err := json.Marshal(jsonSubmodel)
	if err != nil {
		return nil, err
	}
	return payload, err
}

func getValueOnlyPayload(submodel types.ISubmodel) ([]byte, error) {
	valueOnlySubmodel, conversionErr := gen.SubmodelToValueOnly(submodel)
	if conversionErr != nil {
		return nil, conversionErr
	}
	payload, err := json.Marshal(valueOnlySubmodel)
	if err != nil {
		return nil, err
	}
	return payload, err
}
