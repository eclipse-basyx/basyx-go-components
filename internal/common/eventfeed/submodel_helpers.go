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

package eventfeed

import "github.com/FriedJannik/aas-go-sdk/types"

// SemanticIDFromSubmodel returns the last key value of the submodel semanticId.
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

// PCNRecordIDShortsFromSubmodel extracts idShort values under the PCN Records element.
func PCNRecordIDShortsFromSubmodel(submodel types.ISubmodel) []string {
	if submodel == nil {
		return nil
	}
	for _, el := range submodel.SubmodelElements() {
		if el == nil || el.IDShort() == nil || *el.IDShort() != "Records" {
			continue
		}
		var children []types.ISubmodelElement
		switch typed := el.(type) {
		case types.ISubmodelElementCollection:
			children = typed.Value()
		case types.ISubmodelElementList:
			children = typed.Value()
		default:
			continue
		}
		out := make([]string, 0, len(children))
		for _, child := range children {
			if child == nil || child.IDShort() == nil {
				continue
			}
			if idShort := *child.IDShort(); idShort != "" {
				out = append(out, idShort)
			}
		}
		return out
	}
	return nil
}
