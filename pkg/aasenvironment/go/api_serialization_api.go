package openapi

import (
	"net/http"
	"strings"

	"github.com/eclipse-basyx/basyx-go-components/internal/common"
)

// AASEnvironmentSerializationAPIAPIController binds HTTP requests to an API service and writes results to the HTTP response.
type AASEnvironmentSerializationAPIAPIController struct {
	service      SerializationAPIAPIServicer
	errorHandler ErrorHandler
	contextPath  string
}

// SerializationAPIAPIOption for how the controller is set up.
type SerializationAPIAPIOption func(*AASEnvironmentSerializationAPIAPIController)

// WithSerializationAPIAPIErrorHandler inject ErrorHandler into controller
func WithSerializationAPIAPIErrorHandler(h ErrorHandler) SerializationAPIAPIOption {
	return func(c *AASEnvironmentSerializationAPIAPIController) {
		c.errorHandler = h
	}
}

// NewSerializationAPIAPIController creates a default api controller
func NewSerializationAPIAPIController(s SerializationAPIAPIServicer, contextPath string, opts ...SerializationAPIAPIOption) *AASEnvironmentSerializationAPIAPIController {
	controller := &AASEnvironmentSerializationAPIAPIController{
		service:      s,
		errorHandler: DefaultErrorHandler,
		contextPath:  contextPath,
	}

	for _, opt := range opts {
		opt(controller)
	}

	return controller
}

// Routes returns all the api routes for the SerializationAPIAPIController
func (c *AASEnvironmentSerializationAPIAPIController) Routes() Routes {
	return Routes{
		"GenerateSerializationByIds": Route{
			strings.ToUpper("Get"),
			c.contextPath + "/serialization",
			c.GenerateSerializationByIds,
		},
	}
}

// GenerateSerializationByIds - Returns an appropriate serialization based on the specified format (see SerializationFormat)
func (c *AASEnvironmentSerializationAPIAPIController) GenerateSerializationByIds(w http.ResponseWriter, r *http.Request) {
	query, err := parseQuery(r.URL.RawQuery)
	if err != nil {
		c.errorHandler(w, r, &ParsingError{Err: err}, nil)
		return
	}
	aasIDsParam := parseStringArrayQueryParam(query["aasIds"])
	submodelIDsParam := parseStringArrayQueryParam(query["submodelIds"])
	var includeConceptDescriptionsParam bool
	if query.Has("includeConceptDescriptions") {
		param, err := parseBoolParameter(
			query.Get("includeConceptDescriptions"),
			WithParse[bool](parseBool),
		)
		if err != nil {
			c.errorHandler(w, r, &ParsingError{Param: "includeConceptDescriptions", Err: err}, nil)
			return
		}

		includeConceptDescriptionsParam = param
	} else {
		var param = true
		includeConceptDescriptionsParam = param
	}
	requestContext := common.WithAcceptHeader(r.Context(), r.Header.Get("Accept"))
	result, err := c.service.GenerateSerializationByIds(requestContext, aasIDsParam, submodelIDsParam, includeConceptDescriptionsParam)
	// If an error occurred, encode the error with the status code
	if err != nil {
		c.errorHandler(w, r, err, &result)
		return
	}
	// If no error, encode the body and the result code
	_ = EncodeJSONResponse(result.Body, &result.Code, w)
}

func parseStringArrayQueryParam(values []string) []string {
	if len(values) == 0 {
		return nil
	}

	result := make([]string, 0, len(values))
	for _, value := range values {
		for _, item := range strings.Split(value, ",") {
			trimmed := strings.TrimSpace(item)
			if trimmed == "" {
				continue
			}
			result = append(result, trimmed)
		}
	}

	if len(result) == 0 {
		return nil
	}

	return result
}
