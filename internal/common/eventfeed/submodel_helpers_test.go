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

import (
	"testing"

	"github.com/FriedJannik/aas-go-sdk/types"
)

func pcnRecord(idShort, changeID string) *types.SubmodelElementCollection {
	prop := types.NewProperty(types.DataTypeDefXSDString)
	propIDShort := "ManufacturerChangeID"
	prop.SetIDShort(&propIDShort)
	prop.SetValue(&changeID)

	rec := types.NewSubmodelElementCollection()
	if idShort != "" {
		id := idShort
		rec.SetIDShort(&id)
	}
	rec.SetValue([]types.ISubmodelElement{prop})
	return rec
}

func pcnSubmodelWithCollectionRecords(t *testing.T, records ...*types.SubmodelElementCollection) types.ISubmodel {
	t.Helper()
	sm := types.NewSubmodel("sm-pcn")
	recordsIDShort := "Records"
	values := make([]types.ISubmodelElement, 0, len(records))
	for _, r := range records {
		values = append(values, r)
	}
	recordsElement := types.NewSubmodelElementCollection()
	recordsElement.SetIDShort(&recordsIDShort)
	recordsElement.SetValue(values)
	sm.SetSubmodelElements([]types.ISubmodelElement{recordsElement})
	return sm
}

func pcnSubmodelWithListRecords(t *testing.T, changeIDs ...string) types.ISubmodel {
	t.Helper()
	sm := types.NewSubmodel("sm-pcn")
	recordsIDShort := "Records"
	values := make([]types.ISubmodelElement, 0, len(changeIDs))
	for _, id := range changeIDs {
		values = append(values, pcnRecord("", id))
	}
	recordsElement := types.NewSubmodelElementList(types.AASSubmodelElementsSubmodelElementCollection)
	recordsElement.SetIDShort(&recordsIDShort)
	recordsElement.SetValue(values)
	sm.SetSubmodelElements([]types.ISubmodelElement{recordsElement})
	return sm
}

func TestPCNNewRecordValuesFromSubmodelOnCreate(t *testing.T) {
	sm := pcnSubmodelWithCollectionRecords(t, pcnRecord("Record0", "CN1"), pcnRecord("Record1", "CN2"))

	values := PCNNewRecordValuesFromSubmodel(nil, sm)
	if len(values) != 2 {
		t.Fatalf("expected 2 new records on create, got %d", len(values))
	}
}

func TestPCNNewRecordValuesFromSubmodelOnUpdateByIDShort(t *testing.T) {
	previous := pcnSubmodelWithCollectionRecords(t, pcnRecord("Record0", "CN1"))
	current := pcnSubmodelWithCollectionRecords(t, pcnRecord("Record0", "CN1"), pcnRecord("Record1", "CN2"))

	values := PCNNewRecordValuesFromSubmodel(previous, current)
	if len(values) != 1 {
		t.Fatalf("expected exactly 1 new record, got %d: %v", len(values), values)
	}
}

func TestPCNNewRecordValuesFromSubmodelNoChangeYieldsNoEvents(t *testing.T) {
	previous := pcnSubmodelWithCollectionRecords(t, pcnRecord("Record0", "CN1"))
	current := pcnSubmodelWithCollectionRecords(t, pcnRecord("Record0", "CN1"))

	values := PCNNewRecordValuesFromSubmodel(previous, current)
	if len(values) != 0 {
		t.Fatalf("expected no new records when Records is unchanged, got %d", len(values))
	}
}

func TestPCNNewRecordValuesFromSubmodelOnUpdateByPositionForListWithoutIDShorts(t *testing.T) {
	previous := pcnSubmodelWithListRecords(t, "CN1")
	current := pcnSubmodelWithListRecords(t, "CN1", "CN2")

	values := PCNNewRecordValuesFromSubmodel(previous, current)
	if len(values) != 1 {
		t.Fatalf("expected exactly 1 new record appended past previous length, got %d", len(values))
	}
}
