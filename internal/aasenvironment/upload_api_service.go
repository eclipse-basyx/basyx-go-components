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
	"context"
	"database/sql"
	"encoding/json"
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"mime"
	"net/http"
	"net/url"
	"path"
	"path/filepath"
	"strconv"
	"strings"

	aasjsonization "github.com/FriedJannik/aas-go-sdk/jsonization"
	aastypes "github.com/FriedJannik/aas-go-sdk/types"
	aasxmlization "github.com/FriedJannik/aas-go-sdk/xmlization"
	aasx "github.com/aas-core-works/aas-package3-golang/v2"
	"github.com/eclipse-basyx/basyx-go-components/internal/common"
	"github.com/eclipse-basyx/basyx-go-components/internal/common/asyncjob"
	"github.com/eclipse-basyx/basyx-go-components/internal/common/asyncupload"
	"github.com/eclipse-basyx/basyx-go-components/internal/common/binarycontent"
	"github.com/eclipse-basyx/basyx-go-components/internal/common/history"
	commonmodel "github.com/eclipse-basyx/basyx-go-components/internal/common/model"
	auth "github.com/eclipse-basyx/basyx-go-components/internal/common/security"
)

// UploadAPIService imports uploaded AAS Environments synchronously and as
// asynchronous jobs, each in one database transaction.
type UploadAPIService struct {
	persistence               *Persistence
	aasRepositoryService      aasRepositoryUploadWriter
	submodelRepositoryService submodelRepositoryUploadWriter
	asyncJobs                 *asyncjob.Manager
	asyncUploads              *asyncupload.Store
	transaction               *sql.Tx
}

// UploadServiceOption configures an UploadAPIService.
type UploadServiceOption func(*UploadAPIService)

// WithAsyncUploads enables asynchronous uploads backed by the given job
// manager and durable staging store.
func WithAsyncUploads(manager *asyncjob.Manager, uploads *asyncupload.Store) UploadServiceOption {
	return func(service *UploadAPIService) {
		service.asyncJobs = manager
		service.asyncUploads = uploads
	}
}

type aasRepositoryUploadWriter interface {
	UpsertAssetAdministrationShellInTransaction(ctx context.Context, tx *sql.Tx, shell aastypes.IAssetAdministrationShell) error
}

type submodelRepositoryUploadWriter interface {
	UpsertSubmodelInTransaction(ctx context.Context, tx *sql.Tx, submodel aastypes.ISubmodel) error
}

const (
	uploadComponent             = "AASENV"
	uploadOperation             = "HandleUpload"
	supportedUploadContentTypes = "application/aasx+xml, application/aasx+json, application/asset-administration-shell+xml, application/asset-administration-shell+json, application/json, application/xml, text/xml"
	currentAASNamespace         = "https://admin-shell.io/aas/3/2"
	aasNamespacePrefix          = "https://admin-shell.io/aas/"
)

// NewUploadAPIService creates a new UploadAPIService
func NewUploadAPIService(
	persistence *Persistence,
	aasRepositoryService aasRepositoryUploadWriter,
	submodelRepositoryService submodelRepositoryUploadWriter,
	options ...UploadServiceOption,
) *UploadAPIService {
	service := &UploadAPIService{
		persistence:               persistence,
		aasRepositoryService:      aasRepositoryService,
		submodelRepositoryService: submodelRepositoryService,
	}
	for _, option := range options {
		option(service)
	}
	return service
}

// HandleUpload imports an uploaded environment in one transaction: its own, or
// the transaction the service is bound to.
func (s *UploadAPIService) HandleUpload(ctx context.Context, fileName string, contentType string, file io.ReadSeeker) (commonmodel.ImplResponse, error) {
	if file == nil {
		return newUploadErrorResponse(http.StatusBadRequest, "AASENV-HANDLEUPLOAD-MISSINGFILE", fmt.Errorf("uploaded file is required"))
	}

	normalizedFileName := sanitizeUploadMetadataFileName(fileName)

	signature, err := readUploadSignature(file)
	if err != nil {
		return newUploadErrorResponse(http.StatusInternalServerError, "AASENV-HANDLEUPLOAD-READSIGNATURE", err)
	}

	format, resolvedContentType, err := detectUploadFormat(normalizedFileName, contentType, signature)
	if err != nil {
		return newUploadErrorResponse(http.StatusUnsupportedMediaType, "AASENV-HANDLEUPLOAD-UNSUPPORTEDFORMAT", err)
	}

	switch format {
	case "aasx":
		return s.handleAASXUpload(ctx, normalizedFileName, resolvedContentType, file)
	case "json":
		return s.handleJSONUpload(ctx, normalizedFileName, resolvedContentType, file)
	case "xml":
		return s.handleXMLUpload(ctx, normalizedFileName, resolvedContentType, file)
	default:
		return newUploadErrorResponse(
			http.StatusUnsupportedMediaType,
			"AASENV-HANDLEUPLOAD-UNSUPPORTEDFORMAT",
			fmt.Errorf("unsupported upload format %q", format),
		)
	}
}

func (s *UploadAPIService) handleAASXUpload(ctx context.Context, fileName string, contentType string, file io.ReadSeeker) (commonmodel.ImplResponse, error) {
	if _, err := file.Seek(0, 0); err != nil {
		return newUploadErrorResponse(http.StatusInternalServerError, "AASENV-HANDLEUPLOAD-SEEKAASXFILE", err)
	}

	packaging := aasx.NewPackaging()
	limits := common.AASXLimitsFromContext(ctx)
	packageReader, err := packaging.OpenReadFromStream(file, limits.ReaderOptions()...)
	if err != nil {
		if errors.Is(err, aasx.ErrReaderLimitExceeded) {
			return newUploadErrorResponse(http.StatusRequestEntityTooLarge, "AASENV-HANDLEUPLOAD-AASXLIMIT", common.NewErrPayloadTooLarge(err.Error()))
		}
		return newUploadErrorResponse(http.StatusBadRequest, "AASENV-HANDLEUPLOAD-PARSEAASX", err)
	}
	defer func() {
		_ = packageReader.Close()
	}()

	if err = s.processAASXPackage(ctx, fileName, contentType, packageReader); err != nil {
		return newUploadErrorResponse(uploadProcessingStatus(err), "AASENV-HANDLEUPLOAD-PROCESSAASX", err)
	}

	specs, err := packageReader.Specs()
	if err != nil {
		return newUploadErrorResponse(http.StatusInternalServerError, "AASENV-HANDLEUPLOAD-READAASXSPECS", err)
	}

	supplementaries, err := packageReader.SupplementaryRelationships()
	if err != nil {
		return newUploadErrorResponse(http.StatusInternalServerError, "AASENV-HANDLEUPLOAD-READAASXSUPPLEMENTARIES", err)
	}

	return commonmodel.Response(http.StatusOK, map[string]any{
		"message":               "AASX file parsed successfully",
		"fileName":              fileName,
		"contentType":           contentType,
		"format":                "aasx",
		"specCount":             len(specs),
		"supplementaryRelCount": len(supplementaries),
	}), nil
}

