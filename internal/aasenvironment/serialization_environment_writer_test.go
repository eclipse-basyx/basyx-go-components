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

package aasenvironment

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"encoding/xml"
	"errors"
	"io"
	"strconv"
	"strings"
	"testing"

	"github.com/FriedJannik/aas-go-sdk/jsonization"
	aastypes "github.com/FriedJannik/aas-go-sdk/types"
	aasxmlization "github.com/FriedJannik/aas-go-sdk/xmlization"
	"github.com/eclipse-basyx/basyx-go-components/internal/common"
	"github.com/stretchr/testify/require"
)

func TestEnvironmentStreamWritersMatchSDKEnvironmentSerialization(t *testing.T) {
	environment := newStreamWriterTestEnvironment()
	expected := environmentJSON(t, environment)

	for _, contentType := range []string{serializationContentTypeJSON, serializationContentTypeXML} {
		t.Run(contentType, func(t *testing.T) {
			payload := streamTestEnvironment(t, contentType, environment)
			require.JSONEq(t, expected, environmentJSON(t, parseStreamedEnvironment(t, contentType, payload)))
		})
	}
}

func TestEnvironmentStreamWritersOmitEmptySections(t *testing.T) {
	environment := aastypes.NewEnvironment()
	environment.SetSubmodels([]aastypes.ISubmodel{aastypes.NewSubmodel("urn:sm:only")})

	jsonPayload := streamTestEnvironment(t, serializationContentTypeJSON, environment)
	require.JSONEq(t, `{"submodels":[{"modelType":"Submodel","id":"urn:sm:only"}]}`, string(jsonPayload))

	xmlPayload := streamTestEnvironment(t, serializationContentTypeXML, environment)
	require.NotContains(t, string(xmlPayload), environmentSectionAssetAdministrationShells)
	require.NotContains(t, string(xmlPayload), environmentSectionConceptDescriptions)
	require.Len(t, parseStreamedEnvironment(t, serializationContentTypeXML, xmlPayload).Submodels(), 1)
}

func TestEnvironmentStreamWritersSerializeEmptyEnvironment(t *testing.T) {
	require.Equal(t, "{}", string(streamTestEnvironment(t, serializationContentTypeJSON, aastypes.NewEnvironment())))

	xmlPayload := streamTestEnvironment(t, serializationContentTypeXML, aastypes.NewEnvironment())
	environment := parseStreamedEnvironment(t, serializationContentTypeXML, xmlPayload)
	require.Empty(t, environment.AssetAdministrationShells())
	require.Empty(t, environment.Submodels())
}

func TestEnvironmentStreamWriterKeepsClosedPipeRecognizable(t *testing.T) {
	reader, writer := io.Pipe()
	require.NoError(t, reader.Close())
	streamWriter, err := newEnvironmentStreamWriter(serializationContentTypeXML, writer)
	require.NoError(t, err)

	err = streamWriter.close()

	require.ErrorIs(t, err, io.ErrClosedPipe)
}

func TestConceptDescriptionReferencesCollectSemanticIDsOfSubmodel(t *testing.T) {
	submodel := aastypes.NewSubmodel("urn:sm:semantics")
	submodel.SetSemanticID(externalReference("urn:cd:submodel"))
	submodel.SetSupplementalSemanticIDs([]aastypes.IReference{externalReference("urn:cd:supplemental")})
	qualifier := aastypes.NewQualifier("kind", aastypes.DataTypeDefXSDString)
	qualifier.SetSemanticID(externalReference("urn:cd:qualifier"))
	property := aastypes.NewProperty(aastypes.DataTypeDefXSDString)
	property.SetIDShort(stringPointer("property"))
	property.SetSemanticID(aastypes.NewReference(aastypes.ReferenceTypesModelReference, []aastypes.IKey{
		aastypes.NewKey(aastypes.KeyTypesConceptDescription, "urn:cd:property"),
	}))
	property.SetQualifiers([]aastypes.IQualifier{qualifier})
	duplicate := aastypes.NewProperty(aastypes.DataTypeDefXSDString)
	duplicate.SetIDShort(stringPointer("duplicate"))
	duplicate.SetSemanticID(externalReference("urn:cd:submodel"))
	nonConceptDescription := aastypes.NewProperty(aastypes.DataTypeDefXSDString)
	nonConceptDescription.SetIDShort(stringPointer("nonConceptDescription"))
	nonConceptDescription.SetSemanticID(aastypes.NewReference(aastypes.ReferenceTypesModelReference, []aastypes.IKey{
		aastypes.NewKey(aastypes.KeyTypesSubmodel, "urn:sm:other"),
	}))
	collection := aastypes.NewSubmodelElementCollection()
	collection.SetIDShort(stringPointer("collection"))
	collection.SetValue([]aastypes.ISubmodelElement{property, duplicate, nonConceptDescription})
	submodel.SetSubmodelElements([]aastypes.ISubmodelElement{collection})

	references := newConceptDescriptionReferences()
	references.collect(submodel)

	require.Equal(t, []string{"urn:cd:submodel", "urn:cd:supplemental", "urn:cd:property", "urn:cd:qualifier"}, references.identifiers())
}

