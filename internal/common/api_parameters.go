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

package common

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/url"
	"strconv"
	"strings"
	"unicode/utf8"

	"github.com/FriedJannik/aas-go-sdk/jsonization"
	"github.com/FriedJannik/aas-go-sdk/types"
	"github.com/FriedJannik/aas-go-sdk/verification"
	"github.com/eclipse-basyx/basyx-go-components/internal/common/model"
)

// DecodeAPIString decodes a nonempty UTF-8 base64url API parameter with optional padding.
func DecodeAPIString(encoded string) (string, error) {
	if encoded == "" {
		return "", NewErrBadRequest("COMMON-APIPARAM-EMPTY encoded value must not be empty")
	}
	for _, c := range encoded {
		if (c < 'A' || c > 'Z') && (c < 'a' || c > 'z') && (c < '0' || c > '9') && c != '-' && c != '_' && c != '=' {
			return "", NewErrBadRequest("COMMON-APIPARAM-ALPHABET expected base64url")
		}
	}
	encoding := base64.RawURLEncoding.Strict()
	if strings.HasSuffix(encoded, "=") {
		encoding = base64.URLEncoding.Strict()
	}
	decoded, err := encoding.DecodeString(encoded)
	if err != nil {
		return "", NewErrBadRequest("COMMON-APIPARAM-DECODE " + err.Error())
	}
	if !utf8.Valid(decoded) {
		return "", NewErrBadRequest("COMMON-APIPARAM-UTF8 invalid UTF-8")
	}
	return string(decoded), nil
}

// DecodeAPICursor accepts the internal omitted-cursor sentinel.
func DecodeAPICursor(encoded string) (string, error) {
	if encoded == "" {
		return "", nil
	}
	return DecodeAPIString(encoded)
}

// ParseAPILimit preserves zero as the service's omitted-limit sentinel.
func ParseAPILimit(query url.Values) (int32, error) {
	if !query.Has("limit") {
		return 0, nil
	}
	n, err := strconv.ParseInt(query.Get("limit"), 10, 32)
	if err != nil || n < 1 {
		return 0, NewErrBadRequest("COMMON-APIPARAM-LIMIT limit must be a positive 32-bit integer")
	}
	return int32(n), nil
}

// ParseAPICursor preserves the wire token for decoding at the service boundary.
func ParseAPICursor(query url.Values) (string, error) {
	if !query.Has("cursor") {
		return "", nil
	}
	cursor := query.Get("cursor")
	if _, err := DecodeAPIString(cursor); err != nil {
		return "", err
	}
	return cursor, nil
}

// APIPagingMetadata encodes the backend continuation token exactly once.
func APIPagingMetadata(nextCursor string) model.PagedResultPagingMetadata {
	return model.PagedResultPagingMetadata{Cursor: EncodeString(nextCursor)}
}

// DecodeAPIReference validates a JSON Reference filter and its encoded length.
func DecodeAPIReference(encoded string, maxLength int) (types.IReference, error) {
	if maxLength > 0 && len(encoded) > maxLength {
		return nil, NewErrBadRequest("COMMON-APIREFERENCE-LENGTH encoded reference exceeds parameter length")
	}
	jsonable, err := decodeAPIObject(encoded)
	if err != nil {
		return nil, err
	}
	reference, err := jsonization.ReferenceFromJsonable(jsonable)
	if err != nil {
		return nil, NewErrBadRequest("COMMON-APIREFERENCE-JSON " + err.Error())
	}
	if err := verifyAPIObject(reference); err != nil {
		return nil, err
	}
	return reference, nil
}

// DecodeAPISpecificAssetID validates a JSON SpecificAssetId filter.
func DecodeAPISpecificAssetID(encoded string) (types.ISpecificAssetID, error) {
	jsonable, err := decodeAPIObject(encoded)
	if err != nil {
		return nil, err
	}
	assetID, err := jsonization.SpecificAssetIDFromJsonable(jsonable)
	if err != nil {
		return nil, NewErrBadRequest("COMMON-APIASSETID-JSON " + err.Error())
	}
	if err := verifyAPIObject(assetID); err != nil {
		return nil, err
	}
	return assetID, nil
}

