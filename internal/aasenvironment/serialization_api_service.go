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
	"errors"
	"fmt"
	"hash/fnv"
	"io"
	"log/slog"
	"math"
	"mime"
	"net/http"
	"net/url"
	"path"
	"path/filepath"
	"sort"
	"strings"

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
)

type serializationSupplementaryPart struct {
	URI         *url.URL
	ContentType string
	Content     []byte
	SizeBytes   int64
	Consume     func(func(io.Reader) error) error
}

type serializationThumbnailPart struct {
	AASID       string
	URI         *url.URL
	ContentType string
	Content     []byte
	SizeBytes   int64
	Consume     func(func(io.Reader) error) error
}

// SerializationFileDownload defines a binary serialization response.
//
// WriteTo streams the staged serialization directly to the HTTP response.
// Close releases resources retained by WriteTo and is called by the HTTP
// adapter after streaming, including when streaming fails.
type SerializationFileDownload struct {
	WriteTo     func(io.Writer) error
	Close       func() error
	ContentType string
	Filename    string
}

// SerializationAPIService implements SerializationAPIAPIServicer.
type SerializationAPIService struct {
	persistence *Persistence
	stager      common.UploadStager
}

// NewSerializationAPIService creates a serialization service bound to the
// provided persistence layer.
//
// The service coordinates environment loading, media-type negotiation, and
// output packaging, while delegating repository access to persistence backends.
//
// Parameters:
//   - persistence: Repository facade used to load model and binary content.
//   - stager: Seekable storage used for the generated AASX specification part.
//
// Returns:
//   - *SerializationAPIService: Configured service instance.
func NewSerializationAPIService(persistence *Persistence, stager common.UploadStager) *SerializationAPIService {
	return &SerializationAPIService{persistence: persistence, stager: stager}
}

// GenerateSerializationByIds streams the explicitly requested AAS and
// submodels into a downloadable environment serialization.
//
// GenerateSerializationByIds negotiates the response media type from the
// request context and serializes one identifiable at a time into bounded
// staging storage, so memory use does not grow with the size of the request or
// the database. Only explicitly requested identifiables are serialized; when
// concept descriptions are included, only those referenced by the serialized
// submodels are added. AASX serializations additionally package the thumbnails
// and supplementary files of the serialized identifiables.
//
// Parameters:
//   - ctx: Request context containing authorization, Accept header, and limits.
//   - aasIds: Base64URL-encoded AAS identifiers to serialize.
//   - submodelIds: Base64URL-encoded submodel identifiers to serialize.
//   - includeConceptDescriptions: Whether referenced concept descriptions are included.
//
// Returns:
//   - model.ImplResponse: Download response or a mapped API error response;
//     400 when neither aasIds nor submodelIds is given and 413 when the
//     serialization exceeds the configured size limits.
//   - error: Reserved for transport-independent failures not represented in the response.
func (s *SerializationAPIService) GenerateSerializationByIds(ctx context.Context, aasIds []string, submodelIds []string, includeConceptDescriptions bool) (model.ImplResponse, error) {
	const operation = "GenerateSerializationByIds"

	request, requestErr := newSerializationRequest(aasIds, submodelIds, includeConceptDescriptions)
	if requestErr != nil {
		return errorResponseForOperation(requestErr, operation, "ValidateRequest"), nil
	}

	contentType, negotiateErr := negotiateSerializationContentType(common.AcceptHeaderFromContext(ctx))
	if negotiateErr != nil {
		return errorResponseForOperation(negotiateErr, operation, "NegotiateContentType"), nil
	}
	format, formatErr := resolveSerializationFormat(contentType)
	if formatErr != nil {
		return errorResponseForOperation(formatErr, operation, "ResolveFormat"), nil
	}

	staged, packageParts, stageErr := s.stageEnvironment(ctx, request, format)
	if stageErr != nil {
		return errorResponseForOperation(stageErr, operation, "StageEnvironment"), nil
	}

	if !format.aasx {
		return model.Response(http.StatusOK, SerializationFileDownload{
			WriteTo:     func(destination io.Writer) error { return copyStagedUpload(destination, staged) },
			Close:       staged.Close,
			ContentType: contentType,
			Filename:    format.fileName,
		}), nil
	}

	download, packageErr := newAASXSerializationDownload(ctx, staged, packageParts, format, contentType)
	if packageErr != nil {
		_ = staged.Close()
		return errorResponseForOperation(packageErr, operation, "ValidateAASXLimits"), nil
	}
	return model.Response(http.StatusOK, download), nil
}

