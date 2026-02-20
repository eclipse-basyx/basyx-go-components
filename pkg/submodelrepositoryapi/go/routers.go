/*
 * DotAAS Part 2 | HTTP/REST | Submodel Repository Service Specification
 *
 * The entire Submodel Repository Service Specification as part of the [Specification of the Asset Administration Shell: Part 2](http://industrialdigitaltwin.org/en/content-hub).   Publisher: Industrial Digital Twin Association (IDTA) 2023
 *
 * API version: V3.0.3_SSP-001
 * Contact: info@idtwin.org
 */

package openapi

import (
	"encoding/json"
	"errors"
	"io"
	"mime/multipart"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/eclipse-basyx/basyx-go-components/internal/common/model"
	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/go-chi/cors"
)

// Route defines the parameters for an API endpoint.
//
// It encapsulates the HTTP method, URL pattern, and handler function for a single
// API route in the Submodel Repository Service.
type Route struct {
	Method      string
	Pattern     string
	HandlerFunc http.HandlerFunc
}

// Routes is a map of defined API endpoints.
//
// The map key is a unique identifier for the route, and the value contains
// the route's method, pattern, and handler function.
type Routes map[string]Route

// Router defines the required methods for retrieving API routes.
//
// Implementations of this interface provide the route definitions for their
// respective API controllers in the Submodel Repository Service.
type Router interface {
	Routes() Routes
}

const errMsgRequiredMissing = "required parameter is missing"
const errMsgMinValueConstraint = "provided parameter is not respecting minimum value constraint"
const errMsgMaxValueConstraint = "provided parameter is not respecting maximum value constraint"

// NewRouter creates a new chi router for any number of API routers.
//
// This function initializes a chi router with logging middleware and CORS support,
// then registers all routes from the provided Router implementations.
//
// Parameters:
//   - routers: One or more Router implementations to register
//
// Returns:
//   - chi.Router: A configured chi router with all registered routes
func NewRouter(routers ...Router) chi.Router {
	router := chi.NewRouter()
	router.Use(middleware.Logger)
	router.Use(cors.Handler(cors.Options{}))
	for _, api := range routers {
		for _, route := range api.Routes() {
			var handler http.Handler = route.HandlerFunc
			router.Method(route.Method, route.Pattern, handler)
		}
	}

	return router
}

// Redirect is a helper payload type that signals the response encoder to send an HTTP redirect.
type Redirect struct {
	Location string
}

// FileDownload is a helper payload type for file downloads with custom content type.
type FileDownload struct {
	Content     []byte
	ContentType string
	Filename    string
}

// EncodeJSONResponse encodes a response as JSON and writes it to the HTTP response writer.
//
// This function handles both file responses (detected by *os.File type) and JSON responses.
// For files, it sets appropriate Content-Type and Content-Disposition headers. For other
// types, it encodes the response as JSON with application/json content type.
//
// Parameters:
//   - i: The interface to encode (can be *os.File for file downloads or any JSON-serializable type)
//   - status: Optional HTTP status code (uses 200 OK if nil)
//   - w: The HTTP response writer
//
// Returns:
//   - error: An error if encoding or writing fails, nil on success
func EncodeJSONResponse(i interface{}, status *int, w http.ResponseWriter) error {
	wHeader := w.Header()

	// Handle Redirect payloads: set Location header and write status without a body.
	if i != nil {
		switch r := i.(type) {
		case Redirect:
			wHeader.Set("Location", r.Location)
			if status != nil {
				w.WriteHeader(*status)
			} else {
				w.WriteHeader(http.StatusFound)
			}
			return nil
		case *Redirect:
			if r != nil {
				wHeader.Set("Location", r.Location)
				if status != nil {
					w.WriteHeader(*status)
				} else {
					w.WriteHeader(http.StatusFound)
				}
				return nil
			}
		case FileDownload:
			model.SetSafeDownloadHeaders(wHeader, r.Filename, r.ContentType)
			if status != nil {
				w.WriteHeader(*status)
			} else {
				w.WriteHeader(http.StatusOK)
			}
			// #nosec G705 -- writing binary attachment payload with Content-Disposition attachment and nosniff header
			_, err := w.Write(r.Content)
			return err
		case *FileDownload:
			if r != nil {
				model.SetSafeDownloadHeaders(wHeader, r.Filename, r.ContentType)
				if status != nil {
					w.WriteHeader(*status)
				} else {
					w.WriteHeader(http.StatusOK)
				}
				// #nosec G705 -- writing binary attachment payload with Content-Disposition attachment and nosniff header
				_, err := w.Write(r.Content)
				return err
			}
		}
	}

	f, ok := i.(*os.File)
	if ok {
		data, err := io.ReadAll(f)
		if err != nil {
			return err
		}
		model.SetSafeDownloadHeaders(wHeader, f.Name(), "application/octet-stream")
		if status != nil {
			w.WriteHeader(*status)
		} else {
			w.WriteHeader(http.StatusOK)
		}
		// #nosec G705 -- writing binary attachment payload with Content-Disposition attachment and nosniff header
		_, err = w.Write(data)
		return err
	}
	wHeader.Set("Content-Type", "application/json; charset=UTF-8")

	if status != nil {
		w.WriteHeader(*status)
	} else {
		w.WriteHeader(http.StatusOK)
	}

	if i != nil {
		return json.NewEncoder(w).Encode(i)
	}

	return nil
}