func (s *UploadAPIService) handleJSONUpload(ctx context.Context, fileName string, contentType string, file io.ReadSeeker) (commonmodel.ImplResponse, error) {
	if _, err := file.Seek(0, 0); err != nil {
		return newUploadErrorResponse(http.StatusInternalServerError, "AASENV-HANDLEUPLOAD-SEEKJSONFILE", err)
	}
	var jsonable any
	decoder := json.NewDecoder(file)
	if err := decoder.Decode(&jsonable); err != nil {
		return newUploadErrorResponse(http.StatusBadRequest, "AASENV-HANDLEUPLOAD-PARSEJSON", err)
	}

	environment, err := aasjsonization.EnvironmentFromJsonable(jsonable)
	if err != nil {
		return newUploadErrorResponse(http.StatusBadRequest, "AASENV-HANDLEUPLOAD-PARSEJSONENVIRONMENT", err)
	}

	if err = s.processEnvironment(ctx, fileName, contentType, environment); err != nil {
		return newUploadErrorResponse(uploadProcessingStatus(err), "AASENV-HANDLEUPLOAD-PROCESSJSONENVIRONMENT", err)
	}

	return commonmodel.Response(http.StatusOK, map[string]any{
		"message":                       "JSON file parsed successfully",
		"fileName":                      fileName,
		"contentType":                   contentType,
		"format":                        "json",
		"assetAdministrationShellCount": len(environment.AssetAdministrationShells()),
		"submodelCount":                 len(environment.Submodels()),
		"conceptDescriptionCount":       len(environment.ConceptDescriptions()),
	}), nil
}

func (s *UploadAPIService) handleXMLUpload(ctx context.Context, fileName string, contentType string, file io.ReadSeeker) (commonmodel.ImplResponse, error) {
	if _, err := file.Seek(0, 0); err != nil {
		return newUploadErrorResponse(http.StatusInternalServerError, "AASENV-HANDLEUPLOAD-SEEKXMLFILE", err)
	}
	specContent, err := io.ReadAll(file)
	if err != nil {
		return newUploadErrorResponse(http.StatusInternalServerError, "AASENV-HANDLEUPLOAD-READXMLFILE", err)
	}

	instance, err := parseAASXMLInstance(specContent, fileName)
	if err != nil {
		return newUploadErrorResponse(http.StatusBadRequest, "AASENV-HANDLEUPLOAD-PARSEXML", err)
	}

	environment, ok := instance.(aastypes.IEnvironment)
	if !ok {
		return newUploadErrorResponse(
			http.StatusBadRequest,
			"AASENV-HANDLEUPLOAD-XMLNOTENVIRONMENT",
			fmt.Errorf("uploaded XML root is %T, expected AAS Environment", instance),
		)
	}

	if err = s.processEnvironment(ctx, fileName, contentType, environment); err != nil {
		return newUploadErrorResponse(uploadProcessingStatus(err), "AASENV-HANDLEUPLOAD-PROCESSXMLENVIRONMENT", err)
	}

	return commonmodel.Response(http.StatusOK, map[string]any{
		"message":                       "XML file parsed successfully",
		"fileName":                      fileName,
		"contentType":                   contentType,
		"format":                        "xml",
		"assetAdministrationShellCount": len(environment.AssetAdministrationShells()),
		"submodelCount":                 len(environment.Submodels()),
		"conceptDescriptionCount":       len(environment.ConceptDescriptions()),
	}), nil
}

func (s *UploadAPIService) processAASXPackage(ctx context.Context, fileName string, _ string, packageReader *aasx.PackageRead) error {
	specPart, environment, err := ReadEnvironmentFromAASXSpec(packageReader, fileName)
	if err != nil {
		return err
	}
	binaryContent, err := digestAASXBinaryContent(ctx, packageReader, specPart, environment)
	if err != nil {
		return err
	}

	return s.importEnvironment(ctx, environmentImport{
		environment:   environment,
		binaryContent: binaryContent,
		storePackageParts: func(txCtx context.Context, tx *sql.Tx) error {
			if storeErr := s.storeAASXThumbnail(txCtx, tx, packageReader, specPart, environment); storeErr != nil {
				return storeErr
			}
			return s.uploadSupplementaryFiles(txCtx, tx, packageReader, specPart, environment)
		},
	})
}

func (s *UploadAPIService) processEnvironment(ctx context.Context, _ string, _ string, environment aastypes.IEnvironment) error {
	return s.importEnvironment(ctx, environmentImport{environment: environment})
}

// environmentImport is one uploaded environment, the binary content its
// package parts store, and the step that stores those parts.
type environmentImport struct {
	environment       aastypes.IEnvironment
	binaryContent     []binarycontent.ContentDigest
	storePackageParts func(context.Context, *sql.Tx) error
}

// importEnvironment stores every Concept Description, Submodel and Shell of the
// environment, followed by its package parts, in one transaction. Registry
// descriptor sync, history and event outbox writes share that transaction, so
// a failing step leaves nothing of the upload behind.
func (s *UploadAPIService) importEnvironment(ctx context.Context, upload environmentImport) error {
	if err := s.validateEnvironmentImport(upload.environment); err != nil {
		return err
	}
	ctx = common.WithWriterPostgresReads(ctx)
	err := s.inUploadTransaction(func(tx *sql.Tx) error {
		if err := lockEnvironmentImport(ctx, tx, upload); err != nil {
			return err
		}
		if err := s.storeEnvironmentResources(ctx, tx, upload.environment); err != nil {
			return err
		}
		if upload.storePackageParts == nil {
			return nil
		}
		return upload.storePackageParts(ctx, tx)
	})
	if err != nil {
		return fmt.Errorf("AASENV-PROCESSENV-ROLLEDBACK upload rejected, nothing from the uploaded environment was stored: %w", err)
	}
	return nil
}

func (s *UploadAPIService) inUploadTransaction(fn func(*sql.Tx) error) error {
	if s.transaction != nil {
		return fn(s.transaction)
	}
	return s.persistence.ExecuteInTransaction("AASENV-PROCESSENV-STARTTX", "AASENV-PROCESSENV-COMMITTX", fn)
}

// boundTo returns a copy of the service whose imports write into tx instead of
// opening their own transaction.
func (s *UploadAPIService) boundTo(tx *sql.Tx) *UploadAPIService {
	bound := *s
	bound.transaction = tx
	return &bound
}

func (s *UploadAPIService) validateEnvironmentImport(environment aastypes.IEnvironment) error {
	if s == nil || s.persistence == nil {
		return common.NewErrBadRequest("AASENV-PROCESSENV-NILPERSISTENCE persistence is required for environment import")
	}
	if s.persistence.ConceptDescriptionRepository == nil || s.persistence.SubmodelRepository == nil || s.persistence.AASRepository == nil {
		return common.NewErrBadRequest("AASENV-PROCESSENV-NILBACKEND one or more repository backends are not initialized")
	}
	if environment == nil {
		return common.NewErrBadRequest("AASENV-PROCESSENV-NILENV environment is required")
	}
	return nil
}

func (s *UploadAPIService) storeEnvironmentResources(ctx context.Context, tx *sql.Tx, environment aastypes.IEnvironment) error {
	if err := s.storeEnvironmentConceptDescriptions(ctx, tx, environment.ConceptDescriptions()); err != nil {
		return err
	}
	if err := s.storeEnvironmentSubmodels(ctx, tx, environment.Submodels()); err != nil {
		return err
	}
	return s.storeEnvironmentShells(ctx, tx, environment.AssetAdministrationShells())
}