func newAASXSerializationDownload(ctx context.Context, specification common.StagedUpload, packageParts *serializationPackageParts, format serializationFormat, contentType string) (SerializationFileDownload, error) {
	specificationSize, sizeErr := stagedUploadSize(specification)
	if sizeErr != nil {
		return SerializationFileDownload{}, sizeErr
	}
	limits := common.AASXLimitsFromContext(ctx)
	if limitErr := validateAASXSerializationSizeLimits(limits, specificationSize, packageParts.thumbnails, packageParts.supplementaries); limitErr != nil {
		return SerializationFileDownload{}, limitErr
	}
	return SerializationFileDownload{
		WriteTo: func(destination io.Writer) error {
			if _, err := specification.Seek(0, io.SeekStart); err != nil {
				return common.NewInternalServerError("AASENV-SERIALIZEAASX-SEEKSTAGEDSPEC " + err.Error())
			}
			return writeAASXPackageFromReader(destination, specification, format.specContentType, format.specURI, packageParts.thumbnails, packageParts.supplementaries, limits)
		},
		Close:       specification.Close,
		ContentType: contentType,
		Filename:    format.fileName,
	}, nil
}

func copyStagedUpload(destination io.Writer, staged common.StagedUpload) error {
	if _, err := staged.Seek(0, io.SeekStart); err != nil {
		return common.NewInternalServerError("AASENV-SERIALIZE-SEEKSTAGED " + err.Error())
	}
	if _, err := io.Copy(destination, staged); err != nil {
		return common.NewInternalServerError("AASENV-SERIALIZE-COPYSTAGED " + err.Error())
	}
	return nil
}

type serializationRequest struct {
	aasIDs                     []string
	submodelIDs                []string
	includeConceptDescriptions bool
}

func newSerializationRequest(aasIDs []string, submodelIDs []string, includeConceptDescriptions bool) (serializationRequest, error) {
	decodedAASIDs, decodeAASIDsErr := decodeIdentifiers(aasIDs, "AASENV-SERIALIZEREQ-DECODEAASID")
	if decodeAASIDsErr != nil {
		return serializationRequest{}, decodeAASIDsErr
	}
	decodedSubmodelIDs, decodeSubmodelIDsErr := decodeIdentifiers(submodelIDs, "AASENV-SERIALIZEREQ-DECODESUBMODELID")
	if decodeSubmodelIDsErr != nil {
		return serializationRequest{}, decodeSubmodelIDsErr
	}
	if len(decodedAASIDs) == 0 && len(decodedSubmodelIDs) == 0 {
		return serializationRequest{}, common.NewErrBadRequest("AASENV-SERIALIZEREQ-NOIDS at least one aasIds or submodelIds value is required")
	}
	return serializationRequest{
		aasIDs:                     deduplicateIdentifiers(decodedAASIDs),
		submodelIDs:                deduplicateIdentifiers(decodedSubmodelIDs),
		includeConceptDescriptions: includeConceptDescriptions,
	}, nil
}

func deduplicateIdentifiers(ids []string) []string {
	result := make([]string, 0, len(ids))
	seen := make(map[string]struct{}, len(ids))
	for _, id := range ids {
		if _, exists := seen[id]; exists {
			continue
		}
		seen[id] = struct{}{}
		result = append(result, id)
	}
	return result
}

type serializationFormat struct {
	specContentType string
	specURI         string
	fileName        string
	aasx            bool
}

