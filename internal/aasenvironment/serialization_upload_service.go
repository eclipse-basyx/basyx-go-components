package aasenvironment

import (
	"bytes"
	"context"
	"encoding/json"
	"encoding/xml"
	"fmt"
	"io"
	"mime"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"strconv"
	"strings"

	"github.com/aas-core-works/aas-core3.1-golang/jsonization"
	"github.com/aas-core-works/aas-core3.1-golang/types"
	"github.com/aas-core-works/aas-core3.1-golang/xmlization"
	aasx "github.com/aas-core-works/aas-package3-golang"
	"github.com/eclipse-basyx/basyx-go-components/internal/common"
	"github.com/eclipse-basyx/basyx-go-components/internal/common/model"
	"github.com/go-chi/chi/v5"
)

const (
	serializationUploadComponent = "AASENV"

	serializationOperation = "GenerateSerializationByIds"
	uploadOperation        = "UploadEnvironment"

	mediaTypeJSON = "application/json"
	mediaTypeXML  = "application/xml"

	mediaTypeAASXJSON            = "application/aasx+json"
	mediaTypeAASXXML             = "application/aasx+xml"
	mediaTypeAASJSONAlias        = "application/asset-administration-shell+json"
	mediaTypeAASXMLAlias         = "application/asset-administration-shell+xml"
	mediaTypeAASXLegacyXMLBundle = "application/asset-administration-shell-package+xml"
)

type serializationKind string

const (
	serializationKindJSON serializationKind = "json"
	serializationKindXML  serializationKind = "xml"
)

type specCandidate struct {
	part  *aasx.Part
	kind  serializationKind
	uri   string
	mime  string
	score int
}

// SerializationUploadService hosts custom AAS Environment upload/serialization endpoints.
type SerializationUploadService struct {
	persistence *Persistence
}

// NewSerializationUploadService constructs upload/serialization endpoint handlers.
func NewSerializationUploadService(persistence *Persistence) *SerializationUploadService {
	return &SerializationUploadService{persistence: persistence}
}

// RegisterRoutes attaches /serialization and /upload endpoints.
func (s *SerializationUploadService) RegisterRoutes(router chi.Router) {
	router.Get("/serialization", s.HandleSerialization)
	router.Post("/upload", s.HandleUpload)
}