// lockEnvironmentImport takes every lock the upload needs before its first
// write and in one global order: the resource locks of the Concept
// Descriptions, Submodels and Shells, each sorted, then the content locks of
// the binaries, sorted by digest. Concurrent uploads therefore wait for each
// other instead of acquiring locks in opposite orders and deadlocking.
func lockEnvironmentImport(ctx context.Context, tx *sql.Tx, upload environmentImport) error {
	environment := upload.environment
	if err := history.AcquireMutationLocksTx(ctx, tx, history.TableConcept, identifiablesIDs(environment.ConceptDescriptions())); err != nil {
		return fmt.Errorf("AASENV-PROCESSENV-LOCKCD failed to lock the uploaded Concept Descriptions: %w", err)
	}
	if err := history.AcquireMutationLocksTx(ctx, tx, history.TableSubmodel, identifiablesIDs(environment.Submodels())); err != nil {
		return fmt.Errorf("AASENV-PROCESSENV-LOCKSM failed to lock the uploaded Submodels: %w", err)
	}
	if err := history.AcquireMutationLocksTx(ctx, tx, history.TableAAS, identifiablesIDs(environment.AssetAdministrationShells())); err != nil {
		return fmt.Errorf("AASENV-PROCESSENV-LOCKAAS failed to lock the uploaded Asset Administration Shells: %w", err)
	}
	if err := binarycontent.LockContentDigestsTx(ctx, tx, upload.binaryContent); err != nil {
		return fmt.Errorf("AASENV-PROCESSENV-LOCKBINARY failed to lock the uploaded binary content: %w", err)
	}
	return nil
}

func identifiablesIDs[T interface{ ID() string }](identifiables []T) []string {
	identifiers := make([]string, 0, len(identifiables))
	for _, identifiable := range identifiables {
		identifiers = append(identifiers, identifiable.ID())
	}
	return identifiers
}

func (s *UploadAPIService) storeEnvironmentConceptDescriptions(ctx context.Context, tx *sql.Tx, descriptions []aastypes.IConceptDescription) error {
	ctx, err := auth.EnvironmentResourceContext(ctx, auth.SemanticResourceCD)
	if err != nil {
		return err
	}
	for _, description := range descriptions {
		if _, err := s.persistence.ConceptDescriptionRepository.PutConceptDescriptionInTransaction(ctx, tx, description.ID(), description); err != nil {
			return fmt.Errorf("AASENV-PROCESSENV-PUTCD failed to store Concept Description %q: %w", description.ID(), err)
		}
	}
	return nil
}

func (s *UploadAPIService) storeEnvironmentSubmodels(ctx context.Context, tx *sql.Tx, submodels []aastypes.ISubmodel) error {
	ctx, err := auth.EnvironmentResourceContext(ctx, auth.SemanticResourceSM)
	if err != nil {
		return err
	}
	for _, submodel := range submodels {
		if err := s.storeEnvironmentSubmodel(ctx, tx, submodel); err != nil {
			return fmt.Errorf("AASENV-PROCESSENV-PUTSM failed to store Submodel %q: %w", submodel.ID(), err)
		}
	}
	return nil
}

func (s *UploadAPIService) storeEnvironmentSubmodel(ctx context.Context, tx *sql.Tx, submodel aastypes.ISubmodel) error {
	if s.submodelRepositoryService == nil {
		_, err := s.persistence.SubmodelRepository.PutSubmodelInTransaction(ctx, tx, submodel.ID(), submodel)
		return err
	}
	return s.submodelRepositoryService.UpsertSubmodelInTransaction(ctx, tx, submodel)
}

func (s *UploadAPIService) storeEnvironmentShells(ctx context.Context, tx *sql.Tx, shells []aastypes.IAssetAdministrationShell) error {
	ctx, err := auth.EnvironmentResourceContext(ctx, auth.SemanticResourceAAS)
	if err != nil {
		return err
	}
	for _, shell := range shells {
		if err := s.storeEnvironmentShell(ctx, tx, shell); err != nil {
			return fmt.Errorf("AASENV-PROCESSENV-PUTAAS failed to store Asset Administration Shell %q: %w", shell.ID(), err)
		}
	}
	return nil
}

func (s *UploadAPIService) storeEnvironmentShell(ctx context.Context, tx *sql.Tx, shell aastypes.IAssetAdministrationShell) error {
	if s.aasRepositoryService == nil {
		_, err := s.persistence.AASRepository.PutAssetAdministrationShellByIDInTransaction(ctx, tx, shell.ID(), shell)
		return err
	}
	return s.aasRepositoryService.UpsertAssetAdministrationShellInTransaction(ctx, tx, shell)
}

// ReadEnvironmentFromAASXSpec parses the single supported XML or JSON spec in
// an AASX package using the same compatibility handling as environment upload.
//
// Parameters:
//   - packageReader: Open AASX package reader owned by the caller.
//   - uploadSource: Source label used in compatibility diagnostics.
//
// Returns the selected spec part, parsed AAS environment, and any package or
// validation error.
func ReadEnvironmentFromAASXSpec(packageReader *aasx.PackageRead, uploadSource string) (*aasx.Part, aastypes.IEnvironment, error) {
	if packageReader == nil {
		return nil, nil, common.NewErrBadRequest("AASENV-PARSEAASX-NILREADER package reader is required")
	}

	specs, err := packageReader.Specs()
	if err != nil {
		return nil, nil, fmt.Errorf("AASENV-PARSEAASX-READSPECS failed to read AASX specs: %w", err)
	}

	if len(specs) == 0 {
		return nil, nil, common.NewErrBadRequest("AASENV-PARSEAASX-NOSPECS no AASX spec parts found")
	}

	supportedSpecs := make([]*aasx.Part, 0, len(specs))
	for _, spec := range specs {
		if isLikelyJSONSpec(spec) || isLikelyXMLSpec(spec) {
			supportedSpecs = append(supportedSpecs, spec)
		}
	}

	if len(supportedSpecs) == 0 {
		return nil, nil, common.NewErrBadRequest("AASENV-PARSEAASX-NOSUPPORTEDSPEC no supported AASX spec found, expected XML or JSON content spec")
	}
	if len(supportedSpecs) > 1 {
		uris := make([]string, 0, len(supportedSpecs))
		for _, spec := range supportedSpecs {
			uris = append(uris, normalizePartURI(spec.URI))
		}
		return nil, nil, common.NewErrBadRequest("AASENV-PARSEAASX-MULTISPEC multiple supported AASX specs found: " + strings.Join(uris, ", "))
	}

	specPart := supportedSpecs[0]

	if isLikelyJSONSpec(specPart) {
		stream, streamErr := specPart.Stream()
		if streamErr != nil {
			return nil, nil, fmt.Errorf("AASENV-PARSEAASX-OPENJSONSPEC failed to open AASX spec content: %w", streamErr)
		}
		environment, parseErr := parseAASJSONEnvironmentReader(stream)
		closeErr := stream.Close()
		if parseErr != nil {
			return nil, nil, common.NewErrBadRequest(
				fmt.Sprintf("AASENV-PARSEAASX-UNMARSHALJSON failed to parse JSON spec: %v", parseErr),
			)
		}
		if closeErr != nil {
			return nil, nil, fmt.Errorf("AASENV-PARSEAASX-CLOSEJSONSPEC failed to close AASX spec content: %w", closeErr)
		}
		return specPart, environment, nil
	}

	stream, streamErr := specPart.Stream()
	if streamErr != nil {
		return nil, nil, fmt.Errorf("AASENV-PARSEAASX-OPENXMLSPEC failed to open AASX spec content: %w", streamErr)
	}
	instance, err := aasxmlization.Unmarshal(xml.NewDecoder(stream))
	closeErr := stream.Close()
	if err == nil && closeErr != nil {
		return nil, nil, fmt.Errorf("AASENV-PARSEAASX-CLOSEXMLSPEC failed to close AASX spec content: %w", closeErr)
	}
	if err != nil {
		specContent, readErr := specPart.ReadAllBytes()
		if readErr != nil {
			return nil, nil, fmt.Errorf("AASENV-PARSEAASX-READXMLFALLBACK failed to read AASX spec content: %w", readErr)
		}
		instance, err = parseAASXMLInstance(specContent, buildAASXSpecSourceLabel(uploadSource, specPart))
	}
	if err != nil {
		return nil, nil, common.NewErrBadRequest(
			fmt.Sprintf("AASENV-PARSEAASX-UNMARSHALXML failed to parse XML spec: %v", err),
		)
	}
	environment, ok := instance.(aastypes.IEnvironment)
	if !ok {
		return nil, nil, common.NewErrBadRequest(fmt.Sprintf("AASENV-PARSEAASX-XMLNOTENV XML spec root is %T, expected AAS Environment", instance))
	}

	return specPart, environment, nil
}

