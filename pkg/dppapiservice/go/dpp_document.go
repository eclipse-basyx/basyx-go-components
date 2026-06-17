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
// Author: Jannik Fried ( Fraunhofer IESE ), Aaron Zielstorff ( Fraunhofer IESE )

package dppapi

import (
	"bytes"
	"encoding/json"
	"fmt"
	"strings"
	"time"
)

const (
	headerDigitalProductPassportID = "digitalProductPassportId"
	headerUniqueProductIdentifier  = "uniqueProductIdentifier"
	headerGranularity              = "granularity"
	headerDppSchemaVersion         = "dppSchemaVersion"
	headerDppStatus                = "dppStatus"
	headerLastUpdate               = "lastUpdate"
	headerEconomicOperatorID       = "economicOperatorId"
	headerFacilityID               = "facilityId"
	headerContentSpecificationIDs  = "contentSpecificationIds"
)

var dppHeaderFields = map[string]struct{}{
	headerDigitalProductPassportID: {},
	headerUniqueProductIdentifier:  {},
	headerGranularity:              {},
	headerDppSchemaVersion:         {},
	headerDppStatus:                {},
	headerLastUpdate:               {},
	headerEconomicOperatorID:       {},
	headerFacilityID:               {},
	headerContentSpecificationIDs:  {},
}

var validGranularities = map[string]struct{}{
	"Item":          {},
	"Model":         {},
	"Batch":         {},
	"Role":          {},
	"NotApplicable": {},
}

type dppDocument map[string]any

type dppHeader struct {
	DigitalProductPassportID string
	UniqueProductIdentifier  string
	Granularity              string
	DppSchemaVersion         string
	DppStatus                string
	LastUpdate               time.Time
	EconomicOperatorID       string
	FacilityID               string
	ContentSpecificationIDs  []string
}

func decodeDPPDocument(data []byte, requireHeaders bool) (dppDocument, dppHeader, error) {
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.UseNumber()

	var doc dppDocument
	if err := decoder.Decode(&doc); err != nil {
		return nil, dppHeader{}, fmt.Errorf("DPP-DECDOC-DECODE decode request body: %w", err)
	}
	if doc == nil {
		return nil, dppHeader{}, fmt.Errorf("DPP-DECDOC-EMPTY document must be a JSON object")
	}

	header, err := parseDPPHeader(doc, requireHeaders)
	if err != nil {
		return nil, dppHeader{}, err
	}
	return doc, header, nil
}

func parseDPPHeader(doc dppDocument, requireHeaders bool) (dppHeader, error) {
	header := dppHeader{}

	var err error
	header.DigitalProductPassportID, err = stringField(doc, headerDigitalProductPassportID, requireHeaders)
	if err != nil {
		return header, err
	}
	header.UniqueProductIdentifier, err = stringField(doc, headerUniqueProductIdentifier, requireHeaders)
	if err != nil {
		return header, err
	}
	header.Granularity, err = granularityField(doc, requireHeaders)
	if err != nil {
		return header, err
	}
	header.DppSchemaVersion, err = stringField(doc, headerDppSchemaVersion, requireHeaders)
	if err != nil {
		return header, err
	}
	header.DppStatus, err = stringField(doc, headerDppStatus, requireHeaders)
	if err != nil {
		return header, err
	}
	header.EconomicOperatorID, err = stringField(doc, headerEconomicOperatorID, requireHeaders)
	if err != nil {
		return header, err
	}
	header.FacilityID, err = stringField(doc, headerFacilityID, requireHeaders)
	if err != nil {
		return header, err
	}
	header.LastUpdate, err = timeField(doc, headerLastUpdate, requireHeaders)
	if err != nil {
		return header, err
	}
	header.ContentSpecificationIDs, err = stringSliceField(doc, headerContentSpecificationIDs, requireHeaders)
	if err != nil {
		return header, err
	}
	return header, nil
}

func granularityField(doc dppDocument, required bool) (string, error) {
	value, err := stringField(doc, headerGranularity, required)
	if err != nil || value == "" {
		return value, err
	}
	if _, ok := validGranularities[value]; !ok {
		return "", fmt.Errorf("DPP-HEADER-GRANULARITY field %s must be one of Item, Model, Batch, Role, NotApplicable", headerGranularity)
	}
	return value, nil
}

func stringField(doc dppDocument, name string, required bool) (string, error) {
	value, ok := doc[name]
	if !ok {
		if required {
			return "", fmt.Errorf("DPP-HEADER-MISSING missing required field %s", name)
		}
		return "", nil
	}
	text, ok := value.(string)
	if !ok || strings.TrimSpace(text) == "" {
		return "", fmt.Errorf("DPP-HEADER-INVALID field %s must be a non-empty string", name)
	}
	return text, nil
}

func timeField(doc dppDocument, name string, required bool) (time.Time, error) {
	value, ok := doc[name]
	if !ok {
		if required {
			return time.Time{}, fmt.Errorf("DPP-HEADER-MISSING missing required field %s", name)
		}
		return time.Time{}, nil
	}
	text, ok := value.(string)
	if !ok {
		return time.Time{}, fmt.Errorf("DPP-HEADER-INVALID field %s must be an RFC3339 timestamp", name)
	}
	parsed, err := time.Parse(time.RFC3339Nano, text)
	if err != nil {
		return time.Time{}, fmt.Errorf("DPP-HEADER-PARSETIME parse %s: %w", name, err)
	}
	return parsed.UTC(), nil
}

func stringSliceField(doc dppDocument, name string, required bool) ([]string, error) {
	value, ok := doc[name]
	if !ok {
		if required {
			return nil, fmt.Errorf("DPP-HEADER-MISSING missing required field %s", name)
		}
		return nil, nil
	}
	rawItems, ok := value.([]any)
	if !ok {
		return nil, fmt.Errorf("DPP-HEADER-INVALID field %s must be an array of strings", name)
	}
	items := make([]string, 0, len(rawItems))
	for _, rawItem := range rawItems {
		item, ok := rawItem.(string)
		if !ok || strings.TrimSpace(item) == "" {
			return nil, fmt.Errorf("DPP-HEADER-INVALID field %s must contain only non-empty strings", name)
		}
		items = append(items, item)
	}
	if required && len(items) == 0 {
		return nil, fmt.Errorf("DPP-HEADER-MISSING field %s must not be empty", name)
	}
	return items, nil
}

func contentSections(doc dppDocument) map[string]any {
	sections := make(map[string]any)
	for key, value := range doc {
		if _, isHeader := dppHeaderFields[key]; !isHeader {
			sections[key] = value
		}
	}
	return sections
}

func applyMergePatch(target any, patch any) any {
	patchObject := dppObjectFromAny(patch)
	if patchObject == nil {
		return patch
	}

	targetObject := dppObjectFromAny(target)
	if targetObject == nil {
		targetObject = make(map[string]any)
	}
	for key, patchValue := range patchObject {
		if patchValue == nil {
			delete(targetObject, key)
			continue
		}
		targetObject[key] = applyMergePatch(targetObject[key], patchValue)
	}
	return targetObject
}

func lowerFirst(value string) string {
	if value == "" {
		return value
	}
	return strings.ToLower(value[:1]) + value[1:]
}

func upperFirst(value string) string {
	if value == "" {
		return value
	}
	return strings.ToUpper(value[:1]) + value[1:]
}