func resolveSerializationFormat(contentType string) (serializationFormat, error) {
	switch contentType {
	case serializationContentTypeJSON:
		return serializationFormat{specContentType: serializationContentTypeJSON, fileName: "environment.json"}, nil
	case serializationContentTypeXML:
		return serializationFormat{specContentType: serializationContentTypeXML, fileName: "environment.xml"}, nil
	case serializationContentTypeAASXXML, serializationContentTypeAASXXMLAlt, serializationContentTypeAASXXMLPkg:
		return serializationFormat{specContentType: serializationContentTypeXML, specURI: serializationAASXXMLSpecURI, fileName: "environment.aasx", aasx: true}, nil
	case serializationContentTypeAASXJSON, serializationContentTypeAASXJSONAlt, serializationContentTypeAASXJSONPkg:
		return serializationFormat{specContentType: serializationContentTypeJSON, specURI: serializationAASXJSONSpecURI, fileName: "environment.aasx", aasx: true}, nil
	}
	return serializationFormat{}, common.NewErrBadRequest("AASENV-RESOLVEFORMAT-UNSUPPORTED unsupported serialization content type")
}

// stageEnvironment streams the requested environment into bounded staging
// storage and returns the staged serialization together with the AASX package
// parts discovered while streaming.
//
// The environment is produced in a separate goroutine that writes into a pipe
// consumed by the stager, so at most one identifiable is held in memory and the
// staged size is limited by the configured maximum part size.
func (s *SerializationAPIService) stageEnvironment(ctx context.Context, request serializationRequest, format serializationFormat) (common.StagedUpload, *serializationPackageParts, error) {
	if s == nil || s.persistence == nil || s.stager == nil {
		return nil, nil, common.NewInternalServerError("AASENV-STAGEENV-NILSERVICE persistence and upload stager are required")
	}
	maximum, limitErr := aasxStagingLimit(ctx)
	if limitErr != nil {
		return nil, nil, limitErr
	}
	packageParts, partsErr := s.newSerializationPackageParts(format)
	if partsErr != nil {
		return nil, nil, partsErr
	}

	reader, writer := io.Pipe()
	streamResult := make(chan error, 1)
	go func() {
		err := s.streamEnvironment(ctx, request, format.specContentType, packageParts, writer)
		_ = writer.CloseWithError(err)
		streamResult <- err
	}()
	staged, stageErr := s.stager(ctx, reader, maximum)
	_ = reader.Close()
	streamErr := <-streamResult

	if err := resolveEnvironmentStagingError(stageErr, streamErr); err != nil {
		if staged != nil {
			_ = staged.Close()
		}
		return nil, nil, err
	}
	return staged, packageParts, nil
}

// resolveEnvironmentStagingError prefers the producer error, which carries the
// API status of the failed lookup, unless the stager rejected the payload size
// or the producer only failed because the stager stopped reading.
func resolveEnvironmentStagingError(stageErr error, streamErr error) error {
	if streamErr != nil && !errors.Is(streamErr, io.ErrClosedPipe) && !common.IsErrPayloadTooLarge(stageErr) {
		return streamErr
	}
	if stageErr != nil {
		return stageErr
	}
	return streamErr
}

func (s *SerializationAPIService) streamEnvironment(ctx context.Context, request serializationRequest, specContentType string, packageParts *serializationPackageParts, destination io.Writer) error {
	writer, writerErr := newEnvironmentStreamWriter(specContentType, destination)
	if writerErr != nil {
		return writerErr
	}
	if err := s.streamAssetAdministrationShells(ctx, request.aasIDs, packageParts, writer); err != nil {
		return err
	}
	referencedConceptDescriptionIDs, submodelsErr := s.streamSubmodels(ctx, request, packageParts, writer)
	if submodelsErr != nil {
		return submodelsErr
	}
	if err := s.streamConceptDescriptions(ctx, referencedConceptDescriptionIDs, writer); err != nil {
		return err
	}
	return writer.close()
}

func (s *SerializationAPIService) streamAssetAdministrationShells(ctx context.Context, ids []string, packageParts *serializationPackageParts, writer environmentStreamWriter) error {
	for _, id := range ids {
		aas, getErr := s.persistence.AASRepository.GetAssetAdministrationShellByID(ctx, id)
		if getErr != nil {
			return getErr
		}
		if err := packageParts.addThumbnail(ctx, aas); err != nil {
			return err
		}
		if err := writer.writeIdentifiable(environmentSectionAssetAdministrationShells, aas); err != nil {
			return err
		}
	}
	return nil
}