func isLikelyJSONSpec(specPart *aasx.Part) bool {
	if specPart == nil {
		return false
	}
	normalized := strings.ToLower(normalizePartURI(specPart.URI))
	contentType := strings.ToLower(strings.TrimSpace(specPart.ContentType))
	return strings.HasSuffix(normalized, ".json") || strings.Contains(contentType, "json")
}

func isLikelyXMLSpec(specPart *aasx.Part) bool {
	if specPart == nil {
		return false
	}
	normalized := strings.ToLower(normalizePartURI(specPart.URI))
	contentType := strings.ToLower(strings.TrimSpace(specPart.ContentType))
	return strings.HasSuffix(normalized, ".xml") || strings.Contains(contentType, "xml")
}

func parseAASJSONEnvironmentReader(reader io.Reader) (aastypes.IEnvironment, error) {
	var jsonable any
	if err := json.NewDecoder(reader).Decode(&jsonable); err != nil {
		return nil, err
	}

	environment, err := aasjsonization.EnvironmentFromJsonable(jsonable)
	if err != nil {
		return nil, err
	}
	return environment, nil
}

func parseAASXMLInstance(specContent []byte, sourceLabel string) (aastypes.IClass, error) {
	instance, err := aasxmlization.Unmarshal(xml.NewDecoder(bytes.NewReader(specContent)))
	if err == nil {
		return instance, nil
	}

	normalized := normalizeXMLSpecContent(specContent)
	if len(normalized) == 0 {
		normalized = specContent
	}

	retryFailures := make([]string, 0, 4)
	retried, retryErr := aasxmlization.Unmarshal(xml.NewDecoder(bytes.NewReader(normalized)))
	if retryErr == nil {
		return retried, nil
	}
	retryFailures = append(retryFailures, fmt.Sprintf("retry after normalization failed: %v", retryErr))

	contentToSanitize := normalized
	adaptedContent, _, namespaceAdapted, adaptationErr := adaptLegacyAASNamespace(normalized)
	if adaptationErr != nil {
		retryFailures = append(retryFailures, fmt.Sprintf("namespace adaptation failed: %v", adaptationErr))
	} else if namespaceAdapted {
		sanitizedSourceLabel := sanitizeUploadLogValue(strings.TrimSpace(sourceLabel))
		if sanitizedSourceLabel == "" {
			sanitizedSourceLabel = "unknown"
		}
		// #nosec G706 -- source label is sanitized to strip CR/LF before logging.
		slog.Warn("legacy AAS namespace adapted for backward compatibility", "error.code", "AASENV-PARSEAASX-NAMESPACEADAPTED", "source", sanitizedSourceLabel, "namespace", sanitizeUploadLogValue(currentAASNamespace))
		adaptedRetried, adaptedRetryErr := aasxmlization.Unmarshal(xml.NewDecoder(bytes.NewReader(adaptedContent)))
		if adaptedRetryErr == nil {
			return adaptedRetried, nil
		}
		retryFailures = append(retryFailures, fmt.Sprintf("retry after namespace adaptation failed: %v", adaptedRetryErr))
		contentToSanitize = adaptedContent
	}

	sanitized, sanitizeErr := sanitizeXMLRootAttributes(contentToSanitize)
	if sanitizeErr == nil && len(sanitized) > 0 {
		sanitizedRetried, sanitizedRetryErr := aasxmlization.Unmarshal(xml.NewDecoder(bytes.NewReader(sanitized)))
		if sanitizedRetryErr == nil {
			return sanitizedRetried, nil
		}
		retryFailures = append(retryFailures, fmt.Sprintf("retry after root attribute sanitization failed: %v", sanitizedRetryErr))
		return nil, fmt.Errorf("%w (%s)", err, strings.Join(retryFailures, "; "))
	}

	if sanitizeErr != nil {
		retryFailures = append(retryFailures, fmt.Sprintf("root attribute sanitization failed: %v", sanitizeErr))
		return nil, fmt.Errorf("%w (%s)", err, strings.Join(retryFailures, "; "))
	}

	return nil, fmt.Errorf("%w (%s)", err, strings.Join(retryFailures, "; "))
}

func buildAASXSpecSourceLabel(uploadSource string, specPart *aasx.Part) string {
	trimmedUploadSource := strings.TrimSpace(uploadSource)
	specURI := ""
	if specPart != nil {
		specURI = normalizePartURI(specPart.URI)
	}

	if trimmedUploadSource == "" && specURI == "" {
		return ""
	}
	if trimmedUploadSource == "" {
		return specURI
	}
	if specURI == "" {
		return trimmedUploadSource
	}

	return trimmedUploadSource + ":" + specURI
}

func adaptLegacyAASNamespace(content []byte) ([]byte, string, bool, error) {
	decoder := xml.NewDecoder(bytes.NewReader(content))
	var output bytes.Buffer
	encoder := xml.NewEncoder(&output)

	rootProcessed := false
	adaptedNamespace := ""
	namespaceAdapted := false

	for {
		token, err := decoder.Token()
		if err != nil {
			if errors.Is(err, io.EOF) {
				break
			}
			return nil, "", false, err
		}

		startElement, isStart := token.(xml.StartElement)
		if isStart {
			if !rootProcessed {
				adaptedNamespace, namespaceAdapted = rewriteRootAASNamespace(&startElement)
				rootProcessed = true
			}

			if namespaceAdapted {
				startElement = rewriteStartElementNamespace(startElement, adaptedNamespace)
			}
			token = startElement
		}

		if endElement, isEnd := token.(xml.EndElement); isEnd && namespaceAdapted {
			if strings.TrimSpace(endElement.Name.Space) == adaptedNamespace {
				endElement.Name.Space = currentAASNamespace
				token = endElement
			}
		}

		if err := encoder.EncodeToken(token); err != nil {
			return nil, "", false, err
		}
	}

	if err := encoder.Flush(); err != nil {
		return nil, "", false, err
	}

	return output.Bytes(), adaptedNamespace, namespaceAdapted, nil
}

func rewriteStartElementNamespace(start xml.StartElement, sourceNamespace string) xml.StartElement {
	if strings.TrimSpace(start.Name.Space) == sourceNamespace {
		start.Name.Space = currentAASNamespace
	}

	for i, attribute := range start.Attr {
		if isNamespaceAttribute(attribute) && strings.TrimSpace(attribute.Value) == sourceNamespace {
			start.Attr[i].Value = currentAASNamespace
		}
	}

	return start
}

