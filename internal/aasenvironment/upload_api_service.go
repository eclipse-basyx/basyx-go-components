package aasenvironment

import (
	"bytes"
	"context"
	"encoding/json"
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"os"
	"path"
	"path/filepath"
	"strings"

	aasjsonization "github.com/aas-core-works/aas-core3.1-golang/jsonization"
	aastypes "github.com/aas-core-works/aas-core3.1-golang/types"
	aasxmlization "github.com/aas-core-works/aas-core3.1-golang/xmlization"
	aasx "github.com/aas-core-works/aas-package3-golang"
	"github.com/eclipse-basyx/basyx-go-components/internal/common"
	commonmodel "github.com/eclipse-basyx/basyx-go-components/internal/common/model"
)

type uploadAPIService struct {
	persistence *Persistence
}

const (
	uploadComponent = "AASENV"
	uploadOperation = "HandleUpload"
)

// NewUploadAPIService creates a new UploadAPIService
func NewUploadAPIService(persistence *Persistence) UploadService {
	return &uploadAPIService{persistence: persistence}
}

func (s *uploadAPIService) HandleUpload(ctx context.Context, fileName string, contentType string, file *os.File) (commonmodel.ImplResponse, error) {
	if file == nil {
		return newUploadErrorResponse(http.StatusBadRequest, "AASENV-HANDLEUPLOAD-MISSINGFILE", fmt.Errorf("uploaded file is required"))
	}

	normalizedFileName := strings.TrimSpace(fileName)
	if normalizedFileName == "" {
		return newUploadErrorResponse(http.StatusBadRequest, "AASENV-HANDLEUPLOAD-MISSINGFILENAME", fmt.Errorf("file name is required"))
	}

	if _, err := file.Seek(0, io.SeekStart); err != nil {
		return newUploadErrorResponse(http.StatusInternalServerError, "AASENV-HANDLEUPLOAD-RESETFILEPOINTER", err)
	}

	rawContent, err := io.ReadAll(file)
	if err != nil {
		return newUploadErrorResponse(http.StatusInternalServerError, "AASENV-HANDLEUPLOAD-READFILE", err)
	}

	extension := strings.ToLower(filepath.Ext(normalizedFileName))
	switch extension {
	case ".aasx":
		return s.handleAASXUpload(ctx, normalizedFileName, contentType, rawContent)
	case ".json":
		return s.handleJSONUpload(ctx, normalizedFileName, contentType, rawContent)
	case ".xml":
		return s.handleXMLUpload(ctx, normalizedFileName, contentType, rawContent)
	default:
		return newUploadErrorResponse(
			http.StatusBadRequest,
			"AASENV-HANDLEUPLOAD-UNSUPPORTEDFILETYPE",
			fmt.Errorf("unsupported file extension %q, supported: .aasx, .json, .xml", extension),
		)
	}
}

