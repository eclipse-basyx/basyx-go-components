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

// Package aasenvironment provides the AAS environment serialization API service.
package aasenvironment

import (
	"bytes"
	"context"
	"encoding/json"
	"encoding/xml"
	"errors"
	"fmt"
	"hash/fnv"
	"io"
	"log"
	"mime"
	"net/http"
	"net/url"
	"path"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/FriedJannik/aas-go-sdk/jsonization"
	aastypes "github.com/FriedJannik/aas-go-sdk/types"
	aasxmlization "github.com/FriedJannik/aas-go-sdk/xmlization"
	aasx "github.com/aas-core-works/aas-package3-golang/v2"
	"github.com/eclipse-basyx/basyx-go-components/internal/common"
	"github.com/eclipse-basyx/basyx-go-components/internal/common/model"
)

const (
	componentName = "AASENV"

	serializationContentTypeJSON        = "application/json"
	serializationContentTypeXML         = "application/xml"
	serializationContentTypeAASXXML     = "application/aasx+xml"
	serializationContentTypeAASXXMLAlt  = "application/asset-administration-shell+xml"
	serializationContentTypeAASXXMLPkg  = "application/asset-administration-shell-package+xml"
	serializationContentTypeAASXJSON    = "application/aasx+json"
	serializationContentTypeAASXJSONAlt = "application/asset-administration-shell+json"
	serializationContentTypeAASXJSONPkg = "application/asset-administration-shell-package+json"
	serializationAASXXMLSpecURI         = "/aasx/xml/content.xml"
	serializationAASXJSONSpecURI        = "/aasx/json/content.json"
	serializationAASXSupplementaryRoot  = "/aasx/files"
	serializationAASXThumbnailRootPath  = "/"

	defaultListPageSize int32 = 100
)

type serializationSupplementaryPart struct {
	URI         *url.URL
	ContentType string
	Content     []byte
}

type serializationThumbnailPart struct {
	AASID       string
	URI         *url.URL
	ContentType string
	Content     []byte
}

type inMemoryReadWriteSeeker struct {
	content []byte
	offset  int64
}

func (s *inMemoryReadWriteSeeker) Read(target []byte) (int, error) {
	if s.offset >= int64(len(s.content)) {
		return 0, io.EOF
	}

	readCount := copy(target, s.content[s.offset:])
	s.offset += int64(readCount)
	return readCount, nil
}

func (s *inMemoryReadWriteSeeker) Write(source []byte) (int, error) {
	if s.offset < 0 {
		return 0, errors.New("negative stream offset")
	}

	maxInt := int64(int(^uint(0) >> 1))
	if s.offset > maxInt-int64(len(source)) {
		return 0, errors.New("stream size exceeds addressable memory")
	}

	requiredLength := int(s.offset) + len(source)
	if requiredLength > len(s.content) {
		s.grow(requiredLength, len(source))
	}

	copy(s.content[s.offset:], source)
	s.offset += int64(len(source))
	return len(source), nil
}

func (s *inMemoryReadWriteSeeker) grow(requiredLength int, sourceLength int) {
	if requiredLength <= cap(s.content) {
		s.content = s.content[:requiredLength]
		return
	}

	maxInt := int(^uint(0) >> 1)
	newCapacity := cap(s.content)
	if newCapacity == 0 {
		newCapacity = sourceLength
		if newCapacity < 1024 {
			newCapacity = 1024
		}
	}
	for newCapacity < requiredLength {
		if newCapacity > maxInt/2 {
			newCapacity = requiredLength
			break
		}
		newCapacity *= 2
	}

	grown := make([]byte, requiredLength, newCapacity)
	copy(grown, s.content)
	s.content = grown
}

func (s *inMemoryReadWriteSeeker) Seek(offset int64, whence int) (int64, error) {
	nextOffset := offset
	switch whence {
	case io.SeekStart:
	case io.SeekCurrent:
		nextOffset = s.offset + offset
	case io.SeekEnd:
		nextOffset = int64(len(s.content)) + offset
	default:
		return 0, fmt.Errorf("unsupported seek mode %d", whence)
	}
	if nextOffset < 0 {
		return 0, errors.New("negative stream offset")
	}

	s.offset = nextOffset
	return s.offset, nil
}

func (s *inMemoryReadWriteSeeker) Bytes() []byte {
	return s.content
}

// SerializationFileDownload defines a binary payload response for serialization downloads.
type SerializationFileDownload struct {
	Content     []byte
	ContentType string
	Filename    string
}

// SerializationAPIService implements SerializationAPIAPIServicer.
type SerializationAPIService struct {
	persistence *Persistence
}

// NewSerializationAPIService creates a serialization service bound to
// the provided persistence layer.
//
// The service coordinates environment loading, media-type negotiation, and
// output packaging, while delegating repository access to persistence backends.
func NewSerializationAPIService(persistence *Persistence) *SerializationAPIService {
	return &SerializationAPIService{persistence: persistence}
}

// GenerateSerializationByIds builds an environment from the requested AAS and
// submodel identifiers and returns it as a downloadable file.
//
// GenerateSerializationByIds negotiates the response media type from the
// request context, resolves thumbnail and supplementary package parts for AASX
// serializations, and maps domain errors to API responses with operation
// metadata.
func (s *SerializationAPIService) GenerateSerializationByIds(ctx context.Context, aasIds []string, submodelIds []string, includeConceptDescriptions bool) (model.ImplResponse, error) {
	const operation = "GenerateSerializationByIds"

	environment, loadErr := s.loadEnvironment(ctx, aasIds, submodelIds, includeConceptDescriptions)
	if loadErr != nil {
		return errorResponseForOperation(loadErr, operation, "LoadEnvironment"), nil
	}

	serializationContentType, negotiateErr := negotiateSerializationContentType(common.AcceptHeaderFromContext(ctx))
	if negotiateErr != nil {
		return errorResponseForOperation(negotiateErr, operation, "NegotiateContentType"), nil
	}

	thumbnailParts, thumbnailErr := s.resolveSerializationThumbnailParts(ctx, aasIds, environment, serializationContentType)
	if thumbnailErr != nil {
		return errorResponseForOperation(thumbnailErr, operation, "ResolveThumbnail"), nil
	}

	supplementaryParts, supplementaryErr := s.resolveSerializationSupplementaryParts(ctx, environment, serializationContentType)
	if supplementaryErr != nil {
		return errorResponseForOperation(supplementaryErr, operation, "ResolveSupplementaries"), nil
	}

	content, fileName, serializeErr := serializeEnvironment(environment, serializationContentType, thumbnailParts, supplementaryParts)
	if serializeErr != nil {
		return errorResponseForOperation(serializeErr, operation, "SerializeEnvironment"), nil
	}

	return model.Response(http.StatusOK, SerializationFileDownload{
		Content:     content,
		ContentType: serializationContentType,
		Filename:    fileName,
	}), nil
}

