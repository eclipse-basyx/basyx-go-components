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
	"hash/fnv"
	"sort"
	"strconv"
	"strings"

	"github.com/FriedJannik/aas-go-sdk/types"
)

func metadataSubmodelID(dppID string) string {
	return dppID + "/submodels/DppMetadata"
}

func contentSubmodelID(dppID string, sectionName string) string {
	idShort := contentSectionIDShort(sectionName)
	if idShort == upperFirst(sectionName) {
		return dppID + "/submodels/" + idShort
	}
	return dppID + "/submodels/" + idShort + "-" + contentSectionHash(sectionName)
}

func contentSectionIDShort(sectionName string) string {
	if semanticName := semanticIDLocalName(sectionName); semanticName != "" {
		return sanitizeIDShort(upperFirst(semanticName), "Content")
	}
	return sanitizeIDShort(upperFirst(sectionName), "Content")
}

func semanticIDLocalName(value string) string {
	if !strings.Contains(value, "://") && !strings.HasPrefix(value, "urn:") {
		return ""
	}
	parts := strings.FieldsFunc(value, func(r rune) bool {
		return r == '/' || r == ':' || r == '#' || r == '?'
	})
	for index := len(parts) - 1; index >= 0; index-- {
		part := strings.TrimSpace(parts[index])
		if part == "" || !containsLetter(part) {
			continue
		}
		return part
	}
	return ""
}

func containsLetter(value string) bool {
	for _, r := range value {
		if r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' {
			return true
		}
	}
	return false
}

func contentSectionHash(value string) string {
	hash := fnv.New64a()
	_, _ = hash.Write([]byte(value))
	return strconv.FormatUint(hash.Sum64(), 16)
}

func submodelReference(submodelID string) types.IReference {
	return types.NewReference(types.ReferenceTypesModelReference, []types.IKey{
		types.NewKey(types.KeyTypesSubmodel, submodelID),
	})
}

func globalReference(value string) types.IReference {
	return types.NewReference(types.ReferenceTypesExternalReference, []types.IKey{
		types.NewKey(types.KeyTypesGlobalReference, value),
	})
}

func referenceLastValue(ref types.IReference) string {
	if ref == nil || len(ref.Keys()) == 0 {
		return ""
	}
	return ref.Keys()[len(ref.Keys())-1].Value()
}

func sortedKeys(values map[string]any) []string {
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}

func sanitizeIDShort(value string, fallback string) string {
	var builder strings.Builder
	for _, r := range value {
		if r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r >= '0' && r <= '9' || r == '_' {
			_, _ = builder.WriteRune(r)
		}
	}
	if builder.Len() == 0 {
		return fallback
	}
	return builder.String()
}