func (s *uploadAPIService) handleAASXUpload(ctx context.Context, fileName string, contentType string, rawContent []byte) (commonmodel.ImplResponse, error) {
	packaging := aasx.NewPackaging()
	packageReader, err := packaging.OpenReadFromStream(bytes.NewReader(rawContent))
	if err != nil {
		return newUploadErrorResponse(http.StatusBadRequest, "AASENV-HANDLEUPLOAD-PARSEAASX", err)
	}
	defer func() {
		_ = packageReader.Close()
	}()

	if err = s.processAASXPackage(ctx, fileName, contentType, packageReader); err != nil {
		return newUploadErrorResponse(http.StatusInternalServerError, "AASENV-HANDLEUPLOAD-PROCESSAASX", err)
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

func (s *uploadAPIService) handleJSONUpload(ctx context.Context, fileName string, contentType string, rawContent []byte) (commonmodel.ImplResponse, error) {
	var jsonable any
	if err := json.Unmarshal(rawContent, &jsonable); err != nil {
		return newUploadErrorResponse(http.StatusBadRequest, "AASENV-HANDLEUPLOAD-PARSEJSON", err)
	}

	environment, err := aasjsonization.EnvironmentFromJsonable(jsonable)
	if err != nil {
		return newUploadErrorResponse(http.StatusBadRequest, "AASENV-HANDLEUPLOAD-PARSEJSONENVIRONMENT", err)
	}

	if err = s.processEnvironment(ctx, fileName, contentType, environment); err != nil {
		return newUploadErrorResponse(http.StatusInternalServerError, "AASENV-HANDLEUPLOAD-PROCESSJSONENVIRONMENT", err)
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

func (s *uploadAPIService) handleXMLUpload(ctx context.Context, fileName string, contentType string, rawContent []byte) (commonmodel.ImplResponse, error) {
	decoder := xml.NewDecoder(bytes.NewReader(rawContent))
	instance, err := aasxmlization.Unmarshal(decoder)
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
		return newUploadErrorResponse(http.StatusInternalServerError, "AASENV-HANDLEUPLOAD-PROCESSXMLENVIRONMENT", err)
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

func (s *uploadAPIService) processAASXPackage(ctx context.Context, _ string, _ string, packageReader *aasx.PackageRead) error {
	specPart, environment, err := readEnvironmentFromAASXSpec(packageReader)
	if err != nil {
		return err
	}

	if err = s.processEnvironment(ctx, "", "", environment); err != nil {
		return err
	}

	if err = s.storeAASXThumbnail(ctx, packageReader, environment); err != nil {
		return err
	}

	return s.uploadSupplementaryFiles(packageReader, specPart, environment)
}

func (s *uploadAPIService) processEnvironment(ctx context.Context, _ string, _ string, environment aastypes.IEnvironment) error {
	if s == nil || s.persistence == nil {
		return common.NewErrBadRequest("AASENV-PROCESSENV-NILPERSISTENCE persistence is required for environment import")
	}
	if s.persistence.ConceptDescriptionRepository == nil || s.persistence.SubmodelRepository == nil || s.persistence.AASRepository == nil {
		return common.NewErrBadRequest("AASENV-PROCESSENV-NILBACKEND one or more repository backends are not initialized")
	}

	for _, conceptDescription := range environment.ConceptDescriptions() {
		if _, err := s.persistence.ConceptDescriptionRepository.PutConceptDescription(ctx, conceptDescription.ID(), conceptDescription); err != nil {
			return fmt.Errorf("AASENV-PROCESSENV-PUTCD failed to store concept description '%s': %w", conceptDescription.ID(), err)
		}
	}

	for _, submodel := range environment.Submodels() {
		if _, err := s.persistence.SubmodelRepository.PutSubmodel(ctx, submodel.ID(), submodel); err != nil {
			return fmt.Errorf("AASENV-PROCESSENV-PUTSM failed to store submodel '%s': %w", submodel.ID(), err)
		}
	}

	for _, aas := range environment.AssetAdministrationShells() {
		if _, err := s.persistence.AASRepository.PutAssetAdministrationShellByID(ctx, aas.ID(), aas); err != nil {
			return fmt.Errorf("AASENV-PROCESSENV-PUTAAS failed to store AAS '%s': %w", aas.ID(), err)
		}
	}

	return nil
}

func readEnvironmentFromAASXSpec(packageReader *aasx.PackageRead) (*aasx.Part, aastypes.IEnvironment, error) {
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

	specContent, err := specPart.ReadAllBytes()
	if err != nil {
		return nil, nil, fmt.Errorf("AASENV-PARSEAASX-READSPEC failed to read AASX spec content: %w", err)
	}

	if isLikelyJSONSpec(specPart) {
		environment, parseErr := parseAASJSONEnvironment(specContent)
		if parseErr != nil {
			return nil, nil, fmt.Errorf("AASENV-PARSEAASX-UNMARSHALJSON failed to parse JSON spec: %w", parseErr)
		}
		return specPart, environment, nil
	}

	instance, err := parseAASXMLInstance(specContent)
	if err != nil {
		return nil, nil, fmt.Errorf("AASENV-PARSEAASX-UNMARSHALXML failed to parse XML spec: %w", err)
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

func parseAASJSONEnvironment(specContent []byte) (aastypes.IEnvironment, error) {
	var jsonable any
	if err := json.Unmarshal(specContent, &jsonable); err != nil {
		return nil, err
	}

	environment, err := aasjsonization.EnvironmentFromJsonable(jsonable)
	if err != nil {
		return nil, err
	}
	return environment, nil
}

func parseAASXMLInstance(specContent []byte) (aastypes.IClass, error) {
	instance, err := aasxmlization.Unmarshal(xml.NewDecoder(bytes.NewReader(specContent)))
	if err == nil {
		return instance, nil
	}

	normalized := normalizeXMLSpecContent(specContent)
	if len(normalized) == 0 || bytes.Equal(normalized, specContent) {
		return nil, err
	}

	retried, retryErr := aasxmlization.Unmarshal(xml.NewDecoder(bytes.NewReader(normalized)))
	if retryErr == nil {
		return retried, nil
	}

	sanitized, sanitizeErr := sanitizeXMLRootAttributes(normalized)
	if sanitizeErr == nil && len(sanitized) > 0 {
		sanitizedRetried, sanitizedRetryErr := aasxmlization.Unmarshal(xml.NewDecoder(bytes.NewReader(sanitized)))
		if sanitizedRetryErr == nil {
			return sanitizedRetried, nil
		}
		return nil, fmt.Errorf("%w (retry after normalization failed: %v; retry after root attribute sanitization failed: %v)", err, retryErr, sanitizedRetryErr)
	}

	if sanitizeErr != nil {
		return nil, fmt.Errorf("%w (retry after normalization failed: %v; root attribute sanitization failed: %v)", err, retryErr, sanitizeErr)
	}

	return nil, fmt.Errorf("%w (retry after normalization failed: %v)", err, retryErr)
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

type aasxFileLocation struct {
	SubmodelID  string
	IDShortPath string
	FileValue   string
}

func (s *uploadAPIService) uploadSupplementaryFiles(
	packageReader *aasx.PackageRead,
	specPart *aasx.Part,
	environment aastypes.IEnvironment,
) error {
	if s == nil || s.persistence == nil || s.persistence.SubmodelRepository == nil {
		return common.NewErrBadRequest("AASENV-UPLDSUPPL-NILSMREPO submodel repository backend is required")
	}

	supplementaries, err := packageReader.SupplementaryRelationships()
	if err != nil {
		return fmt.Errorf("AASENV-UPLDSUPPL-READREL failed to read supplementary relationships: %w", err)
	}

	specURI := normalizePartURI(specPart.URI)
	fileLocations := collectFileLocations(environment)
	uploaded := 0

	for _, relationship := range supplementaries {
		if normalizePartURI(relationship.Spec.URI) != specURI {
			continue
		}

		suppBytes, readErr := relationship.Supplementary.ReadAllBytes()
		if readErr != nil {
			return fmt.Errorf("AASENV-UPLDSUPPL-READBYTES failed to read supplementary '%s': %w", normalizePartURI(relationship.Supplementary.URI), readErr)
		}

		matched := false
		for _, location := range fileLocations {
			if !matchesSupplementaryTarget(location.FileValue, relationship.Spec.URI, relationship.Supplementary.URI) {
				continue
			}

			uploadName := filepath.Base(normalizePartURI(relationship.Supplementary.URI))
			if uploadName == "." || uploadName == "/" || uploadName == "" {
				uploadName = filepath.Base(strings.TrimSpace(location.FileValue))
			}
			if uploadName == "." || uploadName == "/" || uploadName == "" {
				uploadName = "supplementary.bin"
			}

			tempFile, tempErr := createTempFileForUpload(uploadName, suppBytes)
			if tempErr != nil {
				return fmt.Errorf("AASENV-UPLDSUPPL-CREATETEMP failed to stage supplementary '%s': %w", uploadName, tempErr)
			}

			uploadErr := s.persistence.SubmodelRepository.UploadFileAttachment(location.SubmodelID, location.IDShortPath, tempFile, uploadName)
			closeAndRemoveTempFile(tempFile)
			if uploadErr != nil {
				return fmt.Errorf(
					"AASENV-UPLDSUPPL-UPLOAD failed to upload supplementary '%s' for submodel '%s' at path '%s': %w",
					uploadName,
					location.SubmodelID,
					location.IDShortPath,
					uploadErr,
				)
			}

			matched = true
			uploaded++
		}

		if !matched {
			supplementaryURIForLog := sanitizeLogValue(normalizePartURI(relationship.Supplementary.URI))
			// #nosec G706 -- value is sanitized to strip CR/LF control characters before logging.
			log.Printf("[WARN] AASENV-UPLDSUPPL-NOMATCH no File element path matched supplementary %q", supplementaryURIForLog)
		}
	}

	log.Printf("AASENV-UPLDSUPPL uploaded %d supplementary file attachment(s)", uploaded)
	return nil
}

func (s *uploadAPIService) storeAASXThumbnail(ctx context.Context, packageReader *aasx.PackageRead, environment aastypes.IEnvironment) error {
	if s == nil || s.persistence == nil || s.persistence.AASRepository == nil {
		return common.NewErrBadRequest("AASENV-UPLDTHUMB-NILAASREPO AAS repository backend is required")
	}
	if packageReader == nil {
		return common.NewErrBadRequest("AASENV-UPLDTHUMB-NILREADER package reader is required")
	}

	thumbnail, err := packageReader.Thumbnail()
	if err != nil {
		return fmt.Errorf("AASENV-UPLDTHUMB-READTHUMBNAIL failed to read AASX thumbnail: %w", err)
	}
	if thumbnail == nil {
		return nil
	}

	thumbnailBytes, err := thumbnail.ReadAllBytes()
	if err != nil {
		return fmt.Errorf("AASENV-UPLDTHUMB-READBYTES failed to read AASX thumbnail bytes: %w", err)
	}
	if len(thumbnailBytes) == 0 {
		return nil
	}

	thumbnailName := filepath.Base(normalizePartURI(thumbnail.URI))
	if strings.TrimSpace(thumbnailName) == "" || thumbnailName == "." || thumbnailName == "/" {
		thumbnailName = "thumbnail.bin"
	}

	for _, aas := range environment.AssetAdministrationShells() {
		tempFile, tempErr := createTempFileForUpload(thumbnailName, thumbnailBytes)
		if tempErr != nil {
			return fmt.Errorf("AASENV-UPLDTHUMB-CREATETEMP failed to stage thumbnail for AAS '%s': %w", aas.ID(), tempErr)
		}

		uploadErr := s.persistence.AASRepository.PutThumbnailByAASID(ctx, aas.ID(), thumbnailName, tempFile)
		closeAndRemoveTempFile(tempFile)
		if uploadErr != nil {
			return fmt.Errorf("AASENV-UPLDTHUMB-UPLOAD failed to store thumbnail for AAS '%s': %w", aas.ID(), uploadErr)
		}
	}

	log.Printf("AASENV-UPLDTHUMB stored thumbnail for %d AAS object(s)", len(environment.AssetAdministrationShells()))
	return nil
}

func collectFileLocations(environment aastypes.IEnvironment) []aasxFileLocation {
	locations := make([]aasxFileLocation, 0)
	for _, submodel := range environment.Submodels() {
		walkFileElements(submodel.ID(), submodel.SubmodelElements(), "", false, &locations)
	}
	return locations
}

func walkFileElements(
	submodelID string,
	elements []aastypes.ISubmodelElement,
	parentPath string,
	isFromList bool,
	locations *[]aasxFileLocation,
) {
	for position, element := range elements {
		idShort := ""
		if element.IDShort() != nil {
			idShort = *element.IDShort()
		}

		idShortPath := buildUploadIDShortPath(parentPath, isFromList, position, idShort)
		if element.ModelType() == aastypes.ModelTypeFile {
			if fileElement, ok := element.(*aastypes.File); ok && fileElement.Value() != nil && strings.TrimSpace(*fileElement.Value()) != "" && idShortPath != "" {
				*locations = append(*locations, aasxFileLocation{
					SubmodelID:  submodelID,
					IDShortPath: idShortPath,
					FileValue:   *fileElement.Value(),
				})
			}
		}

		children := extractSubmodelElementChildren(element)
		if len(children) == 0 {
			continue
		}

		walkFileElements(
			submodelID,
			children,
			idShortPath,
			element.ModelType() == aastypes.ModelTypeSubmodelElementList,
			locations,
		)
	}
}

func extractSubmodelElementChildren(element aastypes.ISubmodelElement) []aastypes.ISubmodelElement {
	switch element.ModelType() {
	case aastypes.ModelTypeSubmodelElementCollection:
		if collection, ok := element.(*aastypes.SubmodelElementCollection); ok {
			return collection.Value()
		}
	case aastypes.ModelTypeSubmodelElementList:
		if list, ok := element.(*aastypes.SubmodelElementList); ok {
			return list.Value()
		}
	case aastypes.ModelTypeAnnotatedRelationshipElement:
		if annotated, ok := element.(*aastypes.AnnotatedRelationshipElement); ok {
			children := make([]aastypes.ISubmodelElement, 0, len(annotated.Annotations()))
			for _, annotation := range annotated.Annotations() {
				children = append(children, annotation)
			}
			return children
		}
	case aastypes.ModelTypeEntity:
		if entity, ok := element.(*aastypes.Entity); ok {
			return entity.Statements()
		}
	}
	return nil
}

func buildUploadIDShortPath(parentPath string, isFromList bool, position int, idShort string) string {
	if parentPath == "" {
		if isFromList {
			return "[" + fmt.Sprintf("%d", position) + "]"
		}
		return idShort
	}
	if isFromList {
		return parentPath + "[" + fmt.Sprintf("%d", position) + "]"
	}
	return parentPath + "." + idShort
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

func createTempFileForUpload(fileName string, content []byte) (*os.File, error) {
	baseName := filepath.Base(strings.TrimSpace(fileName))
	if baseName == "." || baseName == "/" || baseName == "" {
		baseName = "supplementary.bin"
	}

	tempFile, err := os.CreateTemp("", baseName+".*")
	if err != nil {
		return nil, err
	}
	if _, err = tempFile.Write(content); err != nil {
		closeAndRemoveTempFile(tempFile)
		return nil, err
	}
	if _, err = tempFile.Seek(0, 0); err != nil {
		closeAndRemoveTempFile(tempFile)
		return nil, err
	}
	return tempFile, nil
}

func closeAndRemoveTempFile(tempFile *os.File) {
	if tempFile == nil {
		return
	}

	tempFileName := tempFile.Name()
	_ = tempFile.Close()
	// #nosec G703 -- path comes from os.CreateTemp/commonmodel.ReadFormFileToTempFile and is not user-controlled traversal.
	_ = os.Remove(tempFileName)
}

func sanitizeLogValue(value string) string {
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
