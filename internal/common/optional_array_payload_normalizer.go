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

func NormalizePayloadNullFields(payload any) {
	_ = normalizePayloadNullFields(payload)
}

func NormalizeAASPayloadOptionalArrays(payload any) {
	NormalizePayloadNullFields(payload)
}

func NormalizeSubmodelPayloadOptionalArrays(payload any) {
	NormalizePayloadNullFields(payload)
}

func normalizePayloadNullFields(payload any) any {
	switch typed := payload.(type) {
	case map[string]any:
		for key, value := range typed {
			if value == nil {
				delete(typed, key)
				continue
			}
			typed[key] = normalizePayloadNullFields(value)
		}
		return typed
	case []any:
		normalized := make([]any, 0, len(typed))
		for _, item := range typed {
			if item == nil {
				continue
			}
			normalized = append(normalized, normalizePayloadNullFields(item))
		}
		return normalized
	default:
		return payload
	}
}