// loadEnvironment resolves all requested model fragments and assembles a
// single in-memory AAS environment.
//
// It decodes externally encoded identifiers, loads AAS, submodels, and
// optionally concept descriptions, and returns an environment object suitable
// for downstream serialization.
func (s *SerializationAPIService) loadEnvironment(ctx context.Context, aasIDs []string, submodelIDs []string, includeConceptDescriptions bool) (aastypes.IEnvironment, error) {
	if s == nil || s.persistence == nil {
		return nil, common.NewInternalServerError("AASENV-LOADENV-NILSERVICE service must not be nil")
	}

	decodedAASIDs, decodeAASIDsErr := decodeIdentifiers(aasIDs, "AASENV-LOADENV-DECODEAASID")
	if decodeAASIDsErr != nil {
		return nil, decodeAASIDsErr
	}

	decodedSubmodelIDs, decodeSubmodelIDsErr := decodeIdentifiers(submodelIDs, "AASENV-LOADENV-DECODESUBMODELID")
	if decodeSubmodelIDsErr != nil {
		return nil, decodeSubmodelIDsErr
	}

	assetAdministrationShells, loadAasErr := s.loadAssetAdministrationShells(ctx, decodedAASIDs)
	if loadAasErr != nil {
		return nil, loadAasErr
	}

	submodels, loadSubmodelsErr := s.loadSubmodels(ctx, decodedSubmodelIDs)
	if loadSubmodelsErr != nil {
		return nil, loadSubmodelsErr
	}

	conceptDescriptions, loadCDsErr := s.loadConceptDescriptions(ctx, includeConceptDescriptions)
	if loadCDsErr != nil {
		return nil, loadCDsErr
	}

	environment := aastypes.NewEnvironment()
	environment.SetAssetAdministrationShells(assetAdministrationShells)
	environment.SetSubmodels(submodels)
	environment.SetConceptDescriptions(conceptDescriptions)

	return environment, nil
}

// loadAssetAdministrationShells returns AAS entries by explicit identifiers or,
// when no identifiers are supplied, by traversing all pages from the repository.
//
// Pagination uses a fixed page size and continues until the backend cursor is
// empty.
func (s *SerializationAPIService) loadAssetAdministrationShells(ctx context.Context, ids []string) ([]aastypes.IAssetAdministrationShell, error) {
	if len(ids) > 0 {
		result := make([]aastypes.IAssetAdministrationShell, 0, len(ids))
		for _, id := range ids {
			aas, getErr := s.persistence.AASRepository.GetAssetAdministrationShellByID(ctx, id)
			if getErr != nil {
				return nil, getErr
			}
			result = append(result, aas)
		}
		return result, nil
	}

	result := make([]aastypes.IAssetAdministrationShell, 0)
	cursor := ""
	for {
		aasPage, nextCursor, getErr := s.persistence.AASRepository.GetAssetAdministrationShells(ctx, defaultListPageSize, cursor, "", nil, time.Time{}, time.Time{})
		if getErr != nil {
			return nil, getErr
		}

		result = append(result, aasPage...)
		if strings.TrimSpace(nextCursor) == "" {
			break
		}
		cursor = nextCursor
	}

	return result, nil
}

// loadSubmodels returns submodels by explicit identifiers or fetches all
// submodels via cursor-based pagination when no identifiers are provided.
//
// Both explicit and paginated loads populate deep submodel element trees to
// ensure complete serialization content.
func (s *SerializationAPIService) loadSubmodels(ctx context.Context, ids []string) ([]aastypes.ISubmodel, error) {
	if len(ids) > 0 {
		result := make([]aastypes.ISubmodel, 0, len(ids))
		for _, id := range ids {
			submodel, getErr := s.persistence.SubmodelRepository.GetSubmodelByID(ctx, id, "deep", false)
			if getErr != nil {
				return nil, getErr
			}
			result = append(result, submodel)
		}
		return result, nil
	}

	result := make([]aastypes.ISubmodel, 0)
	cursor := ""
	for {
		submodelPage, nextCursor, getErr := s.persistence.SubmodelRepository.GetSubmodels(ctx, defaultListPageSize, cursor, "", "", time.Time{}, time.Time{})
		if getErr != nil {
			return nil, getErr
		}

		for _, submodel := range submodelPage {
			if submodel == nil {
				return nil, common.NewInternalServerError("AASENV-LOADSUBMODELS-NILSUBMODEL loaded submodel must not be nil")
			}

			submodelID := strings.TrimSpace(submodel.ID())
			if submodelID == "" {
				return nil, common.NewInternalServerError("AASENV-LOADSUBMODELS-EMPTYSMID loaded submodel id must not be empty")
			}

			unlimited := -1
			submodelElements, _, getElementsErr := s.persistence.SubmodelRepository.GetSubmodelElements(ctx, submodelID, &unlimited, "", false, "deep")
			if getElementsErr != nil {
				return nil, getElementsErr
			}

			submodel.SetSubmodelElements(submodelElements)
		}

		result = append(result, submodelPage...)
		if strings.TrimSpace(nextCursor) == "" {
			break
		}
		cursor = nextCursor
	}

	return result, nil
}

// loadConceptDescriptions fetches concept descriptions only when requested.
//
// The function returns nil without querying repositories when
// includeConceptDescriptions is false, and otherwise pages through all concept
// descriptions.
func (s *SerializationAPIService) loadConceptDescriptions(ctx context.Context, includeConceptDescriptions bool) ([]aastypes.IConceptDescription, error) {
	if !includeConceptDescriptions {
		return nil, nil
	}

	result := make([]aastypes.IConceptDescription, 0)
	cursor := ""
	for {
		conceptDescriptionPage, nextCursor, getErr := s.persistence.ConceptDescriptionRepository.GetConceptDescriptions(ctx, nil, nil, nil, uint(defaultListPageSize), &cursor, time.Time{}, time.Time{})
		if getErr != nil {
			return nil, getErr
		}

		result = append(result, conceptDescriptionPage...)
		if strings.TrimSpace(nextCursor) == "" {
			break
		}
		cursor = nextCursor
	}

	return result, nil
}

