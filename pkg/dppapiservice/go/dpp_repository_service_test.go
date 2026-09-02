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
	"context"
	"database/sql"
	"fmt"
	"strings"
	"testing"

	"github.com/FriedJannik/aas-go-sdk/types"
	aasregistrydb "github.com/eclipse-basyx/basyx-go-components/internal/aasregistry/persistence"
	"github.com/eclipse-basyx/basyx-go-components/internal/common"
	commonmodel "github.com/eclipse-basyx/basyx-go-components/internal/common/model"
	"github.com/eclipse-basyx/basyx-go-components/internal/registrysync"
	smregistrydb "github.com/eclipse-basyx/basyx-go-components/internal/smregistry/persistence"
)

func TestNewDPPRepositoryServiceWithRegistrySyncFlagCombinations(t *testing.T) {
	tests := []struct {
		name        string
		aasEnabled  bool
		smEnabled   bool
		aasRegistry *aasregistrydb.PostgreSQLAASRegistryDatabase
		smRegistry  *smregistrydb.PostgreSQLSMDatabase
		wantCode    string
	}{
		{name: "disabled"},
		{name: "AAS only", aasEnabled: true, aasRegistry: &aasregistrydb.PostgreSQLAASRegistryDatabase{}},
		{name: "Submodel only", smEnabled: true, smRegistry: &smregistrydb.PostgreSQLSMDatabase{}},
		{name: "both", aasEnabled: true, smEnabled: true, aasRegistry: &aasregistrydb.PostgreSQLAASRegistryDatabase{}, smRegistry: &smregistrydb.PostgreSQLSMDatabase{}},
		{name: "missing AAS dependency", aasEnabled: true, wantCode: "DPP-REGSYNC-NILAASREGISTRY"},
		{name: "missing Submodel dependency", smEnabled: true, wantCode: "DPP-REGSYNC-NILSMREGISTRY"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			service, err := NewDPPRepositoryServiceWithRegistrySync(nil, nil, test.aasRegistry, test.smRegistry, registrysync.Config{
				AASRegistryIntegration:      test.aasEnabled,
				SubmodelRegistryIntegration: test.smEnabled,
			})
			if test.wantCode == "" {
				if err != nil || service == nil {
					t.Fatalf("NewDPPRepositoryServiceWithRegistrySync() service = %v, error = %v", service, err)
				}
				return
			}
			if err == nil || !strings.Contains(err.Error(), test.wantCode) {
				t.Fatalf("NewDPPRepositoryServiceWithRegistrySync() error = %v, want %s", err, test.wantCode)
			}
		})
	}
}

func TestRegistrySynchronizationFlagCombinations(t *testing.T) {
	for combination := 0; combination < 8; combination++ {
		aasEnabled := combination&1 != 0
		submodelEnabled := combination&2 != 0
		discoveryEnabled := combination&4 != 0
		t.Run(fmt.Sprintf("aas=%t/submodel=%t/discovery=%t", aasEnabled, submodelEnabled, discoveryEnabled), func(t *testing.T) {
			aasRegistry := newRecordingAASRegistry()
			submodelRegistry := newRecordingSubmodelRegistry()
			service, err := newDPPRepositoryServiceWithRegistrySync(nil, nil, aasRegistry, submodelRegistry, registrysync.Config{
				AASRegistryIntegration:      aasEnabled,
				SubmodelRegistryIntegration: submodelEnabled,
				ExternalBaseURLs:            []string{"https://aas.example.test"},
			})
			if err != nil {
				t.Fatalf("newDPPRepositoryServiceWithRegistrySync() error = %v", err)
			}

			ctx := common.ContextWithConfig(t.Context(), &common.Config{
				General: common.GeneralConfig{DiscoveryIntegration: discoveryEnabled},
			})
			submodel := types.NewSubmodel("urn:example:submodel")
			aas := types.NewAssetAdministrationShell(
				"urn:example:aas",
				types.NewAssetInformation(types.AssetKindInstance),
			)
			aas.SetSubmodels([]types.IReference{submodelReference(submodel.ID())})

			if err = service.syncCreatedDescriptors(ctx, nil, aas, []types.ISubmodel{submodel}); err != nil {
				t.Fatalf("syncCreatedDescriptors() error = %v", err)
			}
			if err = service.syncUpdatedDescriptors(ctx, nil, aas, aas, []submodelDescriptorUpdate{{
				previous: submodel, submitted: submodel,
			}}, []string{"urn:example:stale"}); err != nil {
				t.Fatalf("syncUpdatedDescriptors() error = %v", err)
			}
			if err = service.deleteSubmodelDescriptorIfEnabled(ctx, nil, submodel.ID()); err != nil {
				t.Fatalf("deleteSubmodelDescriptorIfEnabled() error = %v", err)
			}
			if err = service.deleteAASDescriptorIfEnabled(ctx, nil, aas.ID()); err != nil {
				t.Fatalf("deleteAASDescriptorIfEnabled() error = %v", err)
			}

			assertRegistryCalls(t, "AAS", aasRegistry.calls, aasEnabled, 2)
			assertRegistryCalls(t, "Submodel", submodelRegistry.calls, submodelEnabled, 3)
			for _, observed := range append(aasRegistry.discoverySettings, submodelRegistry.discoverySettings...) {
				if observed != discoveryEnabled {
					t.Fatalf("registry context discoveryIntegration = %t, want %t", observed, discoveryEnabled)
				}
			}
		})
	}
}

