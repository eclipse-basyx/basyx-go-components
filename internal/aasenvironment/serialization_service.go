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
// Author: Martin Stemmer ( Fraunhofer IESE )

package aasenvironment

import (
	"bytes"
	"context"
	"encoding/json"
	"encoding/xml"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"path/filepath"
	"reflect"
	"strconv"
	"strings"

	"github.com/aas-core-works/aas-core3.1-golang/jsonization"
	"github.com/aas-core-works/aas-core3.1-golang/types"
	"github.com/aas-core-works/aas-core3.1-golang/xmlization"
	aasx "github.com/aas-core-works/aas-package3-golang"
	"github.com/eclipse-basyx/basyx-go-components/internal/common"
)

const serializationOperation = "GenerateSerializationByIds"

// HandleSerialization serializes the combined environment as JSON, XML or AASX.
func (s *SerializationUploadService) HandleSerialization(w http.ResponseWriter, r *http.Request) {
	if s == nil || s.aasRepository == nil || s.submodelRepository == nil || s.conceptDescriptionRepository == nil {
		s.writeErrorResponse(
			w,
			http.StatusInternalServerError,
			serializationOperation,
			"NilService",
			common.NewInternalServerError("AASENV-SERIALIZATION-NILSERVICE serialization service must not be nil"),
		)
		return
	}

	serializationMediaType, mediaErr := resolveSerializationMediaType(r)
	if mediaErr != nil {
		s.writeErrorResponse(w, http.StatusBadRequest, serializationOperation, "BadSerializationMediaType", mediaErr)
		return
	}

	aasIDs, decodeAASIDsErr := decodeIdentifierQueryValues(r.URL.Query()["aasIds"], "AASIDS")
	if decodeAASIDsErr != nil {
		s.writeErrorResponse(w, http.StatusBadRequest, serializationOperation, "BadAasIdentifiers", decodeAASIDsErr)
		return
	}

	submodelIDs, decodeSubmodelIDsErr := decodeIdentifierQueryValues(r.URL.Query()["submodelIds"], "SUBMODELIDS")
	if decodeSubmodelIDsErr != nil {
		s.writeErrorResponse(w, http.StatusBadRequest, serializationOperation, "BadSubmodelIdentifiers", decodeSubmodelIDsErr)
		return
	}

	includeConceptDescriptions, includeCDErr := parseIncludeConceptDescriptions(r.URL.Query().Get("includeConceptDescriptions"))
	if includeCDErr != nil {
		s.writeErrorResponse(w, http.StatusBadRequest, serializationOperation, "BadIncludeConceptDescriptions", includeCDErr)
		return
	}

	env, buildErr := s.buildEnvironmentForSerialization(r.Context(), aasIDs, submodelIDs, includeConceptDescriptions)
	if buildErr != nil {
		s.writeErrorResponse(
			w,
			statusFromError(buildErr),
			serializationOperation,
			"BuildEnvironment",
			buildErr,
		)
		return
	}

	payload, payloadErr := serializeEnvironment(env, serializationMediaType)
	if payloadErr != nil {
		s.writeErrorResponse(
			w,
			statusFromError(payloadErr),
			serializationOperation,
			"SerializeEnvironment",
			payloadErr,
		)
		return
	}

	writeBinaryResponse(w, http.StatusOK, serializationMediaType, payload, "")
}

func (s *SerializationUploadService) buildEnvironmentForSerialization(ctx context.Context, aasIDs []string, submodelIDs []string, includeConceptDescriptions bool) (types.IEnvironment, error) {
	aasPayloads, aasModels, resolveAASErr := s.resolveAASForSerialization(ctx, aasIDs)
	if resolveAASErr != nil {
		return nil, resolveAASErr
	}

	submodelModels, resolveSubmodelsErr := s.resolveSubmodelsForSerialization(ctx, submodelIDs, aasIDs, aasPayloads)
	if resolveSubmodelsErr != nil {
		return nil, resolveSubmodelsErr
	}

	conceptDescriptionModels, resolveCDErr := s.resolveConceptDescriptionsForSerialization(ctx, includeConceptDescriptions)
	if resolveCDErr != nil {
		return nil, resolveCDErr
	}

	environment := types.NewEnvironment()
	environment.SetAssetAdministrationShells(aasModels)
	environment.SetSubmodels(submodelModels)
	environment.SetConceptDescriptions(conceptDescriptionModels)

	return environment, nil
}

