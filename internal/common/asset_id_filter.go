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
	"encoding/json"
	"fmt"
	"strings"

	"github.com/FriedJannik/aas-go-sdk/jsonization"
	"github.com/FriedJannik/aas-go-sdk/types"
)

// GlobalAssetIDAssetLinkName is the reserved asset-link name for globalAssetId lookup.
const GlobalAssetIDAssetLinkName = "globalAssetId"

// AssetIDFilter contains decoded SpecificAssetId query parameters.
type AssetIDFilter struct {
	globalAssetIDs   map[string]struct{}
	specificAssetIDs map[string]struct{}
}

// DecodeAssetIDFilter decodes the base64url-encoded SpecificAssetId query parameters.
func DecodeAssetIDFilter(encodedAssetIDs []string) (AssetIDFilter, error) {
	filter := AssetIDFilter{
		globalAssetIDs:   make(map[string]struct{}),
		specificAssetIDs: make(map[string]struct{}),
	}
	for index, encodedAssetID := range encodedAssetIDs {
		if strings.TrimSpace(encodedAssetID) == "" {
			continue
		}
		assetID, err := decodeSpecificAssetID(encodedAssetID)
		if err != nil {
			return AssetIDFilter{}, NewErrBadRequest(fmt.Sprintf("COMMON-ASSETIDFILTER-DECODE assetIds[%d]: %v", index, err))
		}
		if assetID.Name() == "" || assetID.Value() == "" {
			return AssetIDFilter{}, NewErrBadRequest(fmt.Sprintf("COMMON-ASSETIDFILTER-EMPTY assetIds[%d]: name and value must not be empty", index))
		}
		if assetID.Name() == GlobalAssetIDAssetLinkName {
			filter.globalAssetIDs[assetID.Value()] = struct{}{}
			continue
		}
		key, err := specificAssetIDKey(assetID)
		if err != nil {
			return AssetIDFilter{}, NewErrBadRequest(fmt.Sprintf("COMMON-ASSETIDFILTER-NORMALIZE assetIds[%d]: %v", index, err))
		}
		filter.specificAssetIDs[key] = struct{}{}
	}
	return filter, nil
}

// IsEmpty reports whether no usable filter value was supplied.
func (f AssetIDFilter) IsEmpty() bool {
	return len(f.globalAssetIDs) == 0 && len(f.specificAssetIDs) == 0
}

// Matches reports whether the asset information matches at least one decoded identifier.
func (f AssetIDFilter) Matches(globalAssetID string, specificAssetIDs []types.ISpecificAssetID) (bool, error) {
	if f.IsEmpty() {
		return true, nil
	}
	if _, ok := f.globalAssetIDs[globalAssetID]; ok {
		return true, nil
	}
	for _, assetID := range specificAssetIDs {
		if assetID == nil {
			continue
		}
		key, err := specificAssetIDKey(assetID)
		if err != nil {
			return false, fmt.Errorf("COMMON-ASSETIDFILTER-NORMALIZEACTUAL: %w", err)
		}
		if _, ok := f.specificAssetIDs[key]; ok {
			return true, nil
		}
	}
	return false, nil
}

func decodeSpecificAssetID(encodedAssetID string) (types.ISpecificAssetID, error) {
	decoded, err := DecodeString(encodedAssetID)
	if err != nil {
		return nil, err
	}
	var jsonable map[string]any
	if err = json.Unmarshal([]byte(decoded), &jsonable); err != nil {
		return nil, err
	}
	return jsonization.SpecificAssetIDFromJsonable(jsonable)
}

func specificAssetIDKey(assetID types.ISpecificAssetID) (string, error) {
	jsonable, err := jsonization.ToJsonable(assetID)
	if err != nil {
		return "", err
	}
	data, err := json.Marshal(jsonable)
	if err != nil {
		return "", err
	}
	return string(data), nil
}
