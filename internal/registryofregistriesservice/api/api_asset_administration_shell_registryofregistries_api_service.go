package api

import (
	"context"
	"errors"
	"log"
	"net/http"
	"strings"

	"github.com/eclipse-basyx/basyx-go-components/internal/common"
	"github.com/eclipse-basyx/basyx-go-components/internal/common/model"
	registryofregistriespostgresql "github.com/eclipse-basyx/basyx-go-components/internal/registryofregistriesservice/persistence"
)

const (
	componentName = "DISC"
)

// AssetAdministrationShellRegistryOfRegistriesAPIAPIService is a service that implements the logic for the AssetAdministrationShellRegistryOfRegistriesAPIAPIServicer
// This service should implement the business logic for every endpoint for the AssetAdministrationShellRegistryOfRegistriesAPIAPI API.
// Include any external packages or services that will be required by this service.
type AssetAdministrationShellRegistryOfRegistriesAPIAPIService struct {
	registryOfRegistriesBackend registryofregistriespostgresql.PostgreSQLRegistryOfRegistriesDatabase
}

// NewAssetAdministrationShellRegistryOfRegistriesAPIAPIService creates a default api service
func NewAssetAdministrationShellRegistryOfRegistriesAPIAPIService(registryOfRegistriesBackend registryofregistriespostgresql.PostgreSQLRegistryOfRegistriesDatabase) *AssetAdministrationShellRegistryOfRegistriesAPIAPIService {
	return &AssetAdministrationShellRegistryOfRegistriesAPIAPIService{
		registryOfRegistriesBackend: registryOfRegistriesBackend,
	}
}

// GetAllRegistryDescriptors - Returns all Registry Descriptors
func (s *AssetAdministrationShellRegistryOfRegistriesAPIAPIService) GetAllRegistryDescriptors(ctx context.Context, limit int32, cursor string, registryType string) (model.ImplResponse, error) {

	var internalCursor string
	if strings.TrimSpace(cursor) != "" {
		dec, decErr := common.DecodeString(cursor)
		if decErr != nil {
			log.Printf("📍 [%s] Error in GetAllRegistryDescriptors: decode cursor=%q limit=%d registryType=%q: %v", componentName, cursor, limit, registryType, decErr)
			return common.NewErrorResponse(
				decErr, http.StatusBadRequest, componentName, "GetAllRegistryDescriptors", "BadCursor",
			), nil
		}
		internalCursor = dec
	}
	aasds, nextCursor, err := s.registryOfRegistriesBackend.ListRegistryDescriptors(ctx, limit, internalCursor, registryType)
	if err != nil {
		log.Printf("📍 [%s] Error in GetAllRegistryDescriptors: list failed (limit=%d cursor=%q registryType=%q): %v", componentName, limit, internalCursor, registryType, err)
		switch {
		case common.IsErrBadRequest(err):
			return common.NewErrorResponse(
				err, http.StatusBadRequest, componentName, "GetAllRegistryDescriptors", "BadRequest",
			), nil
		default:
			return common.NewErrorResponse(
				err, http.StatusInternalServerError, componentName, "GetAllRegistryDescriptors", "InternalServerError",
			), err
		}
	}

	pm := model.PagedResultPagingMetadata{}
	if nextCursor != "" {
		pm.Cursor = common.EncodeString(nextCursor)
	}
	res := interface{}(struct {
		PagingMetadata interface{}
		Result         interface{}
	}{
		PagingMetadata: pm,
		Result:         aasds,
	})
	return model.Response(http.StatusOK, res), nil
}

// PostRegistryDescriptor - Creates a new Registry Descriptor, i.e. registers a Registry
func (s *AssetAdministrationShellRegistryOfRegistriesAPIAPIService) PostRegistryDescriptor(ctx context.Context, registryDescriptor model.RegistryDescriptor) (model.ImplResponse, error) {
	err := s.registryOfRegistriesBackend.InsertRegistryDescriptor(ctx, registryDescriptor)
	if err != nil {
		switch {
		case common.IsErrBadRequest(err):
			log.Printf("📍 [%s] Error in InsertRegistryDescriptor: bad request (aasId=%q): %v", componentName, registryDescriptor.Id, err)
			return common.NewErrorResponse(
				err, http.StatusBadRequest, componentName, "InsertRegistryDescriptor", "BadRequest",
			), nil
		case common.IsErrConflict(err):
			log.Printf("📍 [%s] Error in InsertRegistryDescriptor: conflict (aasId=%q): %v", componentName, registryDescriptor.Id, err)
			return common.NewErrorResponse(
				err, http.StatusConflict, componentName, "InsertRegistryDescriptor", "Conflict",
			), nil
		default:
			log.Printf("📍 [%s] Error in InsertRegistryDescriptor: internal (aasId=%q): %v", componentName, registryDescriptor.Id, err)
			return common.NewErrorResponse(
				err, http.StatusInternalServerError, componentName, "InsertRegistryDescriptor", "Unhandled",
			), err
		}
	}

	return model.Response(http.StatusCreated, registryDescriptor), nil
}