// ReadFormFileToTempFile reads a file from a multipart form request and writes it to a temporary file.
//
// The temporary file is created with the original filename as a prefix and a random suffix.
//
// Parameters:
//   - r: The HTTP request containing the multipart form data
//   - key: The form field name containing the file
//
// Returns:
//   - *os.File: A pointer to the temporary file
//   - error: An error if reading or writing fails
func ReadFormFileToTempFile(r *http.Request, key string) (*os.File, error) {
	_, fileHeader, err := r.FormFile(key)
	if err != nil {
		return nil, err
	}

	return readFileHeaderToTempFile(fileHeader)
}

// ReadFormFilesToTempFiles reads multiple files from a multipart form request and writes them to temporary files.
//
// Parameters:
//   - r: The HTTP request containing the multipart form data
//   - key: The form field name containing the files
//
// Returns:
//   - []*os.File: A slice of pointers to the temporary files
//   - error: An error if reading or writing fails
func ReadFormFilesToTempFiles(r *http.Request, key string) ([]*os.File, error) {
	if err := r.ParseMultipartForm(32 << 20); err != nil {
		return nil, err
	}

	files := make([]*os.File, 0, len(r.MultipartForm.File[key]))

	for _, fileHeader := range r.MultipartForm.File[key] {
		file, err := readFileHeaderToTempFile(fileHeader)
		if err != nil {
			return nil, err
		}

		files = append(files, file)
	}

	return files, nil
}

// readFileHeaderToTempFile reads multipart.FileHeader and writes it to a temporary file
func readFileHeaderToTempFile(fileHeader *multipart.FileHeader) (*os.File, error) {
	formFile, err := fileHeader.Open()
	if err != nil {
		return nil, err
	}

	defer func() {
		_ = formFile.Close()
	}()

	// Use .* as suffix, because the asterisk is a placeholder for the random value,
	// and the period allows consumers of this file to remove the suffix to obtain the original file name
	file, err := os.CreateTemp("", fileHeader.Filename+".*")
	if err != nil {
		return nil, err
	}

	defer func() {
		_ = file.Close()
	}()

	_, err = io.Copy(file, formFile)
	if err != nil {
		return nil, err
	}

	return file, nil
}

// nolint:unused
func parseTimes(param string) ([]time.Time, error) {
	splits := strings.Split(param, ",")
	times := make([]time.Time, 0, len(splits))
	for _, v := range splits {
		t, err := parseTime(v)
		if err != nil {
			return nil, err
		}
		times = append(times, t)
	}
	return times, nil
}

// parseTime will parses a string parameter into a time.Time using the RFC3339 format
// nolint:unused
func parseTime(param string) (time.Time, error) {
	if param == "" {
		return time.Time{}, nil
	}
	return time.Parse(time.RFC3339, param)
}

// Number is a constraint interface for numeric types.
//
// This interface restricts type parameters to numeric types (int32, int64, float32, float64)
// for use in generic parsing and validation functions.
type Number interface {
	~int32 | ~int64 | ~float32 | ~float64
}

// ParseString is a generic function type for parsing string values to specific types.
//
// Parameters:
//   - v: The string value to parse
//
// Returns:
//   - T: The parsed value of type T (constrained to Number, string, or bool)
//   - error: An error if parsing fails
type ParseString[T Number | string | bool] func(v string) (T, error)

// parseFloat64 parses a string parameter to an float64.
//
//nolint:unused
func parseFloat64(param string) (float64, error) {
	if param == "" {
		return 0, nil
	}

	return strconv.ParseFloat(param, 64)
}

// parseFloat32 parses a string parameter to an float32.
//
//nolint:unused
func parseFloat32(param string) (float32, error) {
	if param == "" {
		return 0, nil
	}

	v, err := strconv.ParseFloat(param, 32)
	return float32(v), err
}

// parseInt64 parses a string parameter to an int64.
//
//nolint:unused
func parseInt64(param string) (int64, error) {
	if param == "" {
		return 0, nil
	}

	return strconv.ParseInt(param, 10, 64)
}

// parseInt32 parses a string parameter to an int32.
func parseInt32(param string) (int32, error) {
	if param == "" {
		return 0, nil
	}

	val, err := strconv.ParseInt(param, 10, 32)
	return int32(val), err
}

// parseBool parses a string parameter to an bool.
func parseBool(param string) (bool, error) {
	if param == "" {
		return false, nil
	}

	return strconv.ParseBool(param)
}