// serializeEnvironment transforms the in-memory environment to the negotiated
// output format and returns payload bytes with a suggested filename.
//
// JSON and XML are emitted directly. AASX variants are delegated to AASX
// packaging helpers that can embed thumbnails and supplementary parts.
func serializeEnvironment(
	environment aastypes.IEnvironment,
	serializationContentType string,
	thumbnailParts []serializationThumbnailPart,
	supplementaryParts []serializationSupplementaryPart,
) ([]byte, string, error) {
	switch serializationContentType {
	case serializationContentTypeJSON:
		jsonableEnvironment, toJSONErr := jsonization.ToJsonable(environment)
		if toJSONErr != nil {
			return nil, "", common.NewInternalServerError("AASENV-SERIALIZE-TOJSONABLE " + toJSONErr.Error())
		}

		payload, marshalErr := json.Marshal(jsonableEnvironment)
		if marshalErr != nil {
			return nil, "", common.NewInternalServerError("AASENV-SERIALIZE-MARSHALJSON " + marshalErr.Error())
		}

		return payload, "environment.json", nil

	case serializationContentTypeXML:
		buffer := bytes.NewBuffer(nil)
		xmlEncoder := xml.NewEncoder(buffer)
		xmlEncoder.Indent("", "\t")
		if marshalErr := aasxmlization.Marshal(xmlEncoder, environment, true); marshalErr != nil {
			return nil, "", common.NewInternalServerError("AASENV-SERIALIZE-MARSHALXML " + marshalErr.Error())
		}
		if flushErr := xmlEncoder.Flush(); flushErr != nil {
			return nil, "", common.NewInternalServerError("AASENV-SERIALIZE-FLUSHXML " + flushErr.Error())
		}

		return buffer.Bytes(), "environment.xml", nil

	case serializationContentTypeAASXXML,
		serializationContentTypeAASXXMLAlt,
		serializationContentTypeAASXXMLPkg,
		serializationContentTypeAASXJSON,
		serializationContentTypeAASXJSONAlt,
		serializationContentTypeAASXJSONPkg:
		payload, aasxErr := serializeEnvironmentToAASX(environment, serializationContentType, thumbnailParts, supplementaryParts)
		if aasxErr != nil {
			return nil, "", aasxErr
		}

		return payload, "environment.aasx", nil
	}

	return nil, "", common.NewErrBadRequest("AASENV-SERIALIZE-UNSUPPORTEDCT unsupported serialization content type")
}

// serializeEnvironmentToAASX dispatches AASX serialization by requested media
// type.
//
// XML-based and JSON-based AASX media types are serialized through dedicated
// package writers. Any other media type is returned as an internal error.
func serializeEnvironmentToAASX(
	environment aastypes.IEnvironment,
	requestedContentType string,
	thumbnailParts []serializationThumbnailPart,
	supplementaryParts []serializationSupplementaryPart,
) ([]byte, error) {
	switch requestedContentType {
	case serializationContentTypeAASXXML, serializationContentTypeAASXXMLAlt, serializationContentTypeAASXXMLPkg:
		return serializeEnvironmentToAASXXML(environment, thumbnailParts, supplementaryParts)
	case serializationContentTypeAASXJSON, serializationContentTypeAASXJSONAlt, serializationContentTypeAASXJSONPkg:
		return serializeEnvironmentToAASXJSON(environment, thumbnailParts, supplementaryParts)
	}

	return nil, common.NewInternalServerError("AASENV-SERIALIZEAASX-UNSUPPORTEDCT unsupported AASX serialization content type")
}

// serializeEnvironmentToAASXXML creates an AASX package with an XML spec part,
// plus optional supplementary and thumbnail parts.
//
// The package is assembled in a temporary file, flushed, and then returned as
// raw bytes. All packaging and IO failures are wrapped with component-specific
// error codes.
func serializeEnvironmentToAASXXML(
	environment aastypes.IEnvironment,
	thumbnailParts []serializationThumbnailPart,
	supplementaryParts []serializationSupplementaryPart,
) ([]byte, error) {
	specContent, envToXMLErr := environmentToXMLBytes(environment)
	if envToXMLErr != nil {
		return nil, common.NewInternalServerError("AASENV-SERIALIZEAASX-TRANSFORMENV " + envToXMLErr.Error())
	}

	return serializeEnvironmentToAASXPackage(specContent, "application/xml", serializationAASXXMLSpecURI, thumbnailParts, supplementaryParts)
}

// serializeEnvironmentToAASXJSON creates an AASX package with a JSON spec
// part, plus optional supplementary and thumbnail parts.
func serializeEnvironmentToAASXJSON(
	environment aastypes.IEnvironment,
	thumbnailParts []serializationThumbnailPart,
	supplementaryParts []serializationSupplementaryPart,
) ([]byte, error) {
	specContent, envToJSONErr := environmentToJSONBytes(environment)
	if envToJSONErr != nil {
		return nil, common.NewInternalServerError("AASENV-SERIALIZEAASX-TRANSFORMENV " + envToJSONErr.Error())
	}

	return serializeEnvironmentToAASXPackage(specContent, "application/json", serializationAASXJSONSpecURI, thumbnailParts, supplementaryParts)
}

