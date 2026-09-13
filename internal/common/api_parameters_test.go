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
	"net/url"
	"strings"
	"testing"
)

func TestDecodeAPIString(t *testing.T) {
	for _, value := range []string{"a", "ab", "urn:example:abc", "Grüße 世界", "\t\n"} {
		for _, encoding := range []*base64.Encoding{base64.RawURLEncoding, base64.URLEncoding} {
			got, err := DecodeAPIString(encoding.EncodeToString([]byte(value)))
			if err != nil || got != value {
				t.Fatalf("round trip %q: %q, %v", value, got, err)
			}
		}
	}
	for _, input := range []string{"", "YQ=", "YQ===", "Y=Q=", "YR==", "Y+Q", "Y/Q", "YQ\n", "YQ\r", "YQ==\n", " YQ", "YQ ", "a", "YR", "_w", "_w=="} {
		t.Run(input, func(t *testing.T) {
			if _, err := DecodeAPIString(input); err == nil {
				t.Fatal("accepted malformed parameter")
			}
		})
	}
	if value, err := DecodeString("YQ=="); err != nil || value != "a" {
		t.Fatal("legacy binary codec changed")
	}
}

func TestAPIPaginationPresence(t *testing.T) {
	if value, err := ParseAPILimit(url.Values{}); err != nil || value != 0 {
		t.Fatal("omitted limit")
	}
	for _, input := range []string{"", "0", "-1", "abc", "1.5", "2147483648"} {
		if _, err := ParseAPILimit(url.Values{"limit": {input}}); err == nil {
			t.Fatalf("accepted limit %q", input)
		}
	}
	for _, input := range []string{"1", "100", "2147483647"} {
		if _, err := ParseAPILimit(url.Values{"limit": {input}}); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := ParseAPICursor(url.Values{}); err != nil {
		t.Fatal(err)
	}
	if _, err := ParseAPICursor(url.Values{"cursor": {""}}); err == nil {
		t.Fatal("accepted present empty cursor")
	}
	token := EncodeString("urn:next")
	if got, err := ParseAPICursor(url.Values{"cursor": {token}}); err != nil || got != token {
		t.Fatal("controller altered cursor")
	}
	if got := APIPagingMetadata("urn:next").Cursor; got != token {
		t.Fatal("wrong next cursor")
	}
	if got := APIPagingMetadata("").Cursor; got != "" {
		t.Fatal("terminal cursor")
	}
}

func TestAPIReferenceFilters(t *testing.T) {
	valid := `{ "keys": [{"value":"urn:semantic", "type":"GlobalReference"}],
 "type": "ExternalReference" }`
	if _, err := DecodeAPIReference(EncodeString(valid), 3072); err != nil {
		t.Fatal(err)
	}
	for _, value := range []string{"null", "[]", `"urn:semantic"`, "{", `{}`, `{"type":"ExternalReference","keys":[]}`, `{"type":"ExternalReference","keys":[{"type":"GlobalReference","value":""}]}`} {
		if _, err := DecodeAPIReference(EncodeString(value), 3072); err == nil {
			t.Fatalf("accepted %s", value)
		}
	}
	if _, err := DecodeAPIReference(EncodeString(valid), 1); err == nil {
		t.Fatal("ignored length bound")
	}
}

func TestAPISpecificAssetIDFilters(t *testing.T) {
	if _, err := DecodeAPISpecificAssetID(EncodeString(`{"value":"asset", "name":"serial"}`)); err != nil {
		t.Fatal(err)
	}
	for _, value := range []string{"null", "[]", `{}`, `{"name":"serial"}`, `{"name":"","value":"asset"}`} {
		if _, err := DecodeAPISpecificAssetID(EncodeString(value)); err == nil {
			t.Fatalf("accepted %s", value)
		}
	}
}

func TestAPIUnicodeCharacterBounds(t *testing.T) {
	name := strings.Repeat("é", 64)
	value := strings.Repeat("中", 700)
	asset := `{"name":"` + name + `","value":"` + value + `"}`
	if _, err := DecodeAPISpecificAssetID(EncodeString(asset)); err != nil {
		t.Fatal(err)
	}
	reference := `{"type":"ExternalReference","keys":[{"type":"GlobalReference","value":"` + value + `"}]}`
	if _, err := DecodeAPIReference(EncodeString(reference), 3072); err != nil {
		t.Fatal(err)
	}
	if _, err := DecodeAPIIdentifier(EncodeString(strings.Repeat("é", 2048))); err != nil {
		t.Fatal(err)
	}
	if _, err := DecodeAPIIdentifier(EncodeString(strings.Repeat("é", 2049))); err == nil {
		t.Fatal("oversized identifier accepted")
	}
	for _, value := range []string{"\x00", "\uffff"} {
		if _, err := DecodeAPIIdentifier(EncodeString(value)); err == nil {
			t.Fatal("invalid XML character accepted")
		}
	}
}

func TestAPIEncodedReferenceLengthBoundary(t *testing.T) {
	reference := `{"type":"ExternalReference","keys":[{"type":"GlobalReference","value":"urn:test"}]}`
	reference += strings.Repeat(" ", 2304-len(reference))
	encoded := EncodeString(reference)
	if len(encoded) != 3072 {
		t.Fatal("fixture length")
	}
	if _, err := DecodeAPIReference(encoded, 3072); err != nil {
		t.Fatal(err)
	}
	if _, err := DecodeAPIReference(EncodeString(reference+" "), 3072); err == nil {
		t.Fatal("oversized parameter accepted")
	}
}
