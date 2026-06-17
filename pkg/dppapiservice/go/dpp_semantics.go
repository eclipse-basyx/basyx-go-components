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

func semanticIDForSection(sectionName string, contentSpecificationIDs []string) (string, error) {
	if len(contentSpecificationIDs) == 0 {
		return "", nil
	}
	normalized := strings.ToLower(sectionName)
	var matches []string
	for _, id := range contentSpecificationIDs {
		candidate := strings.ToLower(strings.TrimSpace(id))
		if strings.HasPrefix(candidate, normalized) || strings.Contains(candidate, normalized+" ") || strings.Contains(candidate, normalized+"-") {
			matches = append(matches, id)
		}
	}
	if len(matches) > 1 {
		return "", fmt.Errorf("DPP-SEMSPEC-AMBIGUOUS contentSpecificationIds are ambiguous for section %s", sectionName)
	}
	if len(matches) == 1 {
		return matches[0], nil
	}
	if len(contentSpecificationIDs) == 1 {
		return contentSpecificationIDs[0], nil
	}
	return "", fmt.Errorf("DPP-SEMSPEC-MISSING no contentSpecificationId matches section %s", sectionName)
}