func (s *SerializationUploadService) resolveAASForSerialization(
	ctx context.Context,
	aasIDs []string,
) ([]map[string]any, []types.IAssetAdministrationShell, error) {
	var rawAASPayloads []map[string]any
	if len(aasIDs) == 0 {
		var listErr error
		rawAASPayloads, _, listErr = s.aasRepository.GetAssetAdministrationShellsForEnvironment(ctx, 0, "", "", nil)
		if listErr != nil {
			return nil, nil, fmt.Errorf("AASENV-SERIALIZATION-AAS-LIST %w", listErr)
		}
	} else {
		rawAASPayloads = make([]map[string]any, 0, len(aasIDs))
		for _, aasID := range deduplicateStrings(aasIDs) {
			aasPayload, getErr := s.aasRepository.GetAssetAdministrationShellByIDForEnvironment(ctx, aasID)
			if getErr != nil {
				return nil, nil, fmt.Errorf("AASENV-SERIALIZATION-AAS-GETBYID %w", getErr)
			}
			rawAASPayloads = append(rawAASPayloads, aasPayload)
		}
	}

	normalizedAASPayloads := make([]map[string]any, 0, len(rawAASPayloads))
	aasModels := make([]types.IAssetAdministrationShell, 0, len(rawAASPayloads))

	for _, aasPayload := range rawAASPayloads {
		normalizedAASPayload, normalizeErr := normalizeJSONMap(aasPayload)
		if normalizeErr != nil {
			return nil, nil, common.NewInternalServerError("AASENV-SERIALIZATION-AAS-NORMALIZE " + normalizeErr.Error())
		}

		aasModel, convertErr := jsonization.AssetAdministrationShellFromJsonable(normalizedAASPayload)
		if convertErr != nil {
			return nil, nil, common.NewInternalServerError("AASENV-SERIALIZATION-AAS-TOCLASS " + convertErr.Error())
		}
		normalizedAASPayloads = append(normalizedAASPayloads, normalizedAASPayload)
		aasModels = append(aasModels, aasModel)
	}

	return normalizedAASPayloads, aasModels, nil
}

func (s *SerializationUploadService) resolveSubmodelsForSerialization(
	ctx context.Context,
	explicitSubmodelIDs []string,
	aasIDs []string,
	aasPayloads []map[string]any,
) ([]types.ISubmodel, error) {
	if len(explicitSubmodelIDs) > 0 {
		return s.loadSubmodelsByIDs(ctx, explicitSubmodelIDs)
	}

	if len(aasIDs) > 0 {
		derivedSubmodelIDs := deriveReferencedSubmodelIDs(aasPayloads)
		if len(derivedSubmodelIDs) == 0 {
			return []types.ISubmodel{}, nil
		}
		return s.loadSubmodelsByIDs(ctx, derivedSubmodelIDs)
	}

	submodels, _, err := s.submodelRepository.GetSubmodelsForEnvironment(ctx, 0, "", "")
	if err != nil {
		return nil, fmt.Errorf("AASENV-SERIALIZATION-SM-LIST %w", err)
	}

	submodelIDs := make([]string, 0, len(submodels))
	for _, submodel := range submodels {
		if submodel == nil {
			continue
		}
		submodelID := strings.TrimSpace(submodel.ID())
		if submodelID == "" {
			continue
		}
		submodelIDs = append(submodelIDs, submodelID)
	}

	if len(submodelIDs) == 0 {
		return []types.ISubmodel{}, nil
	}

	return s.loadSubmodelsByIDs(ctx, submodelIDs)
}

func (s *SerializationUploadService) loadSubmodelsByIDs(ctx context.Context, submodelIDs []string) ([]types.ISubmodel, error) {
	result := make([]types.ISubmodel, 0, len(submodelIDs))
	for _, submodelID := range deduplicateStrings(submodelIDs) {
		submodel, err := s.submodelRepository.GetSubmodelByIDForEnvironment(ctx, submodelID, "", false)
		if err != nil {
			return nil, fmt.Errorf("AASENV-SERIALIZATION-SM-GETBYID %w", err)
		}
		result = append(result, submodel)
	}
	return result, nil
}