func decodeAPIObject(encoded string) (map[string]any, error) {
	decoded, err := DecodeAPIString(encoded)
	if err != nil {
		return nil, err
	}
	var object map[string]any
	if err := json.Unmarshal([]byte(decoded), &object); err != nil {
		return nil, NewErrBadRequest("COMMON-APIOBJECT-JSON " + err.Error())
	}
	if object == nil {
		return nil, NewErrBadRequest("COMMON-APIOBJECT-NULL expected an object")
	}
	return object, nil
}

func verifyAPIObject(object types.IClass) error {
	if err := verifyAPICharacterLengths(object); err != nil {
		return err
	}
	var validationErr error
	verification.Verify(object, func(err *verification.VerificationError) bool {
		if err.Message == "Identifier shall have a maximum length of 2048 characters." || err.Message == "Label type shall have a maximum length of 64 characters." {
			return false
		}
		validationErr = NewErrBadRequest(fmt.Sprintf("COMMON-APIOBJECT-VERIFY %s", err.Message))
		return true
	})
	return validationErr
}

// ValidateAPIEncodedQuery checks presence and wire constraints without changing the token.
func ValidateAPIEncodedQuery(query url.Values, name string, maxLength int) error {
	if !query.Has(name) {
		return nil
	}
	value := query.Get(name)
	if maxLength > 0 && len(value) > maxLength {
		return NewErrBadRequest("COMMON-APIPARAM-LENGTH " + name + " exceeds parameter length")
	}
	_, err := DecodeAPIString(value)
	return err
}

// ValidateAPIPagination protects direct service calls, including wrapper early returns.
func ValidateAPIPagination(limit int32, cursor string) (string, error) {
	if limit < 0 {
		return "BadLimit", NewErrBadRequest("COMMON-APIPARAM-LIMIT limit must not be negative")
	}
	_, err := DecodeAPICursor(cursor)
	return "BadCursor", err
}

// GlobalAssetIDAssetLinkName is the reserved name for global asset identifier lookup.
const GlobalAssetIDAssetLinkName = "globalAssetId"

// DecodeAPISpecificAssetIDs preserves repeated query parameters without trimming or splitting.
func DecodeAPISpecificAssetIDs(encoded []string) ([]types.ISpecificAssetID, error) {
	result := make([]types.ISpecificAssetID, 0, len(encoded))
	for _, token := range encoded {
		value, err := DecodeAPISpecificAssetID(token)
		if err != nil {
			return nil, err
		}
		result = append(result, value)
	}
	return result, nil
}

// DecodeAPIIdentifier validates the decoded metamodel identifier, counting Unicode characters.
func DecodeAPIIdentifier(encoded string) (string, error) {
	value, err := DecodeAPIString(encoded)
	if err != nil {
		return "", err
	}
	if utf8.RuneCountInString(value) > 2048 || !verification.MatchesXMLSerializableString(value) {
		return "", NewErrBadRequest("COMMON-APIIDENTIFIER-CONSTRAINT invalid identifier length or characters")
	}
	return value, nil
}

func verifyAPICharacterLengths(object types.IClass) error {
	err := verifyAPIClassCharacterLengths(object)
	if err != nil {
		return err
	}
	object.Descend(func(child types.IClass) bool { err = verifyAPIClassCharacterLengths(child); return err != nil })
	return err
}

func verifyAPIClassCharacterLengths(object types.IClass) error {
	switch value := object.(type) {
	case types.IKey:
		if utf8.RuneCountInString(value.Value()) > 2048 {
			return NewErrBadRequest("COMMON-APIKEY-LENGTH value exceeds 2048 characters")
		}
	case types.ISpecificAssetID:
		if utf8.RuneCountInString(value.Name()) > 64 || utf8.RuneCountInString(value.Value()) > 2048 {
			return NewErrBadRequest("COMMON-APIASSETID-LENGTH name or value exceeds its character limit")
		}
	}
	return nil
}
