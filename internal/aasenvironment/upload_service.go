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
	"database/sql"
	"encoding/json"
	"encoding/xml"
	"fmt"
	"io"
	"net/http"
	"sort"
	"strings"

	"github.com/aas-core-works/aas-core3.1-golang/jsonization"
	"github.com/aas-core-works/aas-core3.1-golang/types"
	"github.com/aas-core-works/aas-core3.1-golang/xmlization"
	aasx "github.com/aas-core-works/aas-package3-golang"
	"github.com/eclipse-basyx/basyx-go-components/internal/common"
)

const (
	uploadOperation = "UploadEnvironment"

	maxUploadFileBytes        int64 = 64 << 20
	maxUploadRequestBodyBytes int64 = maxUploadFileBytes + (4 << 20)
)

type specCandidate struct {
	part  *aasx.Part
	kind  serializationKind
	uri   string
	mime  string
	score int
}

// HandleUpload uploads an Environment payload as JSON, XML or AASX (multipart file part).
func (s *SerializationUploadService) HandleUpload(w http.ResponseWriter, r *http.Request) {
	if s == nil || s.aasRepository == nil || s.submodelRepository == nil || s.conceptDescriptionRepository == nil {
		s.writeErrorResponse(
			w,
			http.StatusInternalServerError,
			uploadOperation,
			"NilService",
			common.NewInternalServerError("AASENV-UPLOAD-NILSERVICE upload service must not be nil"),
		)
		return
	}

	filePayload, fileMediaType, readErr := readUploadedMultipartFile(w, r)
	if readErr != nil {
		s.writeErrorResponse(w, http.StatusBadRequest, uploadOperation, "ReadMultipartFile", readErr)
		return
	}

	env, parseErr := parseEnvironmentPayload(fileMediaType, filePayload)
	if parseErr != nil {
		s.writeErrorResponse(w, http.StatusBadRequest, uploadOperation, "ParseEnvironmentPayload", parseErr)
		return
	}

	upsertErr := s.upsertEnvironment(r.Context(), env)
	if upsertErr != nil {
		s.writeErrorResponse(
			w,
			statusFromError(upsertErr),
			uploadOperation,
			"UpsertEnvironment",
			upsertErr,
		)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

func (s *SerializationUploadService) upsertEnvironment(ctx context.Context, environment types.IEnvironment) error {
	if environment == nil {
		return common.NewErrBadRequest("AASENV-UPLOAD-NILENV parsed environment must not be nil")
	}

	return s.aasRepository.ExecuteInTransaction(func(tx *sql.Tx) error {
		return s.upsertEnvironmentInTransaction(ctx, tx, environment)
	})
}

func (s *SerializationUploadService) upsertEnvironmentInTransaction(ctx context.Context, tx *sql.Tx, environment types.IEnvironment) error {
	if tx == nil {
		return common.NewErrBadRequest("AASENV-UPLOAD-NILTX upload transaction must not be nil")
	}

	for _, submodel := range environment.Submodels() {
		if submodel == nil {
			continue
		}
		identifier := strings.TrimSpace(submodel.ID())
		if identifier == "" {
			return common.NewErrBadRequest("AASENV-UPLOAD-SM-MISSINGID submodel identifier is required")
		}
		if _, err := s.submodelRepository.PutSubmodelWithTxForEnvironment(ctx, tx, identifier, submodel); err != nil {
			return fmt.Errorf("AASENV-UPLOAD-SM-PUTSUBMODEL %w", err)
		}
	}

	for _, shell := range environment.AssetAdministrationShells() {
		if shell == nil {
			continue
		}
		identifier := strings.TrimSpace(shell.ID())
		if identifier == "" {
			return common.NewErrBadRequest("AASENV-UPLOAD-AAS-MISSINGID shell identifier is required")
		}
		if _, err := s.aasRepository.PutAssetAdministrationShellByIDWithTxForEnvironment(ctx, tx, identifier, shell); err != nil {
			return fmt.Errorf("AASENV-UPLOAD-AAS-PUTAAS %w", err)
		}
	}

	for _, conceptDescription := range environment.ConceptDescriptions() {
		if conceptDescription == nil {
			continue
		}
		identifier := strings.TrimSpace(conceptDescription.ID())
		if identifier == "" {
			return common.NewErrBadRequest("AASENV-UPLOAD-CD-MISSINGID concept description identifier is required")
		}
		if err := s.conceptDescriptionRepository.PutConceptDescriptionWithTxForEnvironment(ctx, tx, identifier, conceptDescription); err != nil {
			return fmt.Errorf("AASENV-UPLOAD-CD-PUTCD %w", err)
		}
	}

	return nil
}

func parseEnvironmentPayload(mediaType string, payload []byte) (types.IEnvironment, error) {
	normalizedMediaType := normalizeMediaType(mediaType)
	if normalizedMediaType == "" {
		return nil, common.NewErrBadRequest("AASENV-UPLOAD-MISSINGMEDIATYPE file part Content-Type is required")
	}

	switch normalizedMediaType {
	case mediaTypeJSON:
		return parseEnvironmentFromJSONBytes(payload)
	case mediaTypeXML:
		return parseEnvironmentFromXMLBytes(payload)
	case mediaTypeAASXXML, mediaTypeAASXMLAlias, mediaTypeAASXLegacyXMLBundle:
		return parseEnvironmentFromAASXBytes(payload, serializationKindXML)
	case mediaTypeAASXJSON, mediaTypeAASJSONAlias:
		return parseEnvironmentFromAASXBytes(payload, serializationKindJSON)
	default:
		return nil, common.NewErrBadRequest("AASENV-UPLOAD-UNSUPPORTEDMEDIATYPE unsupported file Content-Type: " + normalizedMediaType)
	}
}

func parseEnvironmentFromJSONBytes(payload []byte) (types.IEnvironment, error) {
	var jsonable any
	if err := json.Unmarshal(payload, &jsonable); err != nil {
		return nil, common.NewErrBadRequest("AASENV-PARSE-JSON-UNMARSHAL " + err.Error())
	}

	jsonMap, ok := jsonable.(map[string]any)
	if !ok {
		return nil, common.NewErrBadRequest("AASENV-PARSE-JSON-NOTOBJECT environment JSON payload must be an object")
	}

	environment, err := jsonization.EnvironmentFromJsonable(jsonMap)
	if err != nil {
		return nil, common.NewErrBadRequest("AASENV-PARSE-JSON-TOENV " + err.Error())
	}

	return environment, nil
}

func parseEnvironmentFromXMLBytes(payload []byte) (types.IEnvironment, error) {
	sanitizedPayload := sanitizeXMLPayload(payload)
	decoder := xml.NewDecoder(bytes.NewReader(sanitizedPayload))
	instance, err := xmlization.Unmarshal(decoder)
	if err != nil {
		return nil, common.NewErrBadRequest("AASENV-PARSE-XML-UNMARSHAL " + err.Error())
	}

	environment, ok := instance.(types.IEnvironment)
	if !ok {
		return nil, common.NewErrBadRequest("AASENV-PARSE-XML-NOTENV XML payload root must be environment")
	}

	return environment, nil
}

func parseEnvironmentFromAASXBytes(payload []byte, preferredKind serializationKind) (types.IEnvironment, error) {
	return parseEnvironmentFromAASXPackageBytes(payload, preferredKind)
}

func parseEnvironmentFromAASXPackageBytes(payload []byte, preferredKind serializationKind) (types.IEnvironment, error) {
	packaging := aasx.NewPackaging()
	pkg, err := packaging.OpenReadFromStream(bytes.NewReader(payload))
	if err != nil {
		return nil, common.NewErrBadRequest("AASENV-PARSE-AASX-OPEN " + err.Error())
	}
	defer func() {
		_ = pkg.Close()
	}()

	// TODO AASENV-UPLOAD-AASX-SUPPL: In addition to the selected spec part,
	// read supplementaries via pkg.SupplementariesFor or
	// pkg.SupplementaryRelationships and persist them so AASX attachments can be
	// restored during /serialization export.

	specsByContentType, specsErr := pkg.SpecsByContentType()
	if specsErr != nil {
		return nil, common.NewErrBadRequest("AASENV-PARSE-AASX-SPECS " + specsErr.Error())
	}

	selectedSpec, selectedKind, selectErr := selectPreferredSpec(specsByContentType, preferredKind)
	if selectErr != nil {
		return nil, selectErr
	}

	specPayload, readErr := selectedSpec.ReadAllBytes()
	if readErr != nil {
		return nil, common.NewErrBadRequest("AASENV-PARSE-AASX-READSPEC " + readErr.Error())
	}

	switch selectedKind {
	case serializationKindJSON:
		return parseEnvironmentFromJSONBytes(specPayload)
	case serializationKindXML:
		return parseEnvironmentFromXMLBytes(specPayload)
	default:
		return nil, common.NewErrBadRequest("AASENV-PARSE-AASX-UNSUPPORTEDSPEC unsupported spec format in AASX package")
	}
}

func selectPreferredSpec(specsByContentType map[string][]*aasx.Part, preferredKind serializationKind) (*aasx.Part, serializationKind, error) {
	candidates := make([]specCandidate, 0)

	for contentType, parts := range specsByContentType {
		for _, part := range parts {
			kind, supported := classifySpecContentType(contentType)
			if !supported && part != nil {
				kind, supported = classifySpecContentType(part.ContentType)
			}
			if !supported || part == nil {
				continue
			}

			score := 1
			if kind == preferredKind {
				score = 0
			}

			uri := ""
			if part.URI != nil {
				uri = part.URI.String()
			}

			candidates = append(candidates, specCandidate{
				part:  part,
				kind:  kind,
				uri:   uri,
				mime:  normalizeMediaType(contentType),
				score: score,
			})
		}
	}

	if len(candidates) == 0 {
		return nil, "", common.NewErrBadRequest("AASENV-PARSE-AASX-NOSUPPORTEDSPEC no supported JSON/XML spec part found in AASX package")
	}

	sort.Slice(candidates, func(i, j int) bool {
		if candidates[i].score != candidates[j].score {
			return candidates[i].score < candidates[j].score
		}
		if candidates[i].uri != candidates[j].uri {
			return candidates[i].uri < candidates[j].uri
		}
		return candidates[i].mime < candidates[j].mime
	})

	chosen := candidates[0]
	return chosen.part, chosen.kind, nil
}

func classifySpecContentType(contentType string) (serializationKind, bool) {
	normalized := normalizeMediaType(contentType)
	switch normalized {
	case mediaTypeJSON, "text/json", mediaTypeAASXJSON, mediaTypeAASJSONAlias:
		return serializationKindJSON, true
	case mediaTypeXML, "text/xml", mediaTypeAASXXML, mediaTypeAASXMLAlias, mediaTypeAASXLegacyXMLBundle:
		return serializationKindXML, true
	}

	if strings.Contains(normalized, "json") {
		return serializationKindJSON, true
	}
	if strings.Contains(normalized, "xml") {
		return serializationKindXML, true
	}

	return "", false
}

func readUploadedMultipartFile(w http.ResponseWriter, r *http.Request) ([]byte, string, error) {
	r.Body = http.MaxBytesReader(w, r.Body, maxUploadRequestBodyBytes)

	if err := r.ParseMultipartForm(32 << 20); err != nil {
		return nil, "", common.NewErrBadRequest("AASENV-UPLOAD-PARSEMULTIPART " + err.Error())
	}
	if r.MultipartForm != nil {
		defer func() {
			_ = r.MultipartForm.RemoveAll()
		}()
	}

	if r.MultipartForm == nil {
		return nil, "", common.NewErrBadRequest("AASENV-UPLOAD-NOMULTIPARTFORM multipart/form-data body is required")
	}

	fileHeaders := r.MultipartForm.File["file"]
	if len(fileHeaders) == 0 {
		return nil, "", common.NewErrBadRequest("AASENV-UPLOAD-MISSINGFILE required multipart field \"file\" is missing")
	}

	fileHeader := fileHeaders[0]
	if fileHeader.Size > maxUploadFileBytes {
		return nil, "", common.NewErrBadRequest("AASENV-UPLOAD-FILETOOLARGE multipart file exceeds maximum allowed size")
	}

	fileMediaType := normalizeMediaType(fileHeader.Header.Get("Content-Type"))
	if fileMediaType == "" {
		return nil, "", common.NewErrBadRequest("AASENV-UPLOAD-MISSINGFILEMEDIATYPE multipart file part Content-Type is required")
	}

	formFile, err := fileHeader.Open()
	if err != nil {
		return nil, "", common.NewErrBadRequest("AASENV-UPLOAD-OPENFILE " + err.Error())
	}
	defer func() {
		_ = formFile.Close()
	}()

	limitedReader := io.LimitReader(formFile, maxUploadFileBytes+1)
	payload, readErr := io.ReadAll(limitedReader)
	if readErr != nil {
		return nil, "", common.NewErrBadRequest("AASENV-UPLOAD-READFILE " + readErr.Error())
	}
	if int64(len(payload)) > maxUploadFileBytes {
		return nil, "", common.NewErrBadRequest("AASENV-UPLOAD-FILETOOLARGE multipart file exceeds maximum allowed size")
	}

	return payload, fileMediaType, nil
}

func sanitizeXMLPayload(payload []byte) []byte {
	sanitized := bytes.TrimSpace(payload)
	sanitized = bytes.TrimPrefix(sanitized, []byte("\uFEFF"))

	if bytes.HasPrefix(sanitized, []byte("<?xml")) {
		if declarationEnd := bytes.Index(sanitized, []byte("?>")); declarationEnd >= 0 {
			sanitized = sanitized[declarationEnd+2:]
		}
	}

	return bytes.TrimSpace(sanitized)
}