func rewriteRootAASNamespace(start *xml.StartElement) (string, bool) {
	if start == nil {
		return "", false
	}

	namespace := strings.TrimSpace(start.Name.Space)
	if namespace == "" {
		namespace = findDefaultNamespace(*start)
	}

	if !isBackwardCompatibleAASNamespace(namespace) {
		return "", false
	}

	start.Name.Space = currentAASNamespace
	for i, attribute := range start.Attr {
		if isNamespaceAttribute(attribute) && strings.TrimSpace(attribute.Value) == namespace {
			start.Attr[i].Value = currentAASNamespace
		}
	}

	return namespace, true
}

func findDefaultNamespace(start xml.StartElement) string {
	for _, attribute := range start.Attr {
		if attribute.Name.Space == "" && attribute.Name.Local == "xmlns" {
			return strings.TrimSpace(attribute.Value)
		}
	}
	return ""
}

func isNamespaceAttribute(attribute xml.Attr) bool {
	return attribute.Name.Space == "xmlns" || (attribute.Name.Space == "" && attribute.Name.Local == "xmlns")
}

func isBackwardCompatibleAASNamespace(namespace string) bool {
	namespace = strings.TrimSpace(namespace)
	if namespace == "" {
		return false
	}

	currentMajor, currentMinor, currentOk := parseAASNamespaceVersion(currentAASNamespace)
	candidateMajor, candidateMinor, candidateOk := parseAASNamespaceVersion(namespace)
	if !currentOk || !candidateOk {
		return false
	}
	if candidateMajor != currentMajor {
		return false
	}

	return candidateMinor <= currentMinor
}

func parseAASNamespaceVersion(namespace string) (int, int, bool) {
	trimmedNamespace := strings.TrimSpace(namespace)
	if !strings.HasPrefix(trimmedNamespace, aasNamespacePrefix) {
		return 0, 0, false
	}

	versionPath := strings.TrimPrefix(trimmedNamespace, aasNamespacePrefix)
	versionPath = strings.TrimSuffix(versionPath, "/")
	parts := strings.Split(versionPath, "/")
	if len(parts) != 2 {
		return 0, 0, false
	}

	major, majorErr := strconv.Atoi(parts[0])
	if majorErr != nil {
		return 0, 0, false
	}
	minor, minorErr := strconv.Atoi(parts[1])
	if minorErr != nil {
		return 0, 0, false
	}

	return major, minor, true
}

func sanitizeXMLRootAttributes(content []byte) ([]byte, error) {
	decoder := xml.NewDecoder(bytes.NewReader(content))
	var output bytes.Buffer
	encoder := xml.NewEncoder(&output)
	rootProcessed := false

	for {
		token, err := decoder.Token()
		if err != nil {
			if errors.Is(err, io.EOF) {
				break
			}
			return nil, err
		}

		start, ok := token.(xml.StartElement)
		if !ok || rootProcessed {
			if encodeErr := encoder.EncodeToken(token); encodeErr != nil {
				return nil, encodeErr
			}
			continue
		}

		filtered := make([]xml.Attr, 0, len(start.Attr))
		for _, attribute := range start.Attr {
			if attribute.Name.Space == "xmlns" || (attribute.Name.Space == "" && attribute.Name.Local == "xmlns") {
				filtered = append(filtered, attribute)
			}
		}
		start.Attr = filtered
		if encodeErr := encoder.EncodeToken(start); encodeErr != nil {
			return nil, encodeErr
		}
		rootProcessed = true
	}

	if err := encoder.Flush(); err != nil {
		return nil, err
	}
	return output.Bytes(), nil
}

func normalizeXMLSpecContent(specContent []byte) []byte {
	content := bytes.TrimSpace(specContent)
	content = bytes.TrimPrefix(content, []byte{0xEF, 0xBB, 0xBF})
	content = bytes.TrimSpace(content)

	start := firstXMLStartElementIndex(content)
	if start < 0 || start >= len(content) {
		return content
	}

	return bytes.TrimSpace(content[start:])
}

func firstXMLStartElementIndex(content []byte) int {
	index := 0
	for index < len(content) {
		lt := bytes.IndexByte(content[index:], '<')
		if lt < 0 {
			return -1
		}
		candidate := index + lt
		if candidate+1 >= len(content) {
			return -1
		}

		next := content[candidate+1]
		if next != '?' && next != '!' {
			return candidate
		}

		if next == '?' {
			endPI := bytes.Index(content[candidate+2:], []byte("?>"))
			if endPI < 0 {
				return -1
			}
			index = candidate + 2 + endPI + 2
			continue
		}

		if bytes.HasPrefix(content[candidate+2:], []byte("--")) {
			endComment := bytes.Index(content[candidate+4:], []byte("-->"))
			if endComment < 0 {
				return -1
			}
			index = candidate + 4 + endComment + 3
			continue
		}

		endDecl := bytes.IndexByte(content[candidate+2:], '>')
		if endDecl < 0 {
			return -1
		}
		index = candidate + 2 + endDecl + 1
	}
	return -1
}

type limitedAASXPartStream struct {
	io.Reader
	io.Closer
}

func openLimitedAASXPart(part *aasx.Part, maximum uint64, label string) (io.ReadCloser, error) {
	if part == nil {
		return nil, common.NewErrBadRequest("AASENV-AASXPART-NIL package part is required")
	}
	stream, err := part.Stream()
	if err != nil {
		return nil, err
	}
	return &limitedAASXPartStream{
		Reader: common.NewPayloadLimitReader(stream, maximum, label),
		Closer: stream,
	}, nil
}

func (s *UploadAPIService) uploadSupplementaryFiles(
	ctx context.Context,
	tx *sql.Tx,
	packageReader *aasx.PackageRead,
	specPart *aasx.Part,
	environment aastypes.IEnvironment,
) error {
	ctx, bindErr := auth.EnvironmentResourceContext(ctx, auth.SemanticResourceSM)
	if bindErr != nil {
		return bindErr
	}
	if s == nil || s.persistence == nil || s.persistence.SubmodelRepository == nil {
		return common.NewErrBadRequest("AASENV-UPLDSUPPL-NILSMREPO submodel repository backend is required")
	}

	supplementaries, err := specSupplementaryRelationships(packageReader, specPart)
	if err != nil {
		return err
	}

	fileLocations := CollectAASXFileElementLocations(environment)
	importedLocations := make(map[int]struct{}, len(fileLocations))

	for _, relationship := range supplementaries {
		matched, uploadErr := s.uploadSupplementaryRelationship(ctx, tx, relationship, fileLocations, importedLocations)
		if uploadErr != nil {
			return uploadErr
		}
		if !matched {
			supplementaryURIForLog := sanitizeUploadLogValue(normalizePartURI(relationship.Supplementary.URI))
			// #nosec G706 -- value is sanitized to strip CR/LF control characters before logging.
			slog.WarnContext(ctx, "no File element path matched supplementary attachment", "error.code", "AASENV-UPLDSUPPL-NOMATCH", "supplementary_uri", supplementaryURIForLog)
		}
	}

	warnFileReferencesWithoutSupplementaryRelationship(ctx, packageReader, specPart.URI, fileLocations, importedLocations)
	slog.InfoContext(ctx, "supplementary file attachments written to the upload transaction", "attachment_count", len(importedLocations))
	return nil
}

