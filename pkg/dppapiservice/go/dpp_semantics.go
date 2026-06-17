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
	"fmt"
	"strings"

	"github.com/FriedJannik/aas-go-sdk/types"
)

var dppMetadataSemanticIDs = map[string]struct{}{
	dppMetadataSemanticID:  {},
	dppMetadataSemanticURN: {},
}

func hasDPPMetadataSemanticID(submodel types.ISubmodel) bool {
	if submodel == nil || submodel.SemanticID() == nil {
		return false
	}
	_, ok := dppMetadataSemanticIDs[referenceLastValue(submodel.SemanticID())]
	return ok
}

func semanticIDsForSections(sections map[string]any, contentSpecificationIDs []string) (map[string]string, error) {
	semanticIDs := make(map[string]string, len(sections))
	if len(sections) == 0 || len(contentSpecificationIDs) == 0 {
		return semanticIDs, nil
	}

	contentSpecificationSet := make(map[string]struct{}, len(contentSpecificationIDs))
	for _, id := range contentSpecificationIDs {
		contentSpecificationSet[id] = struct{}{}
	}
	for sectionName, section := range sections {
		semanticID, hasExplicitMapping, err := explicitSemanticIDForSection(sectionName, section)
		if err != nil {
			return nil, err
		}
		if !hasExplicitMapping {
			continue
		}
		if _, ok := contentSpecificationSet[semanticID]; !ok {
			return nil, fmt.Errorf("DPP-SEMSPEC-UNKNOWN section %s dictionaryReference must be listed in contentSpecificationIds", sectionName)
		}
		semanticIDs[sectionName] = semanticID
	}

	if len(semanticIDs) == len(sections) {
		return semanticIDs, nil
	}
	if len(sections) == 1 && len(contentSpecificationIDs) == 1 {
		for sectionName := range sections {
			semanticIDs[sectionName] = contentSpecificationIDs[0]
		}
		return semanticIDs, nil
	}

	return nil, fmt.Errorf("DPP-SEMSPEC-EXPLICIT content sections must define dictionaryReference when multiple contentSpecificationIds or content sections are present")
}

func explicitSemanticIDForSection(sectionName string, section any) (string, bool, error) {
	object, ok := section.(map[string]any)
	if !ok {
		return "", false, nil
	}
	objectType, ok := object["objectType"].(string)
	if !ok || objectType != "DataElementCollection" {
		return "", false, nil
	}
	dictionaryReference, ok := object["dictionaryReference"].(string)
	if !ok || strings.TrimSpace(dictionaryReference) == "" {
		return "", true, fmt.Errorf("DPP-SEMSPEC-DICTIONARY section %s DataElementCollection must define dictionaryReference", sectionName)
	}
	return dictionaryReference, true, nil
}