func TestConceptDescriptionReferencesNilCollectorIgnoresSubmodels(t *testing.T) {
	var references *conceptDescriptionReferences
	references.collect(aastypes.NewSubmodel("urn:sm:ignored"))

	require.Empty(t, references.identifiers())
}

func TestNewSerializationRequestRequiresExplicitIdentifiers(t *testing.T) {
	_, err := newSerializationRequest(nil, nil, true)

	require.True(t, common.IsErrBadRequest(err), "unexpected error: %v", err)
}

func TestNewSerializationRequestDecodesAndDeduplicatesIdentifiers(t *testing.T) {
	encodedAAS := base64.RawURLEncoding.EncodeToString([]byte("urn:aas:1"))
	encodedSubmodel := base64.RawURLEncoding.EncodeToString([]byte("urn:sm:1"))

	request, err := newSerializationRequest([]string{encodedAAS, encodedAAS}, []string{encodedSubmodel}, false)

	require.NoError(t, err)
	require.Equal(t, []string{"urn:aas:1"}, request.aasIDs)
	require.Equal(t, []string{"urn:sm:1"}, request.submodelIDs)
	require.False(t, request.includeConceptDescriptions)
}

func TestNewSerializationRequestRejectsInvalidIdentifiers(t *testing.T) {
	_, err := newSerializationRequest(nil, []string{"%%%"}, false)

	require.True(t, common.IsErrBadRequest(err), "unexpected error: %v", err)
}

func TestResolveEnvironmentStagingErrorPrecedence(t *testing.T) {
	notFound := common.NewErrNotFound("AASENV-TEST-NOTFOUND missing")
	tooLarge := common.NewErrPayloadTooLarge("AASENV-TEST-TOOLARGE too large")
	stageFailure := errors.New("stage failed")

	require.Equal(t, notFound, resolveEnvironmentStagingError(stageFailure, notFound))
	require.Equal(t, tooLarge, resolveEnvironmentStagingError(tooLarge, notFound))
	require.Equal(t, stageFailure, resolveEnvironmentStagingError(stageFailure, io.ErrClosedPipe))
	require.NoError(t, resolveEnvironmentStagingError(nil, nil))
}

func TestResolveSerializationFormatRejectsUnsupportedContentType(t *testing.T) {
	_, err := resolveSerializationFormat("application/pdf")

	require.True(t, common.IsErrBadRequest(err), "unexpected error: %v", err)
}