// serializeEnvironmentToAASXPackage assembles an AASX package from a prepared
// spec payload and optional supplementary and thumbnail parts.
func serializeEnvironmentToAASXPackage(
	specContent []byte,
	specContentType string,
	specURIPath string,
	thumbnailParts []serializationThumbnailPart,
	supplementaryParts []serializationSupplementaryPart,
) ([]byte, error) {
	packaging := aasx.NewPackaging()
	packageStream := &inMemoryReadWriteSeeker{}
	pkg, createPackageErr := packaging.CreateInStream(packageStream)
	if createPackageErr != nil {
		return nil, common.NewInternalServerError("AASENV-SERIALIZEAASX-CREATEPACKAGE " + createPackageErr.Error())
	}
	defer func() {
		if closePackageErr := pkg.Close(); closePackageErr != nil {
			log.Printf("[WARN] AASENV-SERIALIZEAASX-CLOSEPACKAGE %v", closePackageErr)
		}
	}()

	// set spec
	specURI, urlParsingErr := url.Parse(specURIPath)
	if urlParsingErr != nil {
		return nil, common.NewInternalServerError("AASENV-SERIALIZEAASX-PARSEURL " + urlParsingErr.Error())
	}
	spec, putSpecPartErr := pkg.PutPart(specURI, specContentType, specContent)
	if putSpecPartErr != nil {
		return nil, common.NewInternalServerError("AASENV-SERIALIZEAASX-PUTSPECPART " + putSpecPartErr.Error())
	}
	if makeSpecErr := pkg.MakeSpec(spec); makeSpecErr != nil {
		return nil, common.NewInternalServerError("AASENV-SERIALIZEAASX-MAKESPEC " + makeSpecErr.Error())
	}

	// set supplementaries
	for _, supplementaryPart := range supplementaryParts {
		supplementary, putSupplementaryPartErr := pkg.PutPart(supplementaryPart.URI, supplementaryPart.ContentType, supplementaryPart.Content)
		if putSupplementaryPartErr != nil {
			return nil, common.NewInternalServerError("AASENV-SERIALIZEAASX-PUTSUPPLPART " + putSupplementaryPartErr.Error())
		}

		if relateSupplementaryErr := pkg.RelateSupplementaryToSpec(supplementary, spec); relateSupplementaryErr != nil {
			return nil, common.NewInternalServerError("AASENV-SERIALIZEAASX-RELATESUPPL " + relateSupplementaryErr.Error())
		}
	}

	// set thumbnails
	for index, thumbnailPart := range thumbnailParts {
		thumb, putThumbPartErr := pkg.PutPart(thumbnailPart.URI, thumbnailPart.ContentType, thumbnailPart.Content)
		if putThumbPartErr != nil {
			return nil, common.NewInternalServerError("AASENV-SERIALIZEAASX-PUTTHUMBPART " + putThumbPartErr.Error())
		}

		if relateThumbnailErr := pkg.RelateSupplementaryToSpec(thumb, spec); relateThumbnailErr != nil {
			return nil, common.NewInternalServerError("AASENV-SERIALIZEAASX-RELATETHUMB " + relateThumbnailErr.Error())
		}

		if index == 0 {
			if setThumbnailErr := pkg.SetThumbnail(thumb); setThumbnailErr != nil {
				return nil, common.NewInternalServerError("AASENV-SERIALIZEAASX-SETTHUMBNAIL " + setThumbnailErr.Error())
			}
		}
	}

	// flush package
	if flushErr := pkg.Flush(); flushErr != nil {
		return nil, common.NewInternalServerError("AASENV-SERIALIZEAASX-FLUSHPKG " + flushErr.Error())
	}

	return packageStream.Bytes(), nil
}

// resolveSerializationSupplementaryParts collects supplementary payload parts
// for AASX serialization requests.
//
// It resolves internal file references from AAS file elements, downloads
// attachments from the submodel repository, rewrites file element values to
// packaged targets, and skips unresolved, external, empty, missing, or
// duplicate entries.
func (s *SerializationAPIService) resolveSerializationSupplementaryParts(
	ctx context.Context,
	environment aastypes.IEnvironment,
	serializationContentType string,
) ([]serializationSupplementaryPart, error) {
	if !isAASXSerializationContentType(serializationContentType) {
		return nil, nil
	}

	if s == nil || s.persistence == nil || s.persistence.SubmodelRepository == nil {
		return nil, common.NewInternalServerError("AASENV-SERIALIZESUPPL-NILSMREPO submodel repository backend is required")
	}
	if environment == nil {
		return nil, common.NewInternalServerError("AASENV-SERIALIZESUPPL-NILENV environment must not be nil")
	}

	specURIPath, resolveSpecURIErr := resolveAASXSpecURIByContentType(serializationContentType)
	if resolveSpecURIErr != nil {
		return nil, resolveSpecURIErr
	}

	specURI, parseSpecURIErr := url.Parse(specURIPath)
	if parseSpecURIErr != nil {
		return nil, common.NewInternalServerError("AASENV-SERIALIZESUPPL-PARSESPECURI " + parseSpecURIErr.Error())
	}

	fileLocations := CollectAASXFileElementLocations(environment)
	supplementaryParts := make([]serializationSupplementaryPart, 0, len(fileLocations))
	seenSupplementaries := make(map[string]struct{}, len(fileLocations))

	for _, fileLocation := range fileLocations {
		if IsExternalAASXReference(fileLocation.FileValue) {
			// #nosec G706 -- values are escaped by sanitizeLogValue to prevent control-character log injection.
			log.Printf("[WARN] AASENV-SERIALIZESUPPL-SKIPEXTERNAL skipping external file reference for submodel '%s' at path '%s'", sanitizeLogValue(fileLocation.SubmodelID), sanitizeLogValue(fileLocation.IDShortPath))
			continue
		}

		isManagedReference := strings.HasPrefix(fileLocation.FileValue, serializationAASXSupplementaryRoot+"/")
		resolvedReference := fileLocation.FileValue
		if !isManagedReference {
			resolvedReference = ResolveAASXReferenceAgainstSpec(fileLocation.FileValue, specURI)
		}
		if resolvedReference == "" {
			// #nosec G706 -- values are escaped by sanitizeLogValue to prevent control-character log injection.
			log.Printf("[WARN] AASENV-SERIALIZESUPPL-SKIPUNRESOLVED skipping unresolved file reference for submodel '%s' at path '%s'", sanitizeLogValue(fileLocation.SubmodelID), sanitizeLogValue(fileLocation.IDShortPath))
			continue
		}

		attachmentContent, attachmentContentType, attachmentFileName, downloadAttachmentErr := s.persistence.SubmodelRepository.DownloadFileAttachmentWithContext(ctx, fileLocation.SubmodelID, fileLocation.IDShortPath)
		if downloadAttachmentErr != nil {
			if common.IsErrNotFound(downloadAttachmentErr) {
				// #nosec G706 -- values are escaped by sanitizeLogValue to prevent control-character log injection.
				log.Printf("[WARN] AASENV-SERIALIZESUPPL-SKIPMISSING attachment not found for submodel '%s' at path '%s'", sanitizeLogValue(fileLocation.SubmodelID), sanitizeLogValue(fileLocation.IDShortPath))
				continue
			}
			return nil, common.NewInternalServerError("AASENV-SERIALIZESUPPL-DOWNLOAD " + downloadAttachmentErr.Error())
		}

		if len(attachmentContent) == 0 {
			// #nosec G706 -- values are escaped by sanitizeLogValue to prevent control-character log injection.
			log.Printf("[WARN] AASENV-SERIALIZESUPPL-SKIPEMPTY empty attachment for submodel '%s' at path '%s'", sanitizeLogValue(fileLocation.SubmodelID), sanitizeLogValue(fileLocation.IDShortPath))
			continue
		}

		resolvedContentType := strings.TrimSpace(attachmentContentType)
		if resolvedContentType == "" {
			resolvedContentType = strings.TrimSpace(http.DetectContentType(attachmentContent))
		}
		if resolvedContentType == "" {
			resolvedContentType = "application/octet-stream"
		}

		if !isManagedReference {
			resolvedReference = ensureSupplementaryReferenceFileExtension(
				resolvedReference,
				attachmentFileName,
				resolvedContentType,
				fileLocation.SubmodelID,
				fileLocation.IDShortPath,
			)
			resolvedReference = ResolveAASXSerializationSupplementaryPath(resolvedReference, specURI, serializationAASXSupplementaryRoot)
		}
		if resolvedReference == "" {
			// #nosec G706 -- values are escaped by sanitizeLogValue to prevent control-character log injection.
			log.Printf("[WARN] AASENV-SERIALIZESUPPL-SKIPINVALIDTARGET skipping unresolved supplementary target for submodel '%s' at path '%s'", sanitizeLogValue(fileLocation.SubmodelID), sanitizeLogValue(fileLocation.IDShortPath))
			continue
		}

		if fileLocation.FileElement != nil {
			rewrittenReference := resolvedReference
			fileLocation.FileElement.SetValue(&rewrittenReference)
		}

		if _, seen := seenSupplementaries[resolvedReference]; seen {
			// #nosec G706 -- values are escaped by sanitizeLogValue to prevent control-character log injection.
			log.Printf("[WARN] AASENV-SERIALIZESUPPL-SKIPDUPLICATE duplicate supplementary URI '%s' for submodel '%s' at path '%s'", sanitizeLogValue(resolvedReference), sanitizeLogValue(fileLocation.SubmodelID), sanitizeLogValue(fileLocation.IDShortPath))
			continue
		}

		supplementaryURI, parseSupplementaryURIErr := url.Parse(resolvedReference)
		if parseSupplementaryURIErr != nil {
			return nil, common.NewInternalServerError("AASENV-SERIALIZESUPPL-PARSEURI " + parseSupplementaryURIErr.Error())
		}

		seenSupplementaries[resolvedReference] = struct{}{}
		supplementaryParts = append(supplementaryParts, serializationSupplementaryPart{
			URI:         supplementaryURI,
			ContentType: resolvedContentType,
			Content:     attachmentContent,
		})
	}

	_ = ctx

	return supplementaryParts, nil
}

