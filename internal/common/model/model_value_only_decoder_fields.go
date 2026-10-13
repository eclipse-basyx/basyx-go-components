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

package model

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/FriedJannik/aas-go-sdk/jsonization"
	"github.com/FriedJannik/aas-go-sdk/types"
	"github.com/FriedJannik/aas-go-sdk/verification"
)

func decodeValueOnlyJSON(raw json.RawMessage) (any, error) {
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.UseNumber()
	var value any
	if err := decoder.Decode(&value); err != nil {
		return nil, fmt.Errorf("MODEL-VALONLY-DECODE-JSON %w", err)
	}
	return value, nil
}

func decodeValueOnlyString(raw json.RawMessage) (string, error) {
	value, err := decodeValueOnlyJSON(raw)
	if err != nil {
		return "", err
	}
	text, ok := value.(string)
	if !ok {
		return "", fmt.Errorf("MODEL-VALONLY-DECODE-STRING expected JSON string but got JSON %s", jsonKind(value))
	}
	return text, nil
}

func decodeOptionalString(fields map[string]json.RawMessage, name string) (*string, error) {
	raw, ok := fields[name]
	if !ok {
		return nil, nil
	}
	text, err := decodeValueOnlyString(raw)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", name, err)
	}
	return &text, nil
}

func decodeContentType(fields map[string]json.RawMessage) (*string, error) {
	contentType, err := decodeOptionalString(fields, "contentType")
	if err != nil || contentType == nil || *contentType == "" {
		return contentType, err
	}
	return contentType, verifyValueOnly("MODEL-VALONLY-DECODE-CONTENTTYPEVERIFY", func(collector func(*verification.VerificationError) bool) {
		verification.VerifyContentType(*contentType, collector)
	})
}

func decodeStandardBase64(encoded string) ([]byte, error) {
	content, err := base64.StdEncoding.DecodeString(encoded)
	if err != nil {
		return nil, fmt.Errorf("MODEL-VALONLY-DECODE-BASE64 value is not valid base64: %w", err)
	}
	return content, nil
}

func decodeGlobalAssetID(fields map[string]json.RawMessage) (*string, error) {
	globalAssetID, err := decodeOptionalString(fields, "globalAssetId")
	if err != nil || globalAssetID == nil {
		return nil, err
	}
	return globalAssetID, verifyValueOnly("MODEL-VALONLY-DECODE-GLOBALASSETIDVERIFY", func(collector func(*verification.VerificationError) bool) {
		verification.VerifyIdentifier(*globalAssetID, collector)
	})
}

func decodeSpecificAssetIDs(fields map[string]json.RawMessage) (*[]types.ISpecificAssetID, error) {
	raw, ok := fields["specificAssetIds"]
	if !ok {
		return nil, nil
	}
	value, err := decodeValueOnlyJSON(raw)
	if err != nil {
		return nil, err
	}
	items, ok := value.([]any)
	if !ok {
		return nil, errors.New("MODEL-VALONLY-DECODE-SPECIFICASSETIDS expected JSON array")
	}
	result := make([]types.ISpecificAssetID, 0, len(items))
	for i, item := range items {
		assetID, err := decodeSpecificAssetID(item)
		if err != nil {
			return nil, fmt.Errorf("specificAssetIds[%d]: %w", i, err)
		}
		result = append(result, assetID)
	}
	return &result, nil
}

func decodeSpecificAssetID(jsonable any) (types.ISpecificAssetID, error) {
	assetID, err := jsonization.SpecificAssetIDFromJsonable(jsonable)
	if err != nil {
		return nil, fmt.Errorf("MODEL-VALONLY-DECODE-SPECIFICASSETID %w", err)
	}
	return assetID, verifyValueOnlyClass("MODEL-VALONLY-DECODE-SPECIFICASSETIDVERIFY", assetID)
}

func decodeOptionalReference(fields map[string]json.RawMessage, name string) (types.IReference, error) {
	raw, ok := fields[name]
	if !ok {
		return nil, nil
	}
	jsonable, err := decodeValueOnlyJSON(raw)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", name, err)
	}
	reference, err := decodeReferenceJsonable(jsonable)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", name, err)
	}
	return reference, nil
}

func decodeReferenceJsonable(jsonable any) (types.IReference, error) {
	reference, err := jsonization.ReferenceFromJsonable(jsonable)
	if err != nil {
		return nil, fmt.Errorf("MODEL-VALONLY-DECODE-REFERENCE %w", err)
	}
	return reference, verifyValueOnlyClass("MODEL-VALONLY-DECODE-REFERENCEVERIFY", reference)
}

func verifyValueOnlyClass(code string, class types.IClass) error {
	return verifyValueOnly(code, func(collector func(*verification.VerificationError) bool) {
		verification.Verify(class, collector)
	})
}

// verifyValueOnly applies the configured semantic verification mode, as full-model writes do.
func verifyValueOnly(code string, verify func(func(*verification.VerificationError) bool)) error {
	return ValidateWithMode(GetVerificationMode(), code, verify, func(message string) error {
		return fmt.Errorf("%s %s", code, message)
	})
}