func (s *SerializationAPIService) streamSubmodels(ctx context.Context, request serializationRequest, packageParts *serializationPackageParts, writer environmentStreamWriter) ([]string, error) {
	var references *conceptDescriptionReferences
	if request.includeConceptDescriptions {
		references = newConceptDescriptionReferences()
	}
	for _, id := range request.submodelIDs {
		submodel, getErr := s.persistence.SubmodelRepository.GetSubmodelByID(ctx, id, "deep", false, true)
		if getErr != nil {
			return nil, getErr
		}
		if err := packageParts.addSupplementaries(ctx, submodel); err != nil {
			return nil, err
		}
		references.collect(submodel)
		if err := writer.writeIdentifiable(environmentSectionSubmodels, submodel); err != nil {
			return nil, err
		}
	}
	return references.identifiers(), nil
}

// streamConceptDescriptions writes the referenced concept descriptions known
// to the repository. References to unknown or inaccessible concept
// descriptions are skipped because they are not part of the serialization.
func (s *SerializationAPIService) streamConceptDescriptions(ctx context.Context, ids []string, writer environmentStreamWriter) error {
	for _, id := range ids {
		conceptDescription, getErr := s.persistence.ConceptDescriptionRepository.GetConceptDescriptionByID(ctx, id)
		if common.IsErrNotFound(getErr) || common.IsErrDenied(getErr) {
			continue
		}
		if getErr != nil {
			return getErr
		}
		if err := writer.writeIdentifiable(environmentSectionConceptDescriptions, conceptDescription); err != nil {
			return err
		}
	}
	return nil
}

func stagedUploadSize(upload common.StagedUpload) (uint64, error) {
	if upload == nil {
		return 0, common.NewInternalServerError("AASENV-STAGEDSIZE-NILUPLOAD staged upload is required")
	}
	size := upload.Size()
	if size < 0 {
		return 0, common.NewInternalServerError("AASENV-STAGEDSIZE-NEGATIVE staged upload reported a negative size")
	}
	return uint64(size), nil
}

func aasxStagingLimit(ctx context.Context) (int64, error) {
	maximum := common.AASXLimitsFromContext(ctx).MaxPartExpandedSizeBytes
	if maximum > math.MaxInt64 {
		return 0, common.NewInternalServerError("AASENV-STAGELIMIT-OVERFLOW configured AASX part limit exceeds supported staging size")
	}
	return int64(maximum), nil
}

func validateAASXSerializationSizeLimits(
	limits common.AASXLimits,
	specificationSize uint64,
	thumbnailParts []serializationThumbnailPart,
	supplementaryParts []serializationSupplementaryPart,
) error {
	partCount := uint64(5 + len(thumbnailParts) + len(supplementaryParts))
	if len(thumbnailParts)+len(supplementaryParts) > 0 {
		partCount++
	}
	if partCount > limits.MaxPartCount {
		return common.NewErrPayloadTooLarge("AASENV-SERIALIZELIMIT-PARTCOUNT generated AASX package exceeds configured part count")
	}
	total := specificationSize
	if err := limits.CheckPartSize(total, false); err != nil {
		return err
	}
	for _, part := range supplementaryParts {
		size := serializationPartSize(part.SizeBytes, part.Content)
		if err := limits.CheckPartSize(size, false); err != nil {
			return err
		}
		total += size
		if total > limits.MaxTotalExpandedSizeBytes {
			return common.NewErrPayloadTooLarge("AASENV-SERIALIZELIMIT-TOTAL expanded AASX content exceeds configured maximum")
		}
	}
	for _, part := range thumbnailParts {
		size := serializationPartSize(part.SizeBytes, part.Content)
		if err := limits.CheckPartSize(size, true); err != nil {
			return err
		}
		total += size
		if total > limits.MaxTotalExpandedSizeBytes {
			return common.NewErrPayloadTooLarge("AASENV-SERIALIZELIMIT-TOTAL expanded AASX content exceeds configured maximum")
		}
	}
	if total > limits.MaxTotalExpandedSizeBytes {
		return common.NewErrPayloadTooLarge("AASENV-SERIALIZELIMIT-TOTAL expanded AASX content exceeds configured maximum")
	}
	return nil
}