// resolveAASXSpecURIByContentType selects the canonical AASX spec part path
// for the negotiated serialization content type.
func resolveAASXSpecURIByContentType(contentType string) (string, error) {
	switch contentType {
	case serializationContentTypeAASXXML, serializationContentTypeAASXXMLAlt, serializationContentTypeAASXXMLPkg:
		return serializationAASXXMLSpecURI, nil
	case serializationContentTypeAASXJSON, serializationContentTypeAASXJSONAlt, serializationContentTypeAASXJSONPkg:
		return serializationAASXJSONSpecURI, nil
	default:
		return "", common.NewInternalServerError("AASENV-RESOLVEAASXSPECURI-UNSUPPORTEDCT unsupported AASX serialization content type")
	}
}

// sanitizeLogValue neutralizes control characters in untrusted log fields to
// prevent multiline/log-injection output while preserving visible content.
func sanitizeLogValue(value string) string {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		return ""
	}

	return strings.NewReplacer("\r", "\\r", "\n", "\\n", "\t", "\\t").Replace(trimmed)
}

// ensureSupplementaryReferenceFileExtension normalizes a supplementary target
// reference and replaces its filename with a deterministic export filename.
//
// The returned path preserves the original directory while enforcing a
// normalized package-compatible filename.
func ensureSupplementaryReferenceFileExtension(reference, attachmentFileName, contentType, submodelID, idShortPath string) string {
	trimmedReference := strings.TrimSpace(reference)
	if trimmedReference == "" {
		return ""
	}

	parsedReference, parseReferenceErr := url.Parse(trimmedReference)
	if parseReferenceErr != nil {
		return trimmedReference
	}

	normalizedPath := NormalizeAASXPartURI(parsedReference)
	exportFileName := supplementaryExportFileName(normalizedPath, attachmentFileName, contentType, submodelID, idShortPath)
	if exportFileName == "" {
		return normalizedPath
	}

	directory := path.Dir(normalizedPath)
	if directory == "." || directory == "" {
		directory = "/"
	}
	if directory == "/" {
		return "/" + exportFileName
	}

	return path.Join(directory, exportFileName)
}

// supplementaryExportFileName derives the packaged supplementary filename from
// attachment metadata and reference hints.
//
// It appends a deterministic suffix based on submodel and element path to
// avoid collisions across multiple exported files.
func supplementaryExportFileName(referencePath, attachmentFileName, contentType, submodelID, idShortPath string) string {
	resolvedFileName := normalizeAASXPartFileName(attachmentFileName)
	if resolvedFileName == "" {
		candidate := normalizeAASXPartFileName(path.Base(strings.TrimSpace(referencePath)))
		if candidate != "" && !isLikelyOIDFileName(candidate) {
			resolvedFileName = candidate
		}
	}
	if resolvedFileName == "" {
		resolvedFileName = "file"
	}

	fileExtension := normalizeFileExtension(filepath.Ext(resolvedFileName))
	baseName := strings.TrimSuffix(resolvedFileName, filepath.Ext(resolvedFileName))
	if baseName == "" {
		baseName = "file"
	}

	if fileExtension == "" {
		fileExtension = supplementaryFileExtension(resolvedFileName, contentType)
	}
	if fileExtension == "" {
		fileExtension = normalizeFileExtension(path.Ext(strings.TrimSpace(referencePath)))
	}

	return baseName + "-" + deterministicSupplementarySuffix(submodelID, idShortPath) + fileExtension
}

// deterministicSupplementarySuffix returns a stable hash suffix for a
// supplementary part based on submodel id and idShort path.
func deterministicSupplementarySuffix(submodelID, idShortPath string) string {
	hasher := fnv.New32a()
	_, _ = hasher.Write([]byte(strings.TrimSpace(submodelID)))
	_, _ = hasher.Write([]byte{':'})
	_, _ = hasher.Write([]byte(strings.TrimSpace(idShortPath)))
	return fmt.Sprintf("%08x", hasher.Sum32())
}