func TestRegistrySynchronizationRepairsMissingUnchangedDescriptors(t *testing.T) {
	aasRegistry := newRecordingAASRegistry()
	submodelRegistry := newRecordingSubmodelRegistry()
	service, err := newDPPRepositoryServiceWithRegistrySync(nil, nil, aasRegistry, submodelRegistry, registrysync.Config{
		AASRegistryIntegration:      true,
		SubmodelRegistryIntegration: true,
		ExternalBaseURLs:            []string{"https://aas.example.test"},
	})
	if err != nil {
		t.Fatalf("newDPPRepositoryServiceWithRegistrySync() error = %v", err)
	}

	aas := types.NewAssetAdministrationShell("urn:example:aas", types.NewAssetInformation(types.AssetKindInstance))
	submodel := types.NewSubmodel("urn:example:submodel")
	if err = service.syncUpdatedDescriptors(t.Context(), nil, aas, aas, []submodelDescriptorUpdate{{
		previous: submodel, submitted: submodel,
	}}, nil); err != nil {
		t.Fatalf("syncUpdatedDescriptors() error = %v", err)
	}

	if aasRegistry.calls != 1 {
		t.Fatalf("AAS registry writes = %d, want repair upsert", aasRegistry.calls)
	}
	if submodelRegistry.calls != 1 {
		t.Fatalf("Submodel registry writes = %d, want repair upsert", submodelRegistry.calls)
	}
}

func TestRegistrySynchronizationSkipsExistingUnchangedDescriptors(t *testing.T) {
	aasRegistry := newRecordingAASRegistry()
	submodelRegistry := newRecordingSubmodelRegistry()
	service, err := newDPPRepositoryServiceWithRegistrySync(nil, nil, aasRegistry, submodelRegistry, registrysync.Config{
		AASRegistryIntegration:      true,
		SubmodelRegistryIntegration: true,
		ExternalBaseURLs:            []string{"https://aas.example.test"},
	})
	if err != nil {
		t.Fatalf("newDPPRepositoryServiceWithRegistrySync() error = %v", err)
	}

	aas := types.NewAssetAdministrationShell("urn:example:aas", types.NewAssetInformation(types.AssetKindInstance))
	submodel := types.NewSubmodel("urn:example:submodel")
	if err = service.syncCreatedDescriptors(t.Context(), nil, aas, []types.ISubmodel{submodel}); err != nil {
		t.Fatalf("syncCreatedDescriptors() error = %v", err)
	}
	aasWrites := aasRegistry.calls
	submodelWrites := submodelRegistry.calls

	if err = service.syncUpdatedDescriptors(t.Context(), nil, aas, aas, []submodelDescriptorUpdate{{
		previous: submodel, submitted: submodel,
	}}, nil); err != nil {
		t.Fatalf("syncUpdatedDescriptors() error = %v", err)
	}
	if aasRegistry.calls != aasWrites {
		t.Fatalf("AAS registry writes = %d, want unchanged %d", aasRegistry.calls, aasWrites)
	}
	if submodelRegistry.calls != submodelWrites {
		t.Fatalf("Submodel registry writes = %d, want unchanged %d", submodelRegistry.calls, submodelWrites)
	}
}

type recordingAASRegistry struct {
	calls             int
	discoverySettings []bool
	descriptors       map[string]commonmodel.AssetAdministrationShellDescriptor
}

func newRecordingAASRegistry() *recordingAASRegistry {
	return &recordingAASRegistry{descriptors: make(map[string]commonmodel.AssetAdministrationShellDescriptor)}
}

func (r *recordingAASRegistry) InsertAdministrationShellDescriptorInTransaction(ctx context.Context, _ *sql.Tx, descriptor commonmodel.AssetAdministrationShellDescriptor) error {
	r.record(ctx)
	r.descriptors[descriptor.Id] = descriptor
	return nil
}

func (r *recordingAASRegistry) UpsertAdministrationShellDescriptorInTransaction(ctx context.Context, _ *sql.Tx, descriptor commonmodel.AssetAdministrationShellDescriptor) error {
	r.record(ctx)
	r.descriptors[descriptor.Id] = descriptor
	return nil
}

func (r *recordingAASRegistry) GetAssetAdministrationShellDescriptorByIDInTransaction(_ context.Context, _ *sql.Tx, id string) (commonmodel.AssetAdministrationShellDescriptor, error) {
	descriptor, ok := r.descriptors[id]
	if !ok {
		return commonmodel.AssetAdministrationShellDescriptor{}, common.NewErrNotFound("DPP-TEST-AASDESC-NOTFOUND")
	}
	return descriptor, nil
}

