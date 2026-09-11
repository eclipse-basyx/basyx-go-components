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

package events

import (
	"encoding/json"

	"github.com/FriedJannik/aas-go-sdk/types"
	"github.com/eclipse-basyx/basyx-go-components/internal/common/model"
)

// SemanticIDFromSubmodel returns the final semantic-reference key of submodel.
func SemanticIDFromSubmodel(submodel types.ISubmodel) string {
	if submodel == nil {
		return ""
	}
	ref := submodel.SemanticID()
	if ref == nil {
		return ""
	}
	keys := ref.Keys()
	if len(keys) == 0 || keys[len(keys)-1] == nil {
		return ""
	}
	return keys[len(keys)-1].Value()
}

func pcnRecordElements(submodel types.ISubmodel) []types.ISubmodelElement {
	if submodel == nil {
		return nil
	}
	for _, el := range submodel.SubmodelElements() {
		if el == nil || el.IDShort() == nil || *el.IDShort() != "Records" {
			continue
		}
		switch typed := el.(type) {
		case types.ISubmodelElementList:
			return typed.Value()
		case types.ISubmodelElementCollection:
			return typed.Value()
		default:
			return nil
		}
	}
	return nil
}

// PCNNewRecordValuesFromSubmodel returns value-only PCN records added since previous.
func PCNNewRecordValuesFromSubmodel(previous, submodel types.ISubmodel) []model.SubmodelElementValue {
	currentRecords := pcnRecordElements(submodel)
	if len(currentRecords) == 0 {
		return nil
	}

	previousCounts := pcnRecordCounts(previous)

	values := make([]model.SubmodelElementValue, 0, len(currentRecords))
	for _, record := range currentRecords {
		if record == nil {
			continue
		}
		key := pcnRecordIdentity(record)
		if key != "" && previousCounts[key] > 0 {
			previousCounts[key]--
			continue
		}
		value, err := model.SubmodelElementToValueOnly(record)
		if err != nil || value == nil {
			continue
		}
		values = append(values, value)
	}
	return values
}

func pcnRecordIdentity(record types.ISubmodelElement) string {
	if record == nil {
		return ""
	}
	value, err := model.SubmodelElementToValueOnly(record)
	if err != nil || value == nil {
		return ""
	}
	raw, err := json.Marshal(value)
	if err != nil {
		return ""
	}
	return string(raw)
}

func pcnRecordCounts(submodel types.ISubmodel) map[string]int {
	counts := map[string]int{}
	for _, record := range pcnRecordElements(submodel) {
		if key := pcnRecordIdentity(record); key != "" {
			counts[key]++
		}
	}
	return counts
}
