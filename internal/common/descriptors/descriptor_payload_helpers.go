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

package descriptors

import (
	"bytes"
	"encoding/json"
	"fmt"

	"github.com/FriedJannik/aas-go-sdk/jsonization"
	"github.com/FriedJannik/aas-go-sdk/types"
)

func buildLangStringTextPayload(values []types.ILangStringTextType) (json.RawMessage, error) {
	if len(values) == 0 {
		return json.RawMessage("[]"), nil
	}

	out := make([]map[string]any, 0, len(values))
	for _, value := range values {
		jsonable, err := jsonization.ToJsonable(value)
		if err != nil {
			return nil, fmt.Errorf("build LangStringText payload: %w", err)
		}
		out = append(out, jsonable)
	}

	payload, err := json.Marshal(out)
	if err != nil {
		return nil, fmt.Errorf("marshal LangStringText payload: %w", err)
	}
	return payload, nil
}

func buildLangStringNamePayload(values []types.ILangStringNameType) (json.RawMessage, error) {
	if len(values) == 0 {
		return json.RawMessage("[]"), nil
	}

	out := make([]map[string]any, 0, len(values))
	for _, value := range values {
		jsonable, err := jsonization.ToJsonable(value)
		if err != nil {
			return nil, fmt.Errorf("build LangStringName payload: %w", err)
		}
		out = append(out, jsonable)
	}

	payload, err := json.Marshal(out)
	if err != nil {
		return nil, fmt.Errorf("marshal LangStringName payload: %w", err)
	}
	return payload, nil
}

func buildAdministrativeInfoPayload(value types.IAdministrativeInformation) (json.RawMessage, error) {
	if value == nil {
		return json.RawMessage("{}"), nil
	}

	jsonable, err := jsonization.ToJsonable(value)
	if err != nil {
		return nil, fmt.Errorf("build AdministrativeInformation payload: %w", err)
	}

	payload, err := json.Marshal(jsonable)
	if err != nil {
		return nil, fmt.Errorf("marshal AdministrativeInformation payload: %w", err)
	}
	return payload, nil
}

func parseLangStringTextPayload(payload json.RawMessage) ([]types.ILangStringTextType, error) {
	payload = bytes.TrimSpace(payload)
	if len(payload) == 0 || bytes.Equal(payload, []byte("null")) || bytes.Equal(payload, []byte("[]")) {
		return nil, nil
	}

	var items []map[string]any
	if err := json.Unmarshal(payload, &items); err != nil {
		return nil, fmt.Errorf("unmarshal LangStringText payload: %w", err)
	}
	if len(items) == 0 {
		return nil, nil
	}

	out := make([]types.ILangStringTextType, 0, len(items))
	for _, item := range items {
		langString, err := jsonization.LangStringTextTypeFromJsonable(item)
		if err != nil {
			return nil, fmt.Errorf("parse LangStringText payload item: %w", err)
		}
		out = append(out, langString)
	}

	return out, nil
}

func parseLangStringNamePayload(payload json.RawMessage) ([]types.ILangStringNameType, error) {
	payload = bytes.TrimSpace(payload)
	if len(payload) == 0 || bytes.Equal(payload, []byte("null")) || bytes.Equal(payload, []byte("[]")) {
		return nil, nil
	}

	var items []map[string]any
	if err := json.Unmarshal(payload, &items); err != nil {
		return nil, fmt.Errorf("unmarshal LangStringName payload: %w", err)
	}
	if len(items) == 0 {
		return nil, nil
	}

	out := make([]types.ILangStringNameType, 0, len(items))
	for _, item := range items {
		langString, err := jsonization.LangStringNameTypeFromJsonable(item)
		if err != nil {
			return nil, fmt.Errorf("parse LangStringName payload item: %w", err)
		}
		out = append(out, langString)
	}

	return out, nil
}

func parseAdministrativeInfoPayload(payload json.RawMessage) (types.IAdministrativeInformation, error) {
	payload = bytes.TrimSpace(payload)
	if len(payload) == 0 || bytes.Equal(payload, []byte("null")) || bytes.Equal(payload, []byte("{}")) {
		return nil, nil
	}

	var item map[string]any
	if err := json.Unmarshal(payload, &item); err != nil {
		return nil, fmt.Errorf("unmarshal AdministrativeInformation payload: %w", err)
	}
	if len(item) == 0 {
		return nil, nil
	}

	admin, err := jsonization.AdministrativeInformationFromJsonable(item)
	if err != nil {
		return nil, fmt.Errorf("parse AdministrativeInformation payload: %w", err)
	}
	return admin, nil
}

func parseReferencePayload(payload json.RawMessage) (types.IReference, error) {
	payload = bytes.TrimSpace(payload)
	if len(payload) == 0 || bytes.Equal(payload, []byte("null")) || bytes.Equal(payload, []byte("{}")) || bytes.Equal(payload, []byte("[]")) {
		return nil, nil
	}

	var item map[string]any
	if err := json.Unmarshal(payload, &item); err != nil {
		return nil, fmt.Errorf("unmarshal Reference payload: %w", err)
	}
	if len(item) == 0 {
		return nil, nil
	}

	ref, err := jsonization.ReferenceFromJsonable(item)
	if err != nil {
		return nil, fmt.Errorf("parse Reference payload: %w", err)
	}
	return ref, nil
}

func buildExtensionsPayload(values []types.Extension) (json.RawMessage, error) {
	if len(values) == 0 {
		return json.RawMessage("[]"), nil
	}

	out := make([]map[string]any, 0, len(values))
	for i := range values {
		jsonable, err := jsonization.ToJsonable(&values[i])
		if err != nil {
			return nil, fmt.Errorf("build Extension payload: %w", err)
		}
		out = append(out, jsonable)
	}

	payload, err := json.Marshal(out)
	if err != nil {
		return nil, fmt.Errorf("marshal Extension payload: %w", err)
	}
	return payload, nil
}

func parseExtensionsPayload(payload json.RawMessage) ([]types.Extension, error) {
	payload = bytes.TrimSpace(payload)
	if len(payload) == 0 || bytes.Equal(payload, []byte("null")) || bytes.Equal(payload, []byte("[]")) {
		return nil, nil
	}

	var items []map[string]any
	if err := json.Unmarshal(payload, &items); err != nil {
		return nil, fmt.Errorf("unmarshal Extension payload: %w", err)
	}
	if len(items) == 0 {
		return nil, nil
	}

	out := make([]types.Extension, 0, len(items))
	for _, item := range items {
		extension, err := jsonization.ExtensionFromJsonable(item)
		if err != nil {
			return nil, fmt.Errorf("parse Extension payload item: %w", err)
		}
		convExt, ok := extension.(*types.Extension)
		if !ok || convExt == nil {
			return nil, fmt.Errorf("parse Extension payload item: unexpected extension type")
		}
		out = append(out, *convExt)
	}

	return out, nil
}