func serializationPartSize(sizeBytes int64, content []byte) uint64 {
	if sizeBytes > 0 {
		return uint64(sizeBytes)
	}
	return uint64(len(content))
}

func writeAASXPackageFromReader(
	destination io.Writer,
	specContent io.Reader,
	specContentType string,
	specURIPath string,
	thumbnailParts []serializationThumbnailPart,
	supplementaryParts []serializationSupplementaryPart,
	limits common.AASXLimits,
) error {
	packaging := aasx.NewPackaging()
	pkg, createPackageErr := packaging.CreateWriter(destination)
	if createPackageErr != nil {
		return common.NewInternalServerError("AASENV-SERIALIZEAASX-CREATEPACKAGE " + createPackageErr.Error())
	}

	specURI, urlParsingErr := url.Parse(specURIPath)
	if urlParsingErr != nil {
		_ = pkg.Close()
		return common.NewInternalServerError("AASENV-SERIALIZEAASX-PARSEURL " + urlParsingErr.Error())
	}
	budget := &aasxExpandedBudget{remaining: limits.MaxTotalExpandedSizeBytes, maximum: limits.MaxTotalExpandedSizeBytes}
	limitedSpec := budget.wrap(common.NewPayloadLimitReader(specContent, limits.MaxPartExpandedSizeBytes, "specification"))
	spec, putSpecPartErr := pkg.PutPartFromStream(specURI, specContentType, limitedSpec)
	if putSpecPartErr != nil {
		_ = pkg.Close()
		return aasxPackageWriteError("AASENV-SERIALIZEAASX-PUTSPECPART", putSpecPartErr)
	}
	if makeSpecErr := pkg.MakeSpec(spec); makeSpecErr != nil {
		_ = pkg.Close()
		return common.NewInternalServerError("AASENV-SERIALIZEAASX-MAKESPEC " + makeSpecErr.Error())
	}

	for _, supplementaryPart := range supplementaryParts {
		var supplementary *aasx.Part
		putSupplementaryPartErr := consumeSerializationPart(supplementaryPart.Content, supplementaryPart.Consume, func(reader io.Reader) error {
			var putErr error
			limited := budget.wrap(common.NewPayloadLimitReader(reader, limits.MaxPartExpandedSizeBytes, "supplementary"))
			supplementary, putErr = pkg.PutPartFromStream(supplementaryPart.URI, supplementaryPart.ContentType, limited)
			return putErr
		})
		if putSupplementaryPartErr != nil {
			_ = pkg.Close()
			return aasxPackageWriteError("AASENV-SERIALIZEAASX-PUTSUPPLPART", putSupplementaryPartErr)
		}

		if relateSupplementaryErr := pkg.RelateSupplementaryToSpec(supplementary, spec); relateSupplementaryErr != nil {
			_ = pkg.Close()
			return common.NewInternalServerError("AASENV-SERIALIZEAASX-RELATESUPPL " + relateSupplementaryErr.Error())
		}
	}

	for index, thumbnailPart := range thumbnailParts {
		var thumb *aasx.Part
		putThumbPartErr := consumeSerializationPart(thumbnailPart.Content, thumbnailPart.Consume, func(reader io.Reader) error {
			var putErr error
			limited := budget.wrap(common.NewPayloadLimitReader(reader, limits.MaxThumbnailSizeBytes, "thumbnail"))
			thumb, putErr = pkg.PutPartFromStream(thumbnailPart.URI, thumbnailPart.ContentType, limited)
			return putErr
		})
		if putThumbPartErr != nil {
			_ = pkg.Close()
			return aasxPackageWriteError("AASENV-SERIALIZEAASX-PUTTHUMBPART", putThumbPartErr)
		}

		if relateThumbnailErr := pkg.RelateSupplementaryToSpec(thumb, spec); relateThumbnailErr != nil {
			_ = pkg.Close()
			return common.NewInternalServerError("AASENV-SERIALIZEAASX-RELATETHUMB " + relateThumbnailErr.Error())
		}

		if index == 0 {
			if setThumbnailErr := pkg.SetThumbnail(thumb); setThumbnailErr != nil {
				_ = pkg.Close()
				return common.NewInternalServerError("AASENV-SERIALIZEAASX-SETTHUMBNAIL " + setThumbnailErr.Error())
			}
		}
	}

	if closeErr := pkg.Close(); closeErr != nil {
		return common.NewInternalServerError("AASENV-SERIALIZEAASX-CLOSEPACKAGE " + closeErr.Error())
	}
	return nil
}