func (s *SerializationUploadService) resolveConceptDescriptionsForSerialization(
	ctx context.Context,
	includeConceptDescriptions bool,
) ([]types.IConceptDescription, error) {
	if !includeConceptDescriptions {
		return []types.IConceptDescription{}, nil
	}

	const pageSize = uint(500)

	allConceptDescriptions := make([]types.IConceptDescription, 0, pageSize)
	cursor := ""
	seenCursors := map[string]struct{}{}

	for {
		var cursorPtr *string
		if cursor != "" {
			cursorCopy := cursor
			cursorPtr = &cursorCopy
		}

		conceptDescriptions, nextCursor, err := s.conceptDescriptionRepository.GetConceptDescriptionsForEnvironment(
			ctx,
			nil,
			nil,
			nil,
			pageSize,
			cursorPtr,
		)
		if err != nil {
			return nil, fmt.Errorf("AASENV-SERIALIZATION-CD-LIST %w", err)
		}

		allConceptDescriptions = append(allConceptDescriptions, conceptDescriptions...)

		if nextCursor == "" {
			break
		}
		if _, seen := seenCursors[nextCursor]; seen {
			return nil, common.NewInternalServerError("AASENV-SERIALIZATION-CD-CURSORLOOP concept description cursor loop detected")
		}
		seenCursors[nextCursor] = struct{}{}
		cursor = nextCursor
	}

	return allConceptDescriptions, nil
}

func serializeEnvironment(environment types.IEnvironment, targetMediaType string) ([]byte, error) {
	switch targetMediaType {
	case mediaTypeJSON:
		return environmentToJSONBytes(environment)
	case mediaTypeXML:
		return environmentToXMLBytes(environment)
	case mediaTypeAASXJSON, mediaTypeAASJSONAlias:
		return environmentToAASXBytes(environment, serializationKindJSON)
	case mediaTypeAASXXML, mediaTypeAASXMLAlias, mediaTypeAASXLegacyXMLBundle:
		return environmentToAASXBytes(environment, serializationKindXML)
	default:
		return nil, common.NewErrBadRequest("AASENV-SERIALIZATION-UNSUPPORTEDMEDIATYPE unsupported serialization media type: " + targetMediaType)
	}
}

func environmentToJSONBytes(environment types.IEnvironment) ([]byte, error) {
	jsonableEnvironment, err := jsonization.ToJsonable(environment)
	if err != nil {
		return nil, common.NewInternalServerError("AASENV-SERIALIZATION-JSON-TOJSONABLE " + err.Error())
	}

	payload, err := json.Marshal(jsonableEnvironment)
	if err != nil {
		return nil, common.NewInternalServerError("AASENV-SERIALIZATION-JSON-MARSHAL " + err.Error())
	}

	return payload, nil
}

func environmentToXMLBytes(environment types.IEnvironment) ([]byte, error) {
	buffer := &bytes.Buffer{}
	if _, err := buffer.WriteString(xml.Header); err != nil {
		return nil, common.NewInternalServerError("AASENV-SERIALIZATION-XML-WRITEHEADER " + err.Error())
	}

	encoder := xml.NewEncoder(buffer)
	if err := xmlization.Marshal(encoder, environment, true); err != nil {
		return nil, common.NewInternalServerError("AASENV-SERIALIZATION-XML-MARSHAL " + err.Error())
	}
	if err := encoder.Flush(); err != nil {
		return nil, common.NewInternalServerError("AASENV-SERIALIZATION-XML-FLUSH " + err.Error())
	}

	return buffer.Bytes(), nil
}

func environmentToAASXBytes(environment types.IEnvironment, kind serializationKind) ([]byte, error) {
	specPayload, specContentType, specFileName, specErr := buildAASXSpecPayload(environment, kind)
	if specErr != nil {
		return nil, specErr
	}

	stream := &memoryReadWriteSeeker{}
	packaging := aasx.NewPackaging()
	pkg, createErr := packaging.CreateInStream(stream)
	if createErr != nil {
		return nil, common.NewInternalServerError("AASENV-SERIALIZATION-AASX-CREATEINSTREAM " + createErr.Error())
	}
	defer func() {
		_ = pkg.Close()
	}()

	specURI, parseURLErr := url.Parse("/aasx/" + specFileName)
	if parseURLErr != nil {
		return nil, common.NewInternalServerError("AASENV-SERIALIZATION-AASX-PARSEURI " + parseURLErr.Error())
	}

	specPart, putErr := pkg.PutPart(specURI, specContentType, specPayload)
	if putErr != nil {
		return nil, common.NewInternalServerError("AASENV-SERIALIZATION-AASX-PUTPART " + putErr.Error())
	}

	if makeSpecErr := pkg.MakeSpec(specPart); makeSpecErr != nil {
		return nil, common.NewInternalServerError("AASENV-SERIALIZATION-AASX-MAKESPEC " + makeSpecErr.Error())
	}

	if flushErr := pkg.Flush(); flushErr != nil {
		return nil, common.NewInternalServerError("AASENV-SERIALIZATION-AASX-FLUSH " + flushErr.Error())
	}

	if _, seekErr := stream.Seek(0, io.SeekStart); seekErr != nil {
		return nil, common.NewInternalServerError("AASENV-SERIALIZATION-AASX-SEEKSTREAM " + seekErr.Error())
	}

	result, readErr := io.ReadAll(stream)
	if readErr != nil {
		return nil, common.NewInternalServerError("AASENV-SERIALIZATION-AASX-READSTREAM " + readErr.Error())
	}

	return result, nil
}