func newStreamWriterTestEnvironment() aastypes.IEnvironment {
	firstAAS := aastypes.NewAssetAdministrationShell("urn:aas:1", aastypes.NewAssetInformation(aastypes.AssetKindInstance))
	secondAAS := aastypes.NewAssetAdministrationShell("urn:aas:2", aastypes.NewAssetInformation(aastypes.AssetKindType))
	property := aastypes.NewProperty(aastypes.DataTypeDefXSDString)
	property.SetIDShort(stringPointer("property"))
	property.SetValue(stringPointer("<value> & \"quoted\""))
	property.SetSemanticID(externalReference("urn:cd:1"))
	firstSubmodel := aastypes.NewSubmodel("urn:sm:1")
	firstSubmodel.SetSubmodelElements([]aastypes.ISubmodelElement{property})
	conceptDescription := aastypes.NewConceptDescription("urn:cd:1")

	environment := aastypes.NewEnvironment()
	environment.SetAssetAdministrationShells([]aastypes.IAssetAdministrationShell{firstAAS, secondAAS})
	environment.SetSubmodels([]aastypes.ISubmodel{firstSubmodel, aastypes.NewSubmodel("urn:sm:2")})
	environment.SetConceptDescriptions([]aastypes.IConceptDescription{conceptDescription})
	return environment
}

func streamTestEnvironment(t *testing.T, contentType string, environment aastypes.IEnvironment) []byte {
	t.Helper()
	payload := bytes.NewBuffer(nil)
	writer, err := newEnvironmentStreamWriter(contentType, payload)
	require.NoError(t, err)
	for _, aas := range environment.AssetAdministrationShells() {
		require.NoError(t, writer.writeIdentifiable(environmentSectionAssetAdministrationShells, aas))
	}
	for _, submodel := range environment.Submodels() {
		require.NoError(t, writer.writeIdentifiable(environmentSectionSubmodels, submodel))
	}
	for _, conceptDescription := range environment.ConceptDescriptions() {
		require.NoError(t, writer.writeIdentifiable(environmentSectionConceptDescriptions, conceptDescription))
	}
	require.NoError(t, writer.close())
	return payload.Bytes()
}

func parseStreamedEnvironment(t *testing.T, contentType string, payload []byte) aastypes.IEnvironment {
	t.Helper()
	if contentType == serializationContentTypeXML {
		instance, err := aasxmlization.Unmarshal(xml.NewDecoder(bytes.NewReader(payload)))
		require.NoError(t, err)
		environment, ok := instance.(aastypes.IEnvironment)
		require.True(t, ok)
		return environment
	}
	var jsonable any
	require.NoError(t, json.Unmarshal(payload, &jsonable))
	environment, err := jsonization.EnvironmentFromJsonable(jsonable)
	require.NoError(t, err)
	return environment
}

func environmentJSON(t *testing.T, environment aastypes.IEnvironment) string {
	t.Helper()
	jsonable, err := jsonization.ToJsonable(environment)
	require.NoError(t, err)
	payload, err := json.Marshal(jsonable)
	require.NoError(t, err)
	return string(payload)
}

func externalReference(value string) aastypes.IReference {
	return aastypes.NewReference(aastypes.ReferenceTypesExternalReference, []aastypes.IKey{
		aastypes.NewKey(aastypes.KeyTypesGlobalReference, value),
	})
}

func stringPointer(value string) *string {
	return &value
}

type writeCounter struct {
	writes int
}

func (counter *writeCounter) Write(payload []byte) (int, error) {
	counter.writes++
	return len(payload), nil
}

func TestEnvironmentStreamWritersWriteInChunks(t *testing.T) {
	submodel := aastypes.NewSubmodel("urn:sm:chunked")
	elements := make([]aastypes.ISubmodelElement, 0, 200)
	for index := 0; index < 200; index++ {
		property := aastypes.NewProperty(aastypes.DataTypeDefXSDString)
		property.SetIDShort(stringPointer("property" + strconv.Itoa(index)))
		property.SetValue(stringPointer(strings.Repeat("x", 64)))
		elements = append(elements, property)
	}
	submodel.SetSubmodelElements(elements)

	for _, contentType := range []string{serializationContentTypeJSON, serializationContentTypeXML} {
		t.Run(contentType, func(t *testing.T) {
			counter := &writeCounter{}
			writer, err := newEnvironmentStreamWriter(contentType, counter)
			require.NoError(t, err)
			for index := 0; index < 10; index++ {
				require.NoError(t, writer.writeIdentifiable(environmentSectionSubmodels, submodel))
			}
			require.NoError(t, writer.close())

			require.LessOrEqual(t, counter.writes, 20)
		})
	}
}