// HandleSerialization serializes the combined environment as JSON, XML or AASX.
func (s *SerializationUploadService) HandleSerialization(w http.ResponseWriter, r *http.Request) {
	if s == nil || s.persistence == nil {
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

// HandleUpload uploads an Environment payload as JSON, XML or AASX (multipart file part).
func (s *SerializationUploadService) HandleUpload(w http.ResponseWriter, r *http.Request) {
	if s == nil || s.persistence == nil {
		s.writeErrorResponse(
			w,
			http.StatusInternalServerError,
			uploadOperation,
			"NilService",
			common.NewInternalServerError("AASENV-UPLOAD-NILSERVICE upload service must not be nil"),
		)
		return
	}

	filePayload, fileMediaType, readErr := readUploadedMultipartFile(r)
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

	for _, submodel := range environment.Submodels() {
		if submodel == nil {
			continue
		}
		identifier := strings.TrimSpace(submodel.ID())
		if identifier == "" {
			return common.NewErrBadRequest("AASENV-UPLOAD-SM-MISSINGID submodel identifier is required")
		}
		if _, err := s.persistence.SubmodelRepository.PutSubmodel(ctx, identifier, submodel); err != nil {
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
		if _, err := s.persistence.AASRepository.PutAssetAdministrationShellByID(ctx, identifier, shell); err != nil {
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
		if err := s.persistence.ConceptDescriptionRepository.PutConceptDescription(ctx, identifier, conceptDescription); err != nil {
			return fmt.Errorf("AASENV-UPLOAD-CD-PUTCD %w", err)
		}
	}

	return nil
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
		rawAASPayloads, _, listErr = s.persistence.AASRepository.GetAssetAdministrationShells(ctx, 0, "", "", nil)
		if listErr != nil {
			return nil, nil, fmt.Errorf("AASENV-SERIALIZATION-AAS-LIST %w", listErr)
		}
	} else {
		rawAASPayloads = make([]map[string]any, 0, len(aasIDs))
		for _, aasID := range deduplicateStrings(aasIDs) {
			aasPayload, getErr := s.persistence.AASRepository.GetAssetAdministrationShellByID(ctx, aasID)
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

	submodels, _, err := s.persistence.SubmodelRepository.GetSubmodels(ctx, 0, "", "")
	if err != nil {
		return nil, fmt.Errorf("AASENV-SERIALIZATION-SM-LIST %w", err)
	}

	return submodels, nil
}

func (s *SerializationUploadService) loadSubmodelsByIDs(ctx context.Context, submodelIDs []string) ([]types.ISubmodel, error) {
	result := make([]types.ISubmodel, 0, len(submodelIDs))
	for _, submodelID := range deduplicateStrings(submodelIDs) {
		submodel, err := s.persistence.SubmodelRepository.GetSubmodelByID(ctx, submodelID, "", false)
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

		conceptDescriptions, nextCursor, err := s.persistence.ConceptDescriptionRepository.GetConceptDescriptions(
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
	packaging := aasx.NewPackaging()
	pkg, err := packaging.OpenReadFromStream(bytes.NewReader(payload))
	if err != nil {
		return nil, common.NewErrBadRequest("AASENV-PARSE-AASX-OPEN " + err.Error())
	}
	defer func() {
		_ = pkg.Close()
	}()

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

	tmpFile, err := os.CreateTemp("", "aasenv-serialization-*.aasx")
	if err != nil {
		return nil, common.NewInternalServerError("AASENV-SERIALIZATION-AASX-CREATETEMP " + err.Error())
	}
	tmpPath := tmpFile.Name()
	if closeErr := tmpFile.Close(); closeErr != nil {
		_ = os.Remove(tmpPath)
		return nil, common.NewInternalServerError("AASENV-SERIALIZATION-AASX-CLOSETEMP " + closeErr.Error())
	}
	defer func() {
		_ = os.Remove(tmpPath)
	}()

	packaging := aasx.NewPackaging()
	pkg, createErr := packaging.Create(tmpPath)
	if createErr != nil {
		return nil, common.NewInternalServerError("AASENV-SERIALIZATION-AASX-CREATE " + createErr.Error())
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

	// #nosec G304 -- tmpPath is created by os.CreateTemp in this function.
	result, readErr := os.ReadFile(tmpPath)
	if readErr != nil {
		return nil, common.NewInternalServerError("AASENV-SERIALIZATION-AASX-READFILE " + readErr.Error())
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

func readUploadedMultipartFile(r *http.Request) ([]byte, string, error) {
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

	payload, readErr := io.ReadAll(formFile)
	if readErr != nil {
		return nil, "", common.NewErrBadRequest("AASENV-UPLOAD-READFILE " + readErr.Error())
	}

	return payload, fileMediaType, nil
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

func normalizeMediaType(raw string) string {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return ""
	}

	mediaType, _, err := mime.ParseMediaType(trimmed)
	if err != nil {
		mediaType = strings.Split(trimmed, ";")[0]
	}

	return strings.ToLower(strings.TrimSpace(mediaType))
}

func writeBinaryResponse(w http.ResponseWriter, statusCode int, contentType string, payload []byte, fileName string) {
	w.Header().Set("Content-Type", contentType)
	w.Header().Set("X-Content-Type-Options", "nosniff")
	if fileName != "" {
		safeName := filepath.Base(fileName)
		w.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=%q", safeName))
	}
	w.WriteHeader(statusCode)
	_, _ = w.Write(payload)
}

func statusFromError(err error) int {
	switch {
	case err == nil:
		return http.StatusInternalServerError
	case common.IsErrBadRequest(err):
		return http.StatusBadRequest
	case common.IsErrDenied(err):
		return http.StatusForbidden
	case common.IsErrNotFound(err):
		return http.StatusNotFound
	case common.IsErrConflict(err):
		return http.StatusConflict
	default:
		return http.StatusInternalServerError
	}
}

func (s *SerializationUploadService) writeErrorResponse(
	w http.ResponseWriter,
	statusCode int,
	operation string,
	info string,
	err error,
) {
	response := common.NewErrorResponse(err, statusCode, serializationUploadComponent, operation, info)
	_ = model.EncodeJSONResponse(response.Body, &response.Code, w)
}