func buildAASXSpecPayload(environment types.IEnvironment, kind serializationKind) ([]byte, string, string, error) {
	switch kind {
	case serializationKindJSON:
		payload, err := environmentToJSONBytes(environment)
		if err != nil {
			return nil, "", "", err
		}
		return payload, mediaTypeJSON, "environment.aas.json", nil
	case serializationKindXML:
		payload, err := environmentToXMLBytes(environment)
		if err != nil {
			return nil, "", "", err
		}
		return payload, mediaTypeXML, "environment.aas.xml", nil
	default:
		return nil, "", "", common.NewErrBadRequest("AASENV-SERIALIZATION-AASX-BADKIND unsupported AASX spec kind")
	}
}

func decodeIdentifierQueryValues(rawValues []string, errorSuffix string) ([]string, error) {
	values := splitCSVQueryValues(rawValues)
	if len(values) == 0 {
		return nil, nil
	}

	decodedValues := make([]string, 0, len(values))
	for _, value := range values {
		decoded, err := common.DecodeString(value)
		if err != nil {
			return nil, common.NewErrBadRequest("AASENV-SERIALIZATION-DECODE-" + errorSuffix + " " + err.Error())
		}
		decodedValues = append(decodedValues, decoded)
	}

	return deduplicateStrings(decodedValues), nil
}

func parseIncludeConceptDescriptions(raw string) (bool, error) {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return true, nil
	}

	includeConceptDescriptions, err := strconv.ParseBool(trimmed)
	if err != nil {
		return false, common.NewErrBadRequest("AASENV-SERIALIZATION-INCLUDECDBOOL " + err.Error())
	}

	return includeConceptDescriptions, nil
}

func resolveSerializationMediaType(r *http.Request) (string, error) {
	acceptHeader := strings.TrimSpace(r.Header.Get("Accept"))
	if acceptHeader == "" || acceptHeader == "*/*" {
		return mediaTypeJSON, nil
	}

	for _, rawToken := range strings.Split(acceptHeader, ",") {
		normalized := normalizeMediaType(rawToken)
		if normalized == "" || normalized == "*/*" {
			continue
		}
		if isSupportedSerializationMediaType(normalized) {
			return normalized, nil
		}
	}

	if strings.Contains(acceptHeader, "*/*") {
		return mediaTypeJSON, nil
	}

	return "", common.NewErrBadRequest("AASENV-SERIALIZATION-UNSUPPORTEDACCEPT unsupported Accept media type: " + acceptHeader)
}

func isSupportedSerializationMediaType(mediaType string) bool {
	switch mediaType {
	case mediaTypeJSON,
		mediaTypeXML,
		mediaTypeAASXJSON,
		mediaTypeAASXXML,
		mediaTypeAASJSONAlias,
		mediaTypeAASXMLAlias,
		mediaTypeAASXLegacyXMLBundle:
		return true
	default:
		return false
	}
}

func splitCSVQueryValues(rawValues []string) []string {
	result := make([]string, 0, len(rawValues))
	for _, rawValue := range rawValues {
		for _, splitValue := range strings.Split(rawValue, ",") {
			trimmed := strings.TrimSpace(splitValue)
			if trimmed == "" {
				continue
			}
			result = append(result, trimmed)
		}
	}
	return result
}

func deduplicateStrings(values []string) []string {
	result := make([]string, 0, len(values))
	seen := map[string]struct{}{}

	for _, value := range values {
		if _, exists := seen[value]; exists {
			continue
		}
		seen[value] = struct{}{}
		result = append(result, value)
	}

	return result
}