type aasxExpandedBudget struct {
	remaining uint64
	maximum   uint64
}

func (budget *aasxExpandedBudget) wrap(reader io.Reader) io.Reader {
	return &aasxExpandedBudgetReader{reader: reader, budget: budget}
}

type aasxExpandedBudgetReader struct {
	reader io.Reader
	budget *aasxExpandedBudget
}

func (reader *aasxExpandedBudgetReader) Read(destination []byte) (int, error) {
	if len(destination) == 0 {
		return 0, nil
	}
	if reader.budget.remaining > 0 {
		if uint64(len(destination)) > reader.budget.remaining {
			destination = destination[:reader.budget.remaining]
		}
		count, err := reader.reader.Read(destination)
		if count < 0 || count > len(destination) {
			return 0, common.NewInternalServerError("AASENV-SERIALIZEAASX-INVALIDREAD package stream returned an invalid byte count")
		}
		// #nosec G115 -- count is validated as non-negative and bounded by the destination length.
		reader.budget.remaining -= uint64(count)
		return count, err
	}
	var probe [1]byte
	count, err := reader.reader.Read(probe[:])
	if count > 0 {
		return 0, common.NewErrPayloadTooLarge(fmt.Sprintf(
			"AASENV-SERIALIZEAASX-TOTALEXPANDED package content exceeds configured maximum of %d bytes", reader.budget.maximum,
		))
	}
	return 0, err
}

func aasxPackageWriteError(code string, err error) error {
	if common.IsErrPayloadTooLarge(err) {
		return err
	}
	return common.NewInternalServerError(code + " " + err.Error())
}

func consumeSerializationPart(content []byte, consume func(func(io.Reader) error) error, use func(io.Reader) error) error {
	if use == nil {
		return common.NewInternalServerError("AASENV-SERIALIZEAASX-NILCONSUMER package part consumer is required")
	}
	if consume != nil {
		return consume(use)
	}
	return use(bytes.NewReader(content))
}

type serializationAttachmentMetadata struct {
	contentType string
	fileName    string
	size        int64
	preview     []byte
}

func (s *SerializationAPIService) resolveSerializationSupplementaryPart(
	ctx context.Context,
	specURI *url.URL,
	location AASXFileElementLocation,
) (*serializationSupplementaryPart, string, error) {
	resolvedReference, managed, ok := resolveInitialSupplementaryReference(location, specURI)
	if !ok {
		return nil, "", nil
	}
	metadata, found, err := s.loadSerializationAttachmentMetadata(ctx, location)
	if err != nil || !found {
		return nil, "", err
	}
	resolvedReference, contentType, ok := finalizeSupplementaryReference(resolvedReference, managed, metadata, location, specURI)
	if !ok {
		return nil, "", nil
	}
	if location.FileElement != nil {
		location.FileElement.SetValue(&resolvedReference)
	}
	part, err := s.buildStreamingSupplementaryPart(ctx, resolvedReference, contentType, metadata.size, location)
	return part, resolvedReference, err
}

func resolveInitialSupplementaryReference(location AASXFileElementLocation, specURI *url.URL) (string, bool, bool) {
	if IsExternalAASXReference(location.FileValue) {
		// #nosec G706 -- values are escaped by sanitizeLogValue to prevent control-character log injection.
		slog.Warn("external file reference skipped", "error.code", "AASENV-SERIALIZESUPPL-SKIPEXTERNAL", "submodel.id", sanitizeLogValue(location.SubmodelID), "id_short_path", sanitizeLogValue(location.IDShortPath))
		return "", false, false
	}
	managed := strings.HasPrefix(location.FileValue, serializationAASXSupplementaryRoot+"/")
	resolved := location.FileValue
	if !managed {
		resolved = ResolveAASXReferenceAgainstSpec(location.FileValue, specURI)
	}
	if resolved == "" {
		// #nosec G706 -- values are escaped by sanitizeLogValue to prevent control-character log injection.
		slog.Warn("unresolved file reference skipped", "error.code", "AASENV-SERIALIZESUPPL-SKIPUNRESOLVED", "submodel.id", sanitizeLogValue(location.SubmodelID), "id_short_path", sanitizeLogValue(location.IDShortPath))
		return "", false, false
	}
	return resolved, managed, true
}

