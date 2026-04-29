package aasenvironment

import (
	"bytes"
	"context"
	"encoding/json"
	"encoding/xml"
	"errors"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	aasjsonization "github.com/aas-core-works/aas-core3.1-golang/jsonization"
	aastypes "github.com/aas-core-works/aas-core3.1-golang/types"
	aasxmlization "github.com/aas-core-works/aas-core3.1-golang/xmlization"
	aasx "github.com/aas-core-works/aas-package3-golang"
	"github.com/eclipse-basyx/basyx-go-components/internal/common"
	commonmodel "github.com/eclipse-basyx/basyx-go-components/internal/common/model"
)

type uploadAPIService struct{}

const (
	uploadComponent = "AASENV"
	uploadOperation = "HandleUpload"
)

// NewUploadAPIService creates a new UploadAPIService
func NewUploadAPIService() UploadService {
	return &uploadAPIService{}
}

func (s *uploadAPIService) HandleUpload(ctx context.Context, fileName string, contentType string, file *os.File) (commonmodel.ImplResponse, error) {
	if file == nil {
		return newUploadErrorResponse(http.StatusBadRequest, "AASENV-HANDLEUPLOAD-MISSINGFILE", fmt.Errorf("uploaded file is required"))
	}

	normalizedFileName := strings.TrimSpace(fileName)
	if normalizedFileName == "" {
		return newUploadErrorResponse(http.StatusBadRequest, "AASENV-HANDLEUPLOAD-MISSINGFILENAME", fmt.Errorf("file name is required"))
	}

	rawContent, err := os.ReadFile(file.Name())
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

func (s *uploadAPIService) processAASXPackage(_ context.Context, _ string, _ string, packageReader *aasx.PackageRead) error {
	// TODO AASENV-HANDLEUPLOAD-PROCESSAASX: use packageReader to extract specs, supplementary files and persist/import them.

	// Read the specs
	specs, err := packageReader.Specs()
	if err != nil {
		return fmt.Errorf("failed to read AASX specs: %w", err)
	}
	fmt.Printf("Read %d specs from AASX package\n", len(specs))

	specsByContentType, err := packageReader.SpecsByContentType()
	if err != nil {
		return err
	}

	for ct, specs := range specsByContentType {
		fmt.Printf("%s -> %d\n", ct, len(specs))
	}

	supplementaries, err := packageReader.SupplementaryRelationships()
	if err != nil {
		return fmt.Errorf("failed to read AASX supplementary relationships: %w", err)
	}
	for _, sups := range supplementaries {

		//content, _ := sups.Supplementary.ReadAllBytes()
		contentType := sups.Supplementary.ContentType
		fmt.Println(sups.Spec.URI)
		fmt.Println(contentType)
		fmt.Println("----")
	}

	fmt.Printf("Read %d supplementary relationships from AASX package\n", supplementaries)

	thumbnail, err := packageReader.Thumbnail()
	if err != nil {
		return fmt.Errorf("failed to read AASX thumbnail: %w", err)
	}
	if thumbnail != nil {
		fmt.Printf("Read thumbnail from AASX package: contentType=%s\n", thumbnail.ContentType)
	} else {
		fmt.Println("No thumbnail found in AASX package")
	}
	return nil
}

func (s *uploadAPIService) processEnvironment(_ context.Context, _ string, _ string, environment aastypes.IEnvironment) error {
	// TODO AASENV-HANDLEUPLOAD-PROCESSENVIRONMENT: use environment to import AAS/Submodels/ConceptDescriptions into repositories.
	_ = environment
	return nil
}

func newUploadErrorResponse(status int, step string, err error) (commonmodel.ImplResponse, error) {
	if err == nil {
		err = errors.New(http.StatusText(status))
	}
	return common.NewErrorResponse(err, status, uploadComponent, uploadOperation, step), nil
}