// isLikelyOIDFileName reports whether a filename base is purely numeric and
// long enough to look like an autogenerated OID-style name.
func isLikelyOIDFileName(fileName string) bool {
	trimmedFileName := strings.TrimSpace(fileName)
	if trimmedFileName == "" {
		return false
	}

	baseName := strings.TrimSuffix(trimmedFileName, filepath.Ext(trimmedFileName))
	if len(baseName) < 6 {
		return false
	}

	for _, char := range baseName {
		if char < '0' || char > '9' {
			return false
		}
	}

	return true
}

// supplementaryFileExtension resolves a preferred file extension from filename
// or media type information.
//
// It prefers an explicit filename extension, then uses a curated content-type
// map, and finally falls back to mime type lookup.
func supplementaryFileExtension(attachmentFileName, contentType string) string {
	if fileNameExtension := normalizeFileExtension(filepath.Ext(strings.TrimSpace(attachmentFileName))); fileNameExtension != "" {
		return fileNameExtension
	}

	parsedContentType, _, parseErr := mime.ParseMediaType(strings.TrimSpace(strings.ToLower(contentType)))
	if parseErr != nil || parsedContentType == "" {
		return ""
	}

	if preferredExtension, hasPreferredExtension := preferredSupplementaryExtensions[parsedContentType]; hasPreferredExtension {
		return preferredExtension
	}

	extensions, extensionsErr := mime.ExtensionsByType(parsedContentType)
	if extensionsErr != nil || len(extensions) == 0 {
		return ""
	}

	sort.Strings(extensions)
	for _, extension := range extensions {
		if normalizedExtension := normalizeFileExtension(extension); normalizedExtension != "" {
			return normalizedExtension
		}
	}

	return ""
}

// normalizeFileExtension normalizes an extension token to lower-case dot
// notation and rejects invalid path-like values.
func normalizeFileExtension(value string) string {
	normalized := strings.TrimSpace(strings.ToLower(value))
	if normalized == "" {
		return ""
	}
	if !strings.HasPrefix(normalized, ".") {
		normalized = "." + normalized
	}
	if strings.ContainsAny(normalized, "\\/\t\n\r") {
		return ""
	}
	if normalized == "." {
		return ""
	}

	return normalized
}

var preferredSupplementaryExtensions = map[string]string{
	"application/json": ".json",
	"application/pdf":  ".pdf",
	"application/xml":  ".xml",
	"image/jpeg":       ".jpg",
	"image/png":        ".png",
	"image/svg+xml":    ".svg",
	"text/plain":       ".txt",
}

// decodeIdentifiers decodes externally encoded identifiers in request input.
//
// If any identifier cannot be decoded, the function aborts and returns a bad
// request error prefixed with the provided code to retain operation context.
func decodeIdentifiers(ids []string, codePrefix string) ([]string, error) {
	result := make([]string, 0, len(ids))
	for _, id := range ids {
		decodedID, decodeErr := common.DecodeString(id)
		if decodeErr != nil {
			return nil, common.NewErrBadRequest(codePrefix + " " + decodeErr.Error())
		}
		result = append(result, decodedID)
	}

	return result, nil
}

// resolveSerializationThumbnailParts determines and loads thumbnails for AASX
// serialization outputs.
//
// For non-AASX media types it intentionally returns no thumbnail parts and no
// error. For AASX requests it resolves all non-empty thumbnails for requested
// or serialized AAS entries, rewrites each AAS defaultThumbnail reference to a
// deterministic packaged target path, and returns all package parts.
func (s *SerializationAPIService) resolveSerializationThumbnailParts(
	ctx context.Context,
	requestedAASIDs []string,
	environment aastypes.IEnvironment,
	serializationContentType string,
) ([]serializationThumbnailPart, error) {
	if !isAASXSerializationContentType(serializationContentType) {
		return nil, nil
	}

	if s == nil || s.persistence == nil || s.persistence.AASRepository == nil {
		return nil, common.NewInternalServerError("AASENV-SERIALIZETHUMB-NILAASREPO AAS repository backend is required")
	}

	resolvedAASIDs, resolveErr := resolveSerializationThumbnailAASIDs(requestedAASIDs, environment)
	if resolveErr != nil {
		return nil, resolveErr
	}
	if len(resolvedAASIDs) == 0 {
		return nil, nil
	}

	thumbnailParts := make([]serializationThumbnailPart, 0, len(resolvedAASIDs))
	seenThumbnailURIs := make(map[string]struct{}, len(resolvedAASIDs))

	for _, aasID := range resolvedAASIDs {
		thumbnailContent, thumbnailContentType, thumbnailFileName, thumbnailPath, foundThumbnail, thumbnailErr := s.loadSerializationThumbnailByAASID(ctx, aasID)
		if thumbnailErr != nil {
			return nil, thumbnailErr
		}
		if !foundThumbnail {
			continue
		}

		thumbnailPart, buildPartErr := buildSerializationThumbnailPart(aasID, thumbnailFileName, thumbnailContentType, thumbnailPath, thumbnailContent)
		if buildPartErr != nil {
			return nil, common.NewInternalServerError("AASENV-SERIALIZETHUMB-BUILDPART " + buildPartErr.Error())
		}

		resolvedThumbnailURI := thumbnailPart.URI.String()
		if !strings.HasPrefix(resolvedThumbnailURI, serializationAASXSupplementaryRoot+"/") {
			resolvedThumbnailURI = NormalizeAASXPartURI(thumbnailPart.URI)
		}
		if resolvedThumbnailURI == "" {
			// #nosec G706 -- values are escaped by sanitizeLogValue to prevent control-character log injection.
			log.Printf("[WARN] AASENV-SERIALIZETHUMB-SKIPINVALIDURI skipping invalid thumbnail URI for AAS '%s'", sanitizeLogValue(aasID))
			continue
		}

		if _, seen := seenThumbnailURIs[resolvedThumbnailURI]; seen {
			// #nosec G706 -- values are escaped by sanitizeLogValue to prevent control-character log injection.
			log.Printf("[WARN] AASENV-SERIALIZETHUMB-SKIPDUPLICATE skipping duplicate thumbnail URI '%s' for AAS '%s'", sanitizeLogValue(resolvedThumbnailURI), sanitizeLogValue(aasID))
			continue
		}
		seenThumbnailURIs[resolvedThumbnailURI] = struct{}{}

		rewriteSerializationThumbnailReference(environment, aasID, resolvedThumbnailURI, thumbnailPart.ContentType)
		thumbnailParts = append(thumbnailParts, thumbnailPart)
	}

	return thumbnailParts, nil
}

