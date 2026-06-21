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

package descriptors

import (
	"context"
	"strings"
	"testing"

	"github.com/FriedJannik/aas-go-sdk/types"
	"github.com/eclipse-basyx/basyx-go-components/internal/common/model"
)

func TestBuildAdministrationShellDescriptorCreateBatchCollectsCompleteGraph(t *testing.T) {
	t.Parallel()

	endpoint := model.Endpoint{
		Interface: "AAS-3.0",
		ProtocolInformation: model.ProtocolInformation{
			Href: "https://example.com/aas",
		},
	}
	descriptor := model.AssetAdministrationShellDescriptor{
		Id:               "urn:example:aas:1",
		Endpoints:        []model.Endpoint{endpoint},
		SpecificAssetIds: []types.ISpecificAssetID{types.NewSpecificAssetID("serialNumber", "123")},
		SubmodelDescriptors: []model.SubmodelDescriptor{{
			Id:        "urn:example:submodel:1",
			Endpoints: []model.Endpoint{endpoint},
		}},
	}

	batch, err := BuildAdministrationShellDescriptorCreateBatch(context.Background(), descriptor)
	if err != nil {
		t.Fatalf("BuildAdministrationShellDescriptorCreateBatch returned error: %v", err)
	}

	var collectedSQL strings.Builder
	for _, statement := range batch.Statements() {
		if len(statement.Args) != 0 {
			t.Fatalf("expected fully rendered Goqu statement, got args %#v for %s", statement.Args, statement.SQL)
		}
		if _, err = collectedSQL.WriteString(statement.SQL); err != nil {
			t.Fatalf("failed to collect statement SQL: %v", err)
		}
		if err = collectedSQL.WriteByte('\n'); err != nil {
			t.Fatalf("failed to collect statement separator: %v", err)
		}
	}

	for _, table := range []string{
		`"descriptor"`,
		`"descriptor_payload"`,
		`"aas_descriptor"`,
		`"aas_descriptor_endpoint"`,
		`"specific_asset_id"`,
		`"specific_asset_id_payload"`,
		`"submodel_descriptor"`,
	} {
		if !strings.Contains(collectedSQL.String(), table) {
			t.Fatalf("expected collected SQL to contain table %s", table)
		}
	}
}
