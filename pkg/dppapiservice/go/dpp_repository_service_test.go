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
	"strings"
	"testing"

	"github.com/FriedJannik/aas-go-sdk/types"
	"github.com/eclipse-basyx/basyx-go-components/internal/aasenvironment"
	aasregistrydb "github.com/eclipse-basyx/basyx-go-components/internal/aasregistry/persistence"
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
			service, err := NewDPPRepositoryServiceWithRegistrySync(nil, nil, test.aasRegistry, test.smRegistry, aasenvironment.RegistrySyncConfig{
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
