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

package model

import "encoding/json"

// CreatedSubmodelElement carries the committed path without adding metadata to the response JSON.
type CreatedSubmodelElement struct {
	Path  string
	Value any
}

// MarshalJSON serializes only the created element.
func (c CreatedSubmodelElement) MarshalJSON() ([]byte, error) {
	return json.Marshal(c.Value)
}

// CreatedSubmodelElementPath prefers the committed path and supports legacy service responses.
func CreatedSubmodelElementPath(body any, parentPath string, idShort *string) string {
	if created, ok := body.(CreatedSubmodelElement); ok && created.Path != "" {
		return created.Path
	}
	if idShort == nil || *idShort == "" {
		return ""
	}
	if parentPath == "" {
		return *idShort
	}
	return parentPath + "." + *idShort
}