func specSupplementaryRelationships(packageReader *aasx.PackageRead, specPart *aasx.Part) ([]*aasx.SupplementaryRelationship, error) {
	relationships, err := packageReader.SupplementaryRelationships()
	if err != nil {
		return nil, fmt.Errorf("AASENV-UPLDSUPPL-READREL failed to read supplementary relationships: %w", err)
	}
	specURI := normalizePartURI(specPart.URI)
	ofSpec := make([]*aasx.SupplementaryRelationship, 0, len(relationships))
	for _, relationship := range relationships {
		if relationship.Spec != nil && relationship.Supplementary != nil && normalizePartURI(relationship.Spec.URI) == specURI {
			ofSpec = append(ofSpec, relationship)
		}
	}
	return ofSpec, nil
}

func (s *UploadAPIService) uploadSupplementaryRelationship(
	ctx context.Context,
	tx *sql.Tx,
	relationship *aasx.SupplementaryRelationship,
	fileLocations []AASXFileElementLocation,
	importedLocations map[int]struct{},
) (bool, error) {
	matched := false
	for index, location := range fileLocations {
		if !matchesSupplementaryTarget(location.FileValue, relationship.Spec.URI, relationship.Supplementary.URI) {
			continue
		}
		if err := s.uploadSupplementaryToFileElement(ctx, tx, relationship.Supplementary, location); err != nil {
			return false, err
		}
		matched = true
		importedLocations[index] = struct{}{}
	}
	return matched, nil
}

func (s *UploadAPIService) uploadSupplementaryToFileElement(ctx context.Context, tx *sql.Tx, supplementary *aasx.Part, location AASXFileElementLocation) error {
	uploadName := supplementaryUploadName(supplementary, location)
	partStream, streamErr := openLimitedAASXPart(supplementary, common.AASXLimitsFromContext(ctx).MaxPartExpandedSizeBytes, "supplementary")
	if streamErr != nil {
		return fmt.Errorf("AASENV-UPLDSUPPL-OPENSTREAM failed to open supplementary '%s': %w", normalizePartURI(supplementary.URI), streamErr)
	}
	fileElementContentType := ""
	if contentType := location.FileElement.ContentType(); contentType != nil {
		fileElementContentType = *contentType
	}
	uploadErr := s.persistence.SubmodelRepository.UploadFileAttachmentReaderWithHistoryInTransaction(
		ctx,
		tx,
		location.SubmodelID,
		location.IDShortPath,
		partStream,
		uploadName,
		fileElementContentType,
		supplementary.ContentType,
	)
	closeErr := partStream.Close()
	if uploadErr != nil {
		return fmt.Errorf(
			"AASENV-UPLDSUPPL-UPLOAD failed to store supplementary file '%s' for Submodel '%s' at path '%s': %w",
			uploadName,
			location.SubmodelID,
			location.IDShortPath,
			uploadErr,
		)
	}
	if closeErr != nil {
		return fmt.Errorf("AASENV-UPLDSUPPL-CLOSESTREAM failed to close supplementary '%s': %w", uploadName, closeErr)
	}
	return nil
}

func supplementaryUploadName(supplementary *aasx.Part, location AASXFileElementLocation) string {
	uploadName := filepath.Base(normalizePartURI(supplementary.URI))
	if uploadName == "." || uploadName == "/" || uploadName == "" {
		uploadName = filepath.Base(strings.TrimSpace(location.FileValue))
	}
	if uploadName == "." || uploadName == "/" || uploadName == "" {
		uploadName = "supplementary.bin"
	}
	return uploadName
}

// warnFileReferencesWithoutSupplementaryRelationship reports File elements
// whose value names a package part that the specification does not relate as
// aas-suppl (IDTA-01005). Such parts are not imported as attachments.
func warnFileReferencesWithoutSupplementaryRelationship(
	ctx context.Context,
	packageReader *aasx.PackageRead,
	specURI *url.URL,
	fileLocations []AASXFileElementLocation,
	importedLocations map[int]struct{},
) {
	for index, location := range fileLocations {
		if _, imported := importedLocations[index]; imported {
			continue
		}
		partURI := unrelatedPackagePartURI(packageReader, location.FileValue, specURI)
		if partURI == "" {
			continue
		}
		// #nosec G706 -- values are sanitized to strip CR/LF control characters before logging.
		slog.WarnContext(
			ctx,
			"File element references a package part without an aas-suppl relationship; attachment not imported",
			"error.code", "AASENV-UPLDSUPPL-NOSUPPLREL",
			"submodel_id", sanitizeUploadLogValue(location.SubmodelID),
			"id_short_path", sanitizeUploadLogValue(location.IDShortPath),
			"part_uri", sanitizeUploadLogValue(partURI),
		)
	}
}

func unrelatedPackagePartURI(packageReader *aasx.PackageRead, fileValue string, specURI *url.URL) string {
	if packageReader == nil || IsExternalAASXReference(fileValue) {
		return ""
	}
	partURI := resolveReferenceAgainstSpec(strings.TrimSpace(fileValue), specURI)
	if partURI == "" {
		return ""
	}
	parsedPartURI, err := url.Parse(partURI)
	if err != nil {
		return ""
	}
	part, err := packageReader.FindPart(parsedPartURI)
	if err != nil || part == nil {
		return ""
	}
	return partURI
}

func (s *UploadAPIService) storeAASXThumbnail(ctx context.Context, tx *sql.Tx, packageReader *aasx.PackageRead, specPart *aasx.Part, environment aastypes.IEnvironment) error {
	ctx, bindErr := auth.EnvironmentResourceContext(ctx, auth.SemanticResourceAAS)
	if bindErr != nil {
		return bindErr
	}
	if s == nil || s.persistence == nil || s.persistence.AASRepository == nil {
		return common.NewErrBadRequest("AASENV-UPLDTHUMB-NILAASREPO AAS repository backend is required")
	}
	if packageReader == nil {
		return common.NewErrBadRequest("AASENV-UPLDTHUMB-NILREADER package reader is required")
	}
	if environment == nil {
		return nil
	}

	thumbnailPartsByURI, packageThumbnail, err := resolveAASXThumbnailParts(packageReader, specPart)
	if err != nil {
		return err
	}

	specURI := (*url.URL)(nil)
	if specPart != nil {
		specURI = specPart.URI
	}

	uploaded := 0
	for _, aas := range environment.AssetAdministrationShells() {
		thumbnailPart, resolvedThumbnailURI := resolveAASXThumbnailPartForAAS(aas, packageReader, specURI, thumbnailPartsByURI, packageThumbnail)
		if thumbnailPart == nil {
			continue
		}
		if err = s.storeAASXThumbnailForShell(ctx, tx, aas, thumbnailPart, resolvedThumbnailURI); err != nil {
			return err
		}
		uploaded++
	}

	slog.InfoContext(ctx, "AAS thumbnails written to the upload transaction", "aas_count", uploaded)
	return nil
}

