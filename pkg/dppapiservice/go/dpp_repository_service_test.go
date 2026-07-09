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
// Author: Aaron Zielstorff ( Fraunhofer IESE )

package dppapi

import (
	"testing"

	"github.com/FriedJannik/aas-go-sdk/types"
)

func TestElementResponseSupportsScalarSubmodelElementListItems(t *testing.T) {
	item := scalarProperty("", "A", types.DataTypeDefXSDString)
	item.SetIDShort(nil)
	elementIDPath := "$['technicalData']['energyClasses'][0]"

	compressed, err := elementResponse(item, REPRESENTATION_COMPRESSED, elementIDPath)
	if err != nil {
		t.Fatalf("elementResponse() compressed error = %v", err)
	}
	if compressed != "A" {
		t.Fatalf("elementResponse() compressed = %#v, want A", compressed)
	}

	full, err := elementResponse(item, REPRESENTATION_FULL, elementIDPath)
	if err != nil {
		t.Fatalf("elementResponse() full error = %v", err)
	}
	fullElement, ok := full.(map[string]any)
	if !ok {
		t.Fatalf("elementResponse() full = %#v, want object", full)
	}
	if fullElement["elementId"] != "energyClasses0" {
		t.Fatalf("elementResponse() full elementId = %#v, want energyClasses0", fullElement["elementId"])
	}
}