// OpenAPIOperation is a generic function type for OpenAPI parameter operations.
//
// This function type encapsulates parsing, validation, and default value handling for
// OpenAPI parameters.
//
// Parameters:
//   - actual: The actual string value from the request
//
// Returns:
//   - T: The parsed value of type T (constrained to Number, string, or bool)
//   - bool: True if a default value was used, false otherwise
//   - error: An error if parsing or validation fails
//
// nolint:all
type OpenAPIOperation[T Number | string | bool] func(actual string) (T, bool, error)

// WithRequire creates an OpenAPIOperation that requires a non-empty value.
//
// If the actual parameter is empty, an error is returned indicating the required parameter is missing.
//
// Parameters:
//   - parse: The parsing function to apply to non-empty values
//
// Returns:
//   - OpenAPIOperation[T]: A function that enforces the required constraint and parses the value
func WithRequire[T Number | string | bool](parse ParseString[T]) OpenAPIOperation[T] {
	var empty T
	return func(actual string) (T, bool, error) {
		if actual == "" {
			return empty, false, errors.New(errMsgRequiredMissing)
		}

		v, err := parse(actual)
		return v, false, err
	}
}

// WithDefaultOrParse creates an OpenAPIOperation that uses a default value if the parameter is empty.
//
// If the actual parameter is empty, the default value is returned. Otherwise, the value is parsed.
//
// Parameters:
//   - def: The default value to use when the parameter is empty
//   - parse: The parsing function to apply to non-empty values
//
// Returns:
//   - OpenAPIOperation[T]: A function that provides default value handling and parses non-empty values
func WithDefaultOrParse[T Number | string | bool](def T, parse ParseString[T]) OpenAPIOperation[T] {
	return func(actual string) (T, bool, error) {
		if actual == "" {
			return def, true, nil
		}

		v, err := parse(actual)
		return v, false, err
	}
}

// WithParse creates an OpenAPIOperation that simply parses the value without defaults or requirements.
//
// Parameters:
//   - parse: The parsing function to apply to the value
//
// Returns:
//   - OpenAPIOperation[T]: A function that parses the value without additional constraints
func WithParse[T Number | string | bool](parse ParseString[T]) OpenAPIOperation[T] {
	return func(actual string) (T, bool, error) {
		v, err := parse(actual)
		return v, false, err
	}
}

// Constraint is a generic function type for validating parameter values.
//
// Parameters:
//   - actual: The value to validate
//
// Returns:
//   - error: An error if validation fails, nil if the constraint is satisfied
type Constraint[T Number | string | bool] func(actual T) error

// WithMinimum creates a Constraint that validates a minimum value.
//
// Parameters:
//   - expected: The minimum allowed value
//
// Returns:
//   - Constraint[T]: A function that validates the actual value is >= the minimum
func WithMinimum[T Number](expected T) Constraint[T] {
	return func(actual T) error {
		if actual < expected {
			return errors.New(errMsgMinValueConstraint)
		}

		return nil
	}
}

// WithMaximum creates a Constraint that validates a maximum value.
//
// Parameters:
//   - expected: The maximum allowed value
//
// Returns:
//   - Constraint[T]: A function that validates the actual value is <= the maximum
func WithMaximum[T Number](expected T) Constraint[T] {
	return func(actual T) error {
		if actual > expected {
			return errors.New(errMsgMaxValueConstraint)
		}

		return nil
	}
}

// parseNumericParameter parses a numeric parameter to its respective type.
func parseNumericParameter[T Number](param string, fn OpenAPIOperation[T], checks ...Constraint[T]) (T, error) {
	v, ok, err := fn(param)
	if err != nil {
		return 0, err
	}

	if !ok {
		for _, check := range checks {
			if err := check(v); err != nil {
				return 0, err
			}
		}
	}

	return v, nil
}

// parseBoolParameter parses a string parameter to a bool
func parseBoolParameter(param string, fn OpenAPIOperation[bool]) (bool, error) {
	v, _, err := fn(param)
	return v, err
}

// parseNumericArrayParameter parses a string parameter containing array of values to its respective type.
//
//nolint:unused
func parseNumericArrayParameter[T Number](param, delim string, required bool, fn OpenAPIOperation[T], checks ...Constraint[T]) ([]T, error) {
	if param == "" {
		if required {
			return nil, errors.New(errMsgRequiredMissing)
		}

		return nil, nil
	}

	str := strings.Split(param, delim)
	values := make([]T, len(str))

	for i, s := range str {
		v, ok, err := fn(s)
		if err != nil {
			return nil, err
		}

		if !ok {
			for _, check := range checks {
				if err := check(v); err != nil {
					return nil, err
				}
			}
		}

		values[i] = v
	}

	return values, nil
}

// parseQuery parses query parameters and returns an error if any malformed value pairs are encountered.
func parseQuery(rawQuery string) (url.Values, error) {
	return url.ParseQuery(rawQuery)
}