// resolveSerializationThumbnailAASIDs determines which AAS identifiers should
// be used for thumbnail loading.
//
// Explicit request ids are decoded and deduplicated. When none are provided,
// ids are derived from the serialized environment.
func resolveSerializationThumbnailAASIDs(requestedAASIDs []string, environment aastypes.IEnvironment) ([]string, error) {
	if len(requestedAASIDs) > 0 {
		decodedIDs, decodeErr := resolveRequestedThumbnailAASIDs(requestedAASIDs)
		if decodeErr != nil {
			return nil, decodeErr
		}
		return deduplicateTrimmedIdentifiers(decodedIDs), nil
	}

	if environment == nil {
		return nil, common.NewInternalServerError("AASENV-SERIALIZETHUMB-NILENV environment must not be nil")
	}

	aasIDs := make([]string, 0, len(environment.AssetAdministrationShells()))
	for _, aas := range environment.AssetAdministrationShells() {
		if aas == nil {
			continue
		}
		aasIDs = append(aasIDs, aas.ID())
	}

	return deduplicateTrimmedIdentifiers(aasIDs), nil
}

// deduplicateTrimmedIdentifiers removes empty and duplicate identifiers while
// preserving first-seen order.
func deduplicateTrimmedIdentifiers(values []string) []string {
	if len(values) == 0 {
		return nil
	}

	result := make([]string, 0, len(values))
	seenValues := make(map[string]struct{}, len(values))
	for _, value := range values {
		trimmedValue := strings.TrimSpace(value)
		if trimmedValue == "" {
			continue
		}
		if _, exists := seenValues[trimmedValue]; exists {
			continue
		}
		seenValues[trimmedValue] = struct{}{}
		result = append(result, trimmedValue)
	}

	if len(result) == 0 {
		return nil
	}

	return result
}

// loadSerializationThumbnailByAASID loads thumbnail data for one AAS id and
// reports whether a usable thumbnail exists.
//
// Not found and empty-content cases are treated as "not available" without an
// error.
func (s *SerializationAPIService) loadSerializationThumbnailByAASID(ctx context.Context, aasID string) ([]byte, string, string, string, bool, error) {
	thumbnailContent, thumbnailContentType, thumbnailFileName, thumbnailPath, thumbnailErr := s.persistence.AASRepository.GetThumbnailByAASID(ctx, aasID)
	if thumbnailErr != nil {
		if common.IsErrNotFound(thumbnailErr) {
			return nil, "", "", "", false, nil
		}
		return nil, "", "", "", false, thumbnailErr
	}

	if len(thumbnailContent) == 0 {
		return nil, "", "", "", false, nil
	}

	return thumbnailContent, strings.TrimSpace(thumbnailContentType), strings.TrimSpace(thumbnailFileName), thumbnailPath, true, nil
}

// buildSerializationThumbnailPart creates one thumbnail package part with a
// normalized URI and resolved content type.
func buildSerializationThumbnailPart(aasID, thumbnailFileName, thumbnailContentType, thumbnailPath string, thumbnailContent []byte) (serializationThumbnailPart, error) {
	resolvedThumbnailContentType := strings.TrimSpace(thumbnailContentType)
	if resolvedThumbnailContentType == "" {
		resolvedThumbnailContentType = strings.TrimSpace(http.DetectContentType(thumbnailContent))
	}
	if resolvedThumbnailContentType == "" {
		resolvedThumbnailContentType = "application/octet-stream"
	}

	partPath := strings.TrimSpace(thumbnailPath)
	if !strings.HasPrefix(partPath, serializationAASXSupplementaryRoot+"/") {
		partPath = path.Join(serializationAASXThumbnailRootPath, thumbnailExportFileName(thumbnailFileName, resolvedThumbnailContentType, aasID))
	}
	thumbnailURI, resolveThumbnailURIErr := url.Parse(partPath)
	if resolveThumbnailURIErr != nil {
		return serializationThumbnailPart{}, resolveThumbnailURIErr
	}

	return serializationThumbnailPart{
		AASID:       aasID,
		URI:         thumbnailURI,
		ContentType: resolvedThumbnailContentType,
		Content:     thumbnailContent,
	}, nil
}

// rewriteSerializationThumbnailReference updates the matching AAS
// defaultThumbnail reference in the environment to the packaged thumbnail path.
//
// If no thumbnail exists on the asset information, a new resource is created.
func rewriteSerializationThumbnailReference(environment aastypes.IEnvironment, aasID, thumbnailPath, thumbnailContentType string) {
	if environment == nil {
		return
	}

	for _, aas := range environment.AssetAdministrationShells() {
		if aas == nil || strings.TrimSpace(aas.ID()) != strings.TrimSpace(aasID) {
			continue
		}

		assetInformation := aas.AssetInformation()
		if assetInformation == nil {
			return
		}

		defaultThumbnail := assetInformation.DefaultThumbnail()
		if defaultThumbnail == nil {
			defaultThumbnail = aastypes.NewResource(thumbnailPath)
			assetInformation.SetDefaultThumbnail(defaultThumbnail)
		} else {
			defaultThumbnail.SetPath(thumbnailPath)
		}

		resolvedThumbnailContentType := strings.TrimSpace(thumbnailContentType)
		if resolvedThumbnailContentType != "" {
			contentTypeValue := resolvedThumbnailContentType
			defaultThumbnail.SetContentType(&contentTypeValue)
		}

		return
	}
}

// thumbnailExportFileName creates the final exported thumbnail filename with a
// deterministic AAS-based suffix.
func thumbnailExportFileName(fileName, contentType, aasID string) string {
	resolvedFileName := normalizeAASXPartFileName(fileName)
	if resolvedFileName == "" {
		resolvedFileName = "thumbnail"
	}

	baseName := strings.TrimSuffix(resolvedFileName, filepath.Ext(resolvedFileName))
	if baseName == "" {
		baseName = "thumbnail"
	}

	fileExtension := normalizeFileExtension(filepath.Ext(resolvedFileName))
	if fileExtension == "" {
		fileExtension = supplementaryFileExtension(resolvedFileName, contentType)
	}
	if fileExtension == "" {
		fileExtension = ".bin"
	}

	return baseName + "-" + deterministicThumbnailSuffix(aasID) + fileExtension
}