func (s *UploadAPIService) storeAASXThumbnailForShell(
	ctx context.Context,
	tx *sql.Tx,
	aas aastypes.IAssetAdministrationShell,
	thumbnailPart *aasx.Part,
	resolvedThumbnailURI string,
) error {
	thumbnailName := filepath.Base(resolvedThumbnailURI)
	if strings.TrimSpace(thumbnailName) == "" || thumbnailName == "." || thumbnailName == "/" {
		thumbnailName = "thumbnail.bin"
	}

	partStream, err := openLimitedAASXPart(thumbnailPart, common.AASXLimitsFromContext(ctx).MaxThumbnailSizeBytes, "thumbnail")
	if err != nil {
		return fmt.Errorf("AASENV-UPLDTHUMB-OPENSTREAM failed to open thumbnail for Asset Administration Shell '%s': %w", aas.ID(), err)
	}
	uploadErr := s.persistence.AASRepository.PutThumbnailByAASIDReaderInTransaction(ctx, tx, aas.ID(), partStream, binarycontent.UploadDeclaration{
		FileName:     thumbnailName,
		ContentTypes: []string{defaultThumbnailContentType(aas), thumbnailPart.ContentType},
	})
	closeErr := partStream.Close()
	if uploadErr != nil {
		return fmt.Errorf("AASENV-UPLDTHUMB-UPLOAD failed to store thumbnail for Asset Administration Shell '%s': %w", aas.ID(), uploadErr)
	}
	if closeErr != nil {
		return fmt.Errorf("AASENV-UPLDTHUMB-CLOSESTREAM failed to close thumbnail for Asset Administration Shell '%s': %w", aas.ID(), closeErr)
	}
	return nil
}

func defaultThumbnailContentType(aas aastypes.IAssetAdministrationShell) string {
	if contentType := aas.AssetInformation().DefaultThumbnail().ContentType(); contentType != nil {
		return *contentType
	}
	return ""
}

// digestAASXBinaryContent validates the size limits of the thumbnails and
// supplementary files the upload will store and returns the digests of their
// content, so the import can lock that content in a stable order.
func digestAASXBinaryContent(ctx context.Context, packageReader *aasx.PackageRead, specPart *aasx.Part, environment aastypes.IEnvironment) ([]binarycontent.ContentDigest, error) {
	thumbnails, err := digestAASXThumbnails(ctx, packageReader, specPart, environment)
	if err != nil {
		return nil, err
	}
	supplementaries, err := digestAASXSupplementaries(ctx, packageReader, specPart, environment)
	if err != nil {
		return nil, err
	}
	return append(thumbnails, supplementaries...), nil
}

func digestAASXThumbnails(ctx context.Context, packageReader *aasx.PackageRead, specPart *aasx.Part, environment aastypes.IEnvironment) ([]binarycontent.ContentDigest, error) {
	if environment == nil {
		return nil, nil
	}
	thumbnailPartsByURI, packageThumbnail, err := resolveAASXThumbnailParts(packageReader, specPart)
	if err != nil {
		return nil, err
	}

	var specURI *url.URL
	if specPart != nil {
		specURI = specPart.URI
	}
	digested := make(map[string]struct{})
	digests := make([]binarycontent.ContentDigest, 0)
	maximum := common.AASXLimitsFromContext(ctx).MaxThumbnailSizeBytes
	for _, aas := range environment.AssetAdministrationShells() {
		thumbnailPart, thumbnailURI := resolveAASXThumbnailPartForAAS(aas, packageReader, specURI, thumbnailPartsByURI, packageThumbnail)
		if thumbnailPart == nil {
			continue
		}
		key := normalizePartURI(thumbnailPart.URI)
		if key == "" {
			key = thumbnailURI
		}
		if _, exists := digested[key]; exists {
			continue
		}
		digest, digestErr := digestAASXPart(thumbnailPart, maximum, "thumbnail")
		if digestErr != nil {
			return nil, fmt.Errorf("AASENV-VALIDATETHUMB-LIMIT thumbnail %q for AAS %q is invalid: %w", thumbnailURI, aas.ID(), digestErr)
		}
		digested[key] = struct{}{}
		digests = append(digests, digest)
	}
	return digests, nil
}

func digestAASXSupplementaries(ctx context.Context, packageReader *aasx.PackageRead, specPart *aasx.Part, environment aastypes.IEnvironment) ([]binarycontent.ContentDigest, error) {
	if environment == nil || specPart == nil {
		return nil, nil
	}
	relationships, err := specSupplementaryRelationships(packageReader, specPart)
	if err != nil {
		return nil, err
	}
	fileLocations := CollectAASXFileElementLocations(environment)
	digested := make(map[string]struct{})
	digests := make([]binarycontent.ContentDigest, 0)
	maximum := common.AASXLimitsFromContext(ctx).MaxPartExpandedSizeBytes
	for _, relationship := range relationships {
		key := normalizePartURI(relationship.Supplementary.URI)
		if _, exists := digested[key]; exists || !matchesAnyFileLocation(relationship, fileLocations) {
			continue
		}
		digest, digestErr := digestAASXPart(relationship.Supplementary, maximum, "supplementary")
		if digestErr != nil {
			return nil, fmt.Errorf("AASENV-DIGESTSUPPL-READ failed to read supplementary file '%s': %w", key, digestErr)
		}
		digested[key] = struct{}{}
		digests = append(digests, digest)
	}
	return digests, nil
}

func matchesAnyFileLocation(relationship *aasx.SupplementaryRelationship, fileLocations []AASXFileElementLocation) bool {
	for _, location := range fileLocations {
		if matchesSupplementaryTarget(location.FileValue, relationship.Spec.URI, relationship.Supplementary.URI) {
			return true
		}
	}
	return false
}

func digestAASXPart(part *aasx.Part, maximum uint64, label string) (binarycontent.ContentDigest, error) {
	stream, err := openLimitedAASXPart(part, maximum, label)
	if err != nil {
		return binarycontent.ContentDigest{}, err
	}
	digest, digestErr := binarycontent.DigestContent(stream)
	closeErr := stream.Close()
	if digestErr != nil {
		return binarycontent.ContentDigest{}, digestErr
	}
	if closeErr != nil {
		return binarycontent.ContentDigest{}, common.NewInternalServerError("AASENV-DIGESTPART-CLOSESTREAM " + closeErr.Error())
	}
	return digest, nil
}

func resolveAASXThumbnailParts(packageReader *aasx.PackageRead, specPart *aasx.Part) (map[string]*aasx.Part, *aasx.Part, error) {
	supplementaries, err := packageReader.SupplementaryRelationships()
	if err != nil {
		return nil, nil, fmt.Errorf("AASENV-UPLDTHUMB-READRELATIONSHIPS failed to read AASX supplementary relationships: %w", err)
	}

	resolvedSpecURI := ""
	if specPart != nil {
		resolvedSpecURI = normalizePartURI(specPart.URI)
	}

	thumbnailPartsByURI := make(map[string]*aasx.Part)
	for _, relationship := range supplementaries {
		if relationship.Spec == nil || relationship.Supplementary == nil {
			continue
		}

		if resolvedSpecURI != "" && normalizePartURI(relationship.Spec.URI) != resolvedSpecURI {
			continue
		}

		supplementaryURI := normalizePartURI(relationship.Supplementary.URI)
		if supplementaryURI == "" {
			continue
		}

		if _, exists := thumbnailPartsByURI[supplementaryURI]; exists {
			continue
		}

		thumbnailPartsByURI[supplementaryURI] = relationship.Supplementary
	}

	packageThumbnail, err := packageReader.Thumbnail()
	if err != nil {
		return nil, nil, fmt.Errorf("AASENV-UPLDTHUMB-READTHUMBNAIL failed to read AASX package thumbnail: %w", err)
	}

	return thumbnailPartsByURI, packageThumbnail, nil
}