func (s *SerializationAPIService) loadSerializationAttachmentMetadata(ctx context.Context, location AASXFileElementLocation) (serializationAttachmentMetadata, bool, error) {
	metadata := serializationAttachmentMetadata{}
	maximum := common.AASXLimitsFromContext(ctx).MaxPartExpandedSizeBytes
	err := s.persistence.SubmodelRepository.StreamFileAttachmentWithContext(ctx, location.SubmodelID, location.IDShortPath, func(contentType, fileName string, knownSize int64, reader io.Reader) error {
		metadata.contentType = contentType
		metadata.fileName = fileName
		metadata.size = knownSize
		return inspectSerializationAttachment(common.NewPayloadLimitReader(reader, maximum, "supplementary"), &metadata)
	})
	if common.IsErrNotFound(err) {
		// #nosec G706 -- values are escaped by sanitizeLogValue to prevent control-character log injection.
		slog.WarnContext(ctx, "supplementary attachment not found", "error.code", "AASENV-SERIALIZESUPPL-SKIPMISSING", "submodel.id", sanitizeLogValue(location.SubmodelID), "id_short_path", sanitizeLogValue(location.IDShortPath))
		return metadata, false, nil
	}
	if err != nil {
		if common.IsErrPayloadTooLarge(err) {
			return metadata, false, err
		}
		return metadata, false, common.NewInternalServerError("AASENV-SERIALIZESUPPL-DOWNLOAD " + err.Error())
	}
	if metadata.size == 0 {
		// #nosec G706 -- values are escaped by sanitizeLogValue to prevent control-character log injection.
		slog.WarnContext(ctx, "empty supplementary attachment skipped", "error.code", "AASENV-SERIALIZESUPPL-SKIPEMPTY", "submodel.id", sanitizeLogValue(location.SubmodelID), "id_short_path", sanitizeLogValue(location.IDShortPath))
		return metadata, false, nil
	}
	return metadata, true, nil
}

func inspectSerializationAttachment(reader io.Reader, metadata *serializationAttachmentMetadata) error {
	if strings.TrimSpace(metadata.contentType) == "" || metadata.size == 0 {
		preview, err := io.ReadAll(io.LimitReader(reader, 512))
		if err != nil {
			return err
		}
		metadata.preview = preview
	}
	if metadata.size == 0 {
		remaining, err := io.Copy(io.Discard, reader)
		metadata.size = int64(len(metadata.preview)) + remaining
		return err
	}
	return nil
}

func finalizeSupplementaryReference(resolved string, managed bool, metadata serializationAttachmentMetadata, location AASXFileElementLocation, specURI *url.URL) (string, string, bool) {
	contentType := strings.TrimSpace(metadata.contentType)
	if contentType == "" {
		contentType = strings.TrimSpace(http.DetectContentType(metadata.preview))
	}
	if contentType == "" {
		contentType = "application/octet-stream"
	}
	if !managed {
		resolved = ensureSupplementaryReferenceFileExtension(resolved, metadata.fileName, contentType, location.SubmodelID, location.IDShortPath)
		resolved = ResolveAASXSerializationSupplementaryPath(resolved, specURI, serializationAASXSupplementaryRoot)
	}
	if resolved == "" {
		// #nosec G706 -- values are escaped by sanitizeLogValue to prevent control-character log injection.
		slog.Warn("unresolved supplementary target skipped", "error.code", "AASENV-SERIALIZESUPPL-SKIPINVALIDTARGET", "submodel.id", sanitizeLogValue(location.SubmodelID), "id_short_path", sanitizeLogValue(location.IDShortPath))
		return "", "", false
	}
	return resolved, contentType, true
}