func (r *recordingAASRegistry) DeleteAssetAdministrationShellDescriptorByIDInTransaction(ctx context.Context, _ *sql.Tx, id string) error {
	r.record(ctx)
	delete(r.descriptors, id)
	return nil
}

func (r *recordingAASRegistry) record(ctx context.Context) {
	r.calls++
	r.discoverySettings = append(r.discoverySettings, discoveryIntegrationFromContext(ctx))
}

type recordingSubmodelRegistry struct {
	calls             int
	discoverySettings []bool
	descriptors       map[string]commonmodel.SubmodelDescriptor
}

func newRecordingSubmodelRegistry() *recordingSubmodelRegistry {
	return &recordingSubmodelRegistry{descriptors: make(map[string]commonmodel.SubmodelDescriptor)}
}

func (r *recordingSubmodelRegistry) InsertSubmodelDescriptorsInTransaction(ctx context.Context, _ *sql.Tx, descriptors []commonmodel.SubmodelDescriptor) (int, error) {
	r.record(ctx)
	for _, descriptor := range descriptors {
		r.descriptors[descriptor.Id] = descriptor
	}
	return -1, nil
}

func (r *recordingSubmodelRegistry) UpsertSubmodelDescriptorInTransaction(ctx context.Context, _ *sql.Tx, descriptor commonmodel.SubmodelDescriptor) error {
	r.record(ctx)
	r.descriptors[descriptor.Id] = descriptor
	return nil
}

func (r *recordingSubmodelRegistry) GetSubmodelDescriptorByIDInTransaction(_ context.Context, _ *sql.Tx, id string) (commonmodel.SubmodelDescriptor, error) {
	descriptor, ok := r.descriptors[id]
	if !ok {
		return commonmodel.SubmodelDescriptor{}, common.NewErrNotFound("DPP-TEST-SMDESC-NOTFOUND")
	}
	return descriptor, nil
}

func (r *recordingSubmodelRegistry) DeleteSubmodelDescriptorByIDInTransaction(ctx context.Context, _ *sql.Tx, id string) error {
	r.record(ctx)
	delete(r.descriptors, id)
	return nil
}

func (r *recordingSubmodelRegistry) record(ctx context.Context) {
	r.calls++
	r.discoverySettings = append(r.discoverySettings, discoveryIntegrationFromContext(ctx))
}

func discoveryIntegrationFromContext(ctx context.Context) bool {
	cfg, ok := common.ConfigFromContext(ctx)
	return ok && cfg.General.DiscoveryIntegration
}

func assertRegistryCalls(t *testing.T, registry string, calls int, enabled bool, enabledCalls int) {
	t.Helper()
	want := 0
	if enabled {
		want = enabledCalls
	}
	if calls != want {
		t.Fatalf("%s registry calls = %d, want %d", registry, calls, want)
	}
}

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

func TestPreserveManagedFileValuesRestoresRepositoryPathOnDPPUpdate(t *testing.T) {
	managedPath := "/aasx/files/token/manual.pdf"
	currentFile := testFileElement("manual", managedPath)
	current := types.NewSubmodel("submodel/id")
	current.SetSubmodelElements([]types.ISubmodelElement{currentFile})

	attachmentURL := "https://aas.example.test/submodels/c3VibW9kZWwvaWQ/submodel-elements/manual/attachment"
	replacementFile := testFileElement("manual", attachmentURL)
	replacement := types.NewSubmodel("submodel/id")
	replacement.SetSubmodelElements([]types.ISubmodelElement{replacementFile})

	preserveManagedFileValues(current, replacement, dppSerializationContext{
		submodelID:             current.ID(),
		externalBaseURL:        "https://aas.example.test",
		managedAttachmentPaths: map[string]struct{}{"manual": {}},
	})
	if got := dereferenceString(replacementFile.Value()); got != managedPath {
		t.Fatalf("replacement File value = %q, want managed path %q", got, managedPath)
	}
}

func TestPreserveManagedFileValuesKeepsChangedExternalURL(t *testing.T) {
	current := types.NewSubmodel("submodel/id")
	current.SetSubmodelElements([]types.ISubmodelElement{testFileElement("manual", "/aasx/files/token/manual.pdf")})
	replacementFile := testFileElement("manual", "https://files.example.test/replacement.pdf")
	replacement := types.NewSubmodel("submodel/id")
	replacement.SetSubmodelElements([]types.ISubmodelElement{replacementFile})

	preserveManagedFileValues(current, replacement, dppSerializationContext{
		submodelID:             current.ID(),
		externalBaseURL:        "https://aas.example.test",
		managedAttachmentPaths: map[string]struct{}{"manual": {}},
	})
	if got := dereferenceString(replacementFile.Value()); got != "https://files.example.test/replacement.pdf" {
		t.Fatalf("replacement File value = %q, want changed external URL", got)
	}
}

func testFileElement(idShort string, value string) *types.File {
	file := types.NewFile()
	file.SetIDShort(&idShort)
	file.SetValue(&value)
	return file
}