func deriveReferencedSubmodelIDs(aasPayloads []map[string]any) []string {
	submodelIDs := make([]string, 0)
	seenSubmodelIDs := map[string]struct{}{}

	for _, aasPayload := range aasPayloads {
		rawReferences, exists := aasPayload["submodels"]
		if !exists {
			continue
		}

		references := toAnySlice(rawReferences)
		if len(references) == 0 {
			continue
		}

		for _, rawReference := range references {
			reference, ok := rawReference.(map[string]any)
			if !ok {
				continue
			}

			rawKeys, keyExists := reference["keys"]
			if !keyExists {
				continue
			}

			keys := toAnySlice(rawKeys)
			if len(keys) == 0 {
				continue
			}

			for _, rawKey := range keys {
				key, ok := rawKey.(map[string]any)
				if !ok {
					continue
				}

				value := anyToString(key["value"])
				if strings.TrimSpace(value) == "" {
					continue
				}

				keyType := strings.TrimSpace(anyToString(key["type"]))
				if keyType != "" && !strings.EqualFold(keyType, "Submodel") {
					continue
				}

				if _, alreadySeen := seenSubmodelIDs[value]; alreadySeen {
					break
				}

				seenSubmodelIDs[value] = struct{}{}
				submodelIDs = append(submodelIDs, value)
				break
			}
		}
	}

	return submodelIDs
}

func anyToString(value any) string {
	switch typedValue := value.(type) {
	case nil:
		return ""
	case string:
		return typedValue
	case fmt.Stringer:
		return typedValue.String()
	default:
		return fmt.Sprintf("%v", value)
	}
}

func toAnySlice(value any) []any {
	switch typedValue := value.(type) {
	case []any:
		return typedValue
	case []map[string]any:
		result := make([]any, 0, len(typedValue))
		for _, entry := range typedValue {
			result = append(result, entry)
		}
		return result
	}

	reflectValue := reflect.ValueOf(value)
	if !reflectValue.IsValid() || reflectValue.Kind() != reflect.Slice {
		return nil
	}

	result := make([]any, 0, reflectValue.Len())
	for index := 0; index < reflectValue.Len(); index++ {
		result = append(result, reflectValue.Index(index).Interface())
	}

	return result
}

func normalizeJSONMap(raw map[string]any) (map[string]any, error) {
	normalizedValue, err := normalizeJSONValue(raw)
	if err != nil {
		return nil, err
	}

	normalizedMap, ok := normalizedValue.(map[string]any)
	if !ok {
		return nil, fmt.Errorf("AASENV-NORMALIZE-JSON-NOTMAP normalized value is not an object")
	}

	return normalizedMap, nil
}

func normalizeJSONValue(raw any) (any, error) {
	payload, marshalErr := json.Marshal(raw)
	if marshalErr != nil {
		return nil, marshalErr
	}

	var normalized any
	if unmarshalErr := json.Unmarshal(payload, &normalized); unmarshalErr != nil {
		return nil, unmarshalErr
	}

	return normalized, nil
}

func writeBinaryResponse(w http.ResponseWriter, statusCode int, contentType string, payload []byte, fileName string) {
	w.Header().Set("Content-Type", contentType)
	w.Header().Set("X-Content-Type-Options", "nosniff")
	if fileName != "" {
		safeName := filepath.Base(fileName)
		w.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=%q", safeName))
	}
	w.WriteHeader(statusCode)
	_, _ = io.Copy(w, bytes.NewReader(payload))
}

type memoryReadWriteSeeker struct {
	buffer []byte
	offset int64
}

func (m *memoryReadWriteSeeker) Read(p []byte) (int, error) {
	if m.offset >= int64(len(m.buffer)) {
		return 0, io.EOF
	}

	n := copy(p, m.buffer[m.offset:])
	m.offset += int64(n)
	return n, nil
}

func (m *memoryReadWriteSeeker) Write(p []byte) (int, error) {
	if m.offset < 0 {
		return 0, common.NewInternalServerError("AASENV-SERIALIZATION-AASX-NEGATIVEOFFSET negative stream offset")
	}

	end := m.offset + int64(len(p))
	if end > int64(len(m.buffer)) {
		if end <= int64(cap(m.buffer)) {
			m.buffer = m.buffer[:end]
		} else {
			m.buffer = append(m.buffer, make([]byte, end-int64(len(m.buffer)))...)
		}
	}

	copy(m.buffer[m.offset:end], p)
	m.offset = end
	return len(p), nil
}

func (m *memoryReadWriteSeeker) Seek(offset int64, whence int) (int64, error) {
	var base int64
	switch whence {
	case io.SeekStart:
		base = 0
	case io.SeekCurrent:
		base = m.offset
	case io.SeekEnd:
		base = int64(len(m.buffer))
	default:
		return 0, common.NewInternalServerError("AASENV-SERIALIZATION-AASX-BADSEEKWHENCE invalid seek whence")
	}

	next := base + offset
	if next < 0 {
		return 0, common.NewInternalServerError("AASENV-SERIALIZATION-AASX-NEGATIVESEEK negative seek position")
	}

	m.offset = next
	return m.offset, nil
}