func (s *SerializationAPIService) buildStreamingSupplementaryPart(ctx context.Context, resolvedReference, contentType string, size int64, location AASXFileElementLocation) (*serializationSupplementaryPart, error) {
	uri, err := url.Parse(resolvedReference)
	if err != nil {
		return nil, common.NewInternalServerError("AASENV-SERIALIZESUPPL-PARSEURI " + err.Error())
	}
	maximum := common.AASXLimitsFromContext(ctx).MaxPartExpandedSizeBytes
	return &serializationSupplementaryPart{
		URI: uri, ContentType: contentType, SizeBytes: size,
		Consume: func(use func(io.Reader) error) error {
			return s.persistence.SubmodelRepository.StreamFileAttachmentWithContext(ctx, location.SubmodelID, location.IDShortPath, func(_ string, _ string, _ int64, reader io.Reader) error {
				return use(common.NewPayloadLimitReader(reader, maximum, "supplementary"))
			})
		},
	}, nil
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
		decodedID, decodeErr := common.DecodeAPIIdentifier(id)
		if decodeErr != nil {
			return nil, common.NewErrBadRequest(codePrefix + " " + decodeErr.Error())
		}
		result = append(result, decodedID)
	}

	return result, nil
}

type serializationThumbnailMetadata struct {
	serializationAttachmentMetadata
	path string
}

func (s *SerializationAPIService) resolveSerializationThumbnailPart(ctx context.Context, aasID string) (*serializationThumbnailPart, string, error) {
	metadata, found, err := s.loadSerializationThumbnailMetadata(ctx, aasID)
	if err != nil || !found {
		return nil, "", err
	}
	part, err := buildSerializationThumbnailPart(aasID, metadata.fileName, metadata.contentType, metadata.path, metadata.preview)
	if err != nil {
		return nil, "", common.NewInternalServerError("AASENV-SERIALIZETHUMB-BUILDPART " + err.Error())
	}
	resolvedURI := part.URI.String()
	if !strings.HasPrefix(resolvedURI, serializationAASXSupplementaryRoot+"/") {
		resolvedURI = NormalizeAASXPartURI(part.URI)
	}
	if resolvedURI == "" {
		// #nosec G706 -- values are escaped by sanitizeLogValue to prevent control-character log injection.
		slog.WarnContext(ctx, "invalid thumbnail URI skipped", "error.code", "AASENV-SERIALIZETHUMB-SKIPINVALIDURI", "aas.id", sanitizeLogValue(aasID))
		return nil, "", nil
	}
	maximum := common.AASXLimitsFromContext(ctx).MaxThumbnailSizeBytes
	part.Content = nil
	part.SizeBytes = metadata.size
	part.Consume = func(use func(io.Reader) error) error {
		return s.persistence.AASRepository.StreamThumbnailByAASID(ctx, aasID, func(_ string, _ string, _ string, _ int64, reader io.Reader) error {
			return use(common.NewPayloadLimitReader(reader, maximum, "thumbnail"))
		})
	}
	return &part, resolvedURI, nil
}

func (s *SerializationAPIService) loadSerializationThumbnailMetadata(ctx context.Context, aasID string) (serializationThumbnailMetadata, bool, error) {
	metadata := serializationThumbnailMetadata{}
	maximum := common.AASXLimitsFromContext(ctx).MaxThumbnailSizeBytes
	err := s.persistence.AASRepository.StreamThumbnailByAASID(ctx, aasID, func(contentType, fileName, path string, knownSize int64, reader io.Reader) error {
		metadata.contentType = contentType
		metadata.fileName = fileName
		metadata.path = path
		metadata.size = knownSize
		return inspectSerializationAttachment(common.NewPayloadLimitReader(reader, maximum, "thumbnail"), &metadata.serializationAttachmentMetadata)
	})
	if common.IsErrNotFound(err) {
		return metadata, false, nil
	}
	if err != nil {
		return metadata, false, err
	}
	return metadata, metadata.size > 0, nil
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
	case common.IsErrPayloadTooLarge(err):
		return common.NewErrorResponse(err, http.StatusRequestEntityTooLarge, componentName, operation, info)
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