// GetRegistryDescriptorById - Returns a specific Registry Descriptor
func (s *AssetAdministrationShellRegistryOfRegistriesAPIAPIService) GetRegistryDescriptorById(ctx context.Context, registryIdentifier string) (model.ImplResponse, error) {
	decoded, decodeErr := common.DecodeString(registryIdentifier)
	if decodeErr != nil {
		log.Printf("R [%s] Error in GetRegistryDescriptorById: decode registryIdentifier=%q: %v", componentName, registryIdentifier, decodeErr)
		return common.NewErrorResponse(
			decodeErr, http.StatusBadRequest, componentName, "GetRegistryDescriptorById", "BadRequest-Decode",
		), nil
	}

	result, err := s.registryOfRegistriesBackend.GetRegistryDescriptorByID(ctx, decoded)

	if err != nil {
		switch {
		case common.IsErrBadRequest(err):
			log.Printf("📍 [%s] Error in GetRegistryDescriptorById: bad request (aasId=%q): %v", componentName, string(decoded), err)
			return common.NewErrorResponse(
				err, http.StatusBadRequest, componentName, "GetRegistryDescriptorById", "BadRequest",
			), nil
		case common.IsErrNotFound(err):
			log.Printf("📍 [%s] Error in GetRegistryDescriptorById: not found (aasId=%q): %v", componentName, string(decoded), err)
			return common.NewErrorResponse(
				err, http.StatusNotFound, componentName, "GetRegistryDescriptorById", "NotFound",
			), nil
		default:
			log.Printf("📍 [%s] Error in GetRegistryDescriptorById: internal (aasId=%q): %v", componentName, string(decoded), err)
			return common.NewErrorResponse(
				err, http.StatusInternalServerError, componentName, "GetRegistryDescriptorById", "Unhandled",
			), err
		}
	}
	return model.Response(http.StatusOK, result), nil
}

// PutRegistryDescriptorById - Creates or updates an existing Registry Descriptor
func (s *AssetAdministrationShellRegistryOfRegistriesAPIAPIService) PutRegistryDescriptorById(ctx context.Context, registryIdentifier string, registryDescriptor model.RegistryDescriptor) (model.ImplResponse, error) {
	// Decode path AAS id
	decodedRegistry, decErr := common.DecodeString(registryIdentifier)
	if decErr != nil {
		log.Printf("📍 [%s] Error in PutRegistryDescriptorById: decode registryIdentifier=%q: %v", componentName, registryIdentifier, decErr)
		return common.NewErrorResponse(
			decErr, http.StatusBadRequest, componentName, "PutRegistryDescriptorById", "BadRequest-Decode",
		), nil
	}

	// Enforce id consistency with path
	if strings.TrimSpace(registryDescriptor.Id) != "" && registryDescriptor.Id != decodedRegistry {
		log.Printf("📍 [%s] Error in PutRegistryDescriptorById: body id does not match path id (body=%q path=%q)", componentName, registryDescriptor.Id, decodedRegistry)
		return common.NewErrorResponse(
			errors.New("body id does not match path id"), http.StatusBadRequest, componentName, "PutRegistryDescriptorById", "BadRequest-IdMismatch",
		), nil
	}
	registryDescriptor.Id = decodedRegistry

	existed, err := s.registryOfRegistriesBackend.ReplaceRegistryDescriptor(ctx, registryDescriptor)
	if err != nil {
		switch {
		case common.IsErrBadRequest(err):
			log.Printf("📍 [%s] Error in PutRegistryDescriptorById: bad request (registryId=%q): %v", componentName, decodedRegistry, err)
			return common.NewErrorResponse(
				err, http.StatusBadRequest, componentName, "PutRegistryDescriptorById", "BadRequest",
			), nil
		case common.IsErrConflict(err):
			log.Printf("📍 [%s] Error in PutRegistryDescriptorById: conflict (registryId=%q): %v", componentName, decodedRegistry, err)
			return common.NewErrorResponse(
				err, http.StatusConflict, componentName, "PutRegistryDescriptorById", "Conflict",
			), nil
		default:
			log.Printf("📍 [%s] Error in PutRegistryDescriptorById: internal (registryId=%q): %v", componentName, decodedRegistry, err)
			return common.NewErrorResponse(
				err, http.StatusInternalServerError, componentName, "PutRegistryDescriptorById", "Unhandled-Insert",
			), err
		}
	}

	if existed {
		return model.Response(http.StatusNoContent, nil), nil
	}
	return model.Response(http.StatusCreated, registryDescriptor), nil
}

// DeleteRegistryDescriptorById - Deletes a Registry Descriptor, i.e. de-registers a Registry
func (s *AssetAdministrationShellRegistryOfRegistriesAPIAPIService) DeleteRegistryDescriptorById(ctx context.Context, registryIdentifier string) (model.ImplResponse, error) {

	decoded, decodeErr := common.DecodeString(registryIdentifier)
	if decodeErr != nil {
		log.Printf("📍 [%s] Error DeleteRegistryDescriptorById: decode aasIdentifier=%q failed: %v", componentName, registryIdentifier, decodeErr)
		return common.NewErrorResponse(
			decodeErr, http.StatusBadRequest, componentName, "DeleteRegistryDescriptorById", "BadRequest-Decode",
		), nil
	}

	if err := s.registryOfRegistriesBackend.DeleteRegistryDescriptorByID(ctx, decoded); err != nil {
		switch {
		case common.IsErrNotFound(err):
			log.Printf("📍 [%s] Error in DeleteRegistryDescriptorById: not found (aasId=%q): %v", componentName, decoded, err)
			return common.NewErrorResponse(
				err, http.StatusNotFound, componentName, "DeleteRegistryDescriptorById", "NotFound",
			), nil
		case common.IsErrBadRequest(err):
			log.Printf("📍 [%s] Error in DeleteRegistryDescriptorById: bad request (aasId=%q): %v", componentName, decoded, err)
			return common.NewErrorResponse(
				err, http.StatusBadRequest, componentName, "DeleteRegistryDescriptorById", "BadRequest",
			), nil
		default:
			log.Printf("📍 [%s] Error in DeleteRegistryDescriptorById: internal (aasId=%q): %v", componentName, decoded, err)
			return common.NewErrorResponse(
				err, http.StatusInternalServerError, componentName, "DeleteRegistryDescriptorById", "Unhandled",
			), err
		}
	}

	return model.Response(http.StatusNoContent, nil), nil
}