func resolveAASXThumbnailPartForAAS(
	aas aastypes.IAssetAdministrationShell,
	packageReader *aasx.PackageRead,
	specURI *url.URL,
	thumbnailPartsByURI map[string]*aasx.Part,
	packageThumbnail *aasx.Part,
) (*aasx.Part, string) {
	if aas == nil || aas.AssetInformation() == nil || aas.AssetInformation().DefaultThumbnail() == nil {
		return nil, ""
	}

	thumbnailReference := strings.TrimSpace(aas.AssetInformation().DefaultThumbnail().Path())
	if thumbnailReference == "" {
		return nil, ""
	}

	normalizedReference := strings.ToLower(thumbnailReference)
	if strings.HasPrefix(normalizedReference, "http://") || strings.HasPrefix(normalizedReference, "https://") {
		return nil, ""
	}

	resolvedThumbnailURI := resolveReferenceAgainstSpec(thumbnailReference, specURI)
	if resolvedThumbnailURI != "" {
		if packageReader != nil {
			resolvedThumbnailURL, parseErr := url.Parse(resolvedThumbnailURI)
			if parseErr == nil {
				if thumbnailPart, findErr := packageReader.FindPart(resolvedThumbnailURL); findErr == nil && thumbnailPart != nil {
					return thumbnailPart, resolvedThumbnailURI
				}
			}
		}

		if thumbnailPart, exists := thumbnailPartsByURI[resolvedThumbnailURI]; exists {
			return thumbnailPart, resolvedThumbnailURI
		}
		if packageThumbnail != nil && normalizePartURI(packageThumbnail.URI) == resolvedThumbnailURI {
			return packageThumbnail, resolvedThumbnailURI
		}
	}

	if packageThumbnail == nil || len(thumbnailPartsByURI) > 0 {
		return nil, ""
	}

	packageThumbnailURI := normalizePartURI(packageThumbnail.URI)
	if packageThumbnailURI == "" {
		packageThumbnailURI = resolvedThumbnailURI
	}
	return packageThumbnail, packageThumbnailURI
}

func matchesSupplementaryTarget(fileValue string, specURI *url.URL, supplementaryURI *url.URL) bool {
	reference := strings.TrimSpace(fileValue)
	if reference == "" {
		return false
	}
	if strings.HasPrefix(reference, "http://") || strings.HasPrefix(reference, "https://") {
		return false
	}

	resolvedReference := resolveReferenceAgainstSpec(reference, specURI)
	resolvedSupplementary := normalizePartURI(supplementaryURI)
	return resolvedReference != "" && resolvedReference == resolvedSupplementary
}

func resolveReferenceAgainstSpec(reference string, specURI *url.URL) string {
	referenceURL, err := url.Parse(reference)
	if err != nil {
		return ""
	}
	if referenceURL.IsAbs() {
		return normalizePartURI(referenceURL)
	}

	if specURI == nil {
		if strings.HasPrefix(reference, "/") {
			parsed, parseErr := url.Parse(reference)
			if parseErr != nil {
				return ""
			}
			return normalizePartURI(parsed)
		}
		return ""
	}

	base := &url.URL{Path: normalizePartURI(specURI)}
	return normalizePartURI(base.ResolveReference(referenceURL))
}

func normalizePartURI(uri *url.URL) string {
	if uri == nil {
		return ""
	}

	uriPath := strings.TrimSpace(uri.Path)
	if uriPath == "" {
		uriPath = strings.TrimSpace(uri.String())
	}
	if uriPath == "" {
		return ""
	}

	uriPath = strings.ReplaceAll(uriPath, "\\", "/")
	if !strings.HasPrefix(uriPath, "/") {
		uriPath = "/" + uriPath
	}
	return path.Clean(uriPath)
}

func sanitizeUploadLogValue(value string) string {
	sanitized := strings.ReplaceAll(value, "\r", "")
	sanitized = strings.ReplaceAll(sanitized, "\n", "")
	return sanitized
}

func newUploadErrorResponse(status int, step string, err error) (commonmodel.ImplResponse, error) {
	if err == nil {
		err = errors.New(http.StatusText(status))
	}
	return common.NewErrorResponse(err, status, uploadComponent, uploadOperation, step), nil
}

func readUploadSignature(file io.ReadSeeker) ([]byte, error) {
	if _, err := file.Seek(0, 0); err != nil {
		return nil, err
	}

	buffer := make([]byte, 1024)
	readCount, readErr := file.Read(buffer)
	if readErr != nil && !errors.Is(readErr, io.EOF) {
		return nil, readErr
	}
	if _, err := file.Seek(0, 0); err != nil {
		return nil, err
	}

	return buffer[:readCount], nil
}

func detectUploadFormat(fileName string, contentType string, signature []byte) (string, string, error) {
	normalizedContentType := normalizeUploadContentType(contentType)
	if normalizedContentType != "" {
		switch normalizedContentType {
		case "application/aasx+xml", "application/aasx+json", "application/asset-administration-shell+xml", "application/asset-administration-shell+json":
			return "aasx", normalizedContentType, nil
		case "application/json":
			return "json", normalizedContentType, nil
		case "application/xml", "text/xml":
			return "xml", normalizedContentType, nil
		default:
			return "", "", fmt.Errorf(
				"unsupported content type %q, supported: %s",
				normalizedContentType,
				supportedUploadContentTypes,
			)
		}
	}

	extension := strings.ToLower(filepath.Ext(strings.TrimSpace(fileName)))
	switch extension {
	case ".aasx":
		return "aasx", "application/aasx+xml", nil
	case ".json":
		return "json", "application/json", nil
	case ".xml":
		return "xml", "application/xml", nil
	default:
		return detectUploadFormatFromContent(signature)
	}
}

func normalizeUploadContentType(contentType string) string {
	trimmed := strings.TrimSpace(strings.ToLower(contentType))
	if trimmed == "" {
		return ""
	}

	mediaType, _, err := mime.ParseMediaType(trimmed)
	if err != nil {
		return ""
	}

	normalized := strings.TrimSpace(strings.ToLower(mediaType))
	if normalized == "application/octet-stream" {
		return ""
	}

	return normalized
}

func uploadProcessingStatus(err error) int {
	switch {
	case common.IsErrPayloadTooLarge(err), errors.Is(err, aasx.ErrReaderLimitExceeded):
		return http.StatusRequestEntityTooLarge
	case common.IsErrBadRequest(err):
		return http.StatusBadRequest
	case common.IsErrMethodNotAllowed(err):
		return http.StatusMethodNotAllowed
	case common.IsErrNotFound(err):
		return http.StatusNotFound
	case common.IsErrDenied(err):
		return http.StatusForbidden
	case common.IsErrConflict(err):
		return http.StatusConflict
	case common.IsInternalServerError(err):
		return http.StatusInternalServerError
	default:
		return http.StatusInternalServerError
	}
}

func detectUploadFormatFromContent(signature []byte) (string, string, error) {
	trimmed := bytes.TrimSpace(signature)
	if len(trimmed) == 0 {
		return "", "", fmt.Errorf("unable to determine upload format from empty payload")
	}

	if len(signature) >= 2 && signature[0] == 0x50 && signature[1] == 0x4b {
		return "aasx", "application/aasx+xml", nil
	}

	firstByte := trimmed[0]
	if firstByte == '{' || firstByte == '[' {
		return "json", "application/json", nil
	}
	if firstByte == '<' {
		return "xml", "application/xml", nil
	}

	return "", "", fmt.Errorf(
		"unable to determine upload format from missing content type and file extension; supported content types: %s",
		supportedUploadContentTypes,
	)
}

func sanitizeUploadMetadataFileName(fileName string) string {
	baseName := filepath.Base(strings.TrimSpace(fileName))
	if baseName == "" || baseName == "." || baseName == "/" {
		return ""
	}

	return baseName
}
