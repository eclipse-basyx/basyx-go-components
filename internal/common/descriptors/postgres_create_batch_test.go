/*******************************************************************************
* Copyright (C) 2026 the Eclipse BaSyx Authors and Fraunhofer IESE
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
		collectedSQL.WriteString(statement.SQL)
		collectedSQL.WriteByte('\n')
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