// deterministicThumbnailSuffix returns a stable hash suffix for a thumbnail
// part based on AAS id.
func deterministicThumbnailSuffix(aasID string) string {
	hasher := fnv.New32a()
	_, _ = hasher.Write([]byte(strings.TrimSpace(aasID)))
	return fmt.Sprintf("%08x", hasher.Sum32())
}

// normalizeAASXPartFileName reduces a path-like filename input to a clean
// basename suitable for AASX part names.
func normalizeAASXPartFileName(fileName string) string {
	normalizedFileName := strings.TrimSpace(fileName)
	if normalizedFileName == "" {
		return ""
	}

	normalizedFileName = strings.ReplaceAll(normalizedFileName, "\\", "/")
	normalizedFileName = strings.TrimSpace(path.Base(normalizedFileName))
	if normalizedFileName == "" || normalizedFileName == "." || normalizedFileName == "/" {
		return ""
	}

	return normalizedFileName
}

// resolveRequestedThumbnailAASIDs decodes explicit request identifiers for
// thumbnail lookup.
func resolveRequestedThumbnailAASIDs(requestedAASIDs []string) ([]string, error) {
	decodedIDs, decodeErr := decodeIdentifiers(requestedAASIDs, "AASENV-SERIALIZETHUMB-DECODEAASID")
	if decodeErr != nil {
		return nil, decodeErr
	}

	return decodedIDs, nil
}

// isAASXSerializationContentType reports whether the media type represents one
// of the supported AASX serialization variants.
func isAASXSerializationContentType(contentType string) bool {
	switch contentType {
	case serializationContentTypeAASXXML,
		serializationContentTypeAASXXMLAlt,
		serializationContentTypeAASXJSON,
		serializationContentTypeAASXJSONAlt,
		serializationContentTypeAASXXMLPkg,
		serializationContentTypeAASXJSONPkg:
		return true
	default:
		return false
	}
}

// parseMediaType normalizes and validates a single media type token.
//
// It strips parameters (for example charset values), lowercases the base media
// type, and returns a bad request error when the token is empty or malformed.
func parseMediaType(contentType string) (string, error) {
	trimmed := strings.TrimSpace(contentType)
	if trimmed == "" {
		return "", common.NewErrBadRequest("AASENV-PARSEMEDIATYPE-EMPTY missing content type")
	}

	parsedContentType, _, parseErr := mime.ParseMediaType(trimmed)
	if parseErr == nil {
		return strings.ToLower(strings.TrimSpace(parsedContentType)), nil
	}

	// Accept headers from clients occasionally contain a trailing semicolon or
	// non-critical parameter formatting issues. Fall back to the media-type token.
	fallbackContentType := strings.TrimSpace(strings.SplitN(trimmed, ";", 2)[0])
	if fallbackContentType == "" {
		return "", common.NewErrBadRequest("AASENV-PARSEMEDIATYPE-INVALID " + parseErr.Error())
	}

	return strings.ToLower(fallbackContentType), nil
}

// negotiateSerializationContentType resolves the effective serialization media
// type from the HTTP Accept header value.
//
// Empty or wildcard values default to AASX XML. Unsupported values yield a bad
// request error.
func negotiateSerializationContentType(acceptHeader string) (string, error) {
	trimmedAccept := strings.TrimSpace(acceptHeader)
	if trimmedAccept == "" {
		return serializationContentTypeAASXXML, nil
	}

	for _, part := range strings.Split(trimmedAccept, ",") {
		mediaType, parseErr := parseMediaType(part)
		if parseErr != nil {
			continue
		}

		switch mediaType {
		case "*/*", "application/*":
			return serializationContentTypeAASXXML, nil
		case serializationContentTypeJSON,
			serializationContentTypeXML,
			serializationContentTypeAASXXML,
			serializationContentTypeAASXXMLAlt,
			serializationContentTypeAASXXMLPkg,
			serializationContentTypeAASXJSON,
			serializationContentTypeAASXJSONAlt,
			serializationContentTypeAASXJSONPkg:
			return mediaType, nil
		}
	}

	return "", common.NewErrBadRequest("AASENV-NEGOTIATEMEDIATYPE-UNSUPPORTED unsupported Accept header")
}

// errorResponseForOperation maps domain errors to HTTP status-aware API error
// responses with component and operation metadata.
//
// Nil errors are normalized to an internal server error to keep response
// handling deterministic.
func errorResponseForOperation(err error, operation string, info string) model.ImplResponse {
	if err == nil {
		err = errors.New(http.StatusText(http.StatusInternalServerError))
	}

	switch {
	case common.IsErrBadRequest(err):
		return common.NewErrorResponse(err, http.StatusBadRequest, componentName, operation, info)
	case common.IsErrNotFound(err):
		return common.NewErrorResponse(err, http.StatusNotFound, componentName, operation, info)
	case common.IsErrDenied(err):
		return common.NewErrorResponse(err, http.StatusForbidden, componentName, operation, info)
	case common.IsErrConflict(err):
		return common.NewErrorResponse(err, http.StatusConflict, componentName, operation, info)
	default:
		return common.NewErrorResponse(err, http.StatusInternalServerError, componentName, operation, info)
	}
}

// environmentToJSONBytes serializes an environment to JSON bytes using the AAS
// jsonization model and encoding/json.
func environmentToJSONBytes(env aastypes.IEnvironment) ([]byte, error) {
	jsonableEnv, err := jsonization.ToJsonable(env)
	if err != nil {
		return nil, err
	}

	payload, err := json.Marshal(jsonableEnv)
	if err != nil {
		return nil, err
	}

	return payload, nil
}

// environmentToXMLBytes serializes an environment to XML bytes using the AAS
// XML marshaller and pretty indentation.
func environmentToXMLBytes(env aastypes.IEnvironment) ([]byte, error) {
	buffer := bytes.NewBuffer(nil)
	xmlEncoder := xml.NewEncoder(buffer)
	xmlEncoder.Indent("", "\t")

	if err := aasxmlization.Marshal(xmlEncoder, env, true); err != nil {
		return nil, err
	}

	if err := xmlEncoder.Flush(); err != nil {
		return nil, err
	}

	return buffer.Bytes(), nil
}
