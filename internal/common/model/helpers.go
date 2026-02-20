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

/*
 * DotAAS Part 1 | Metamodel | Schemas
 *
 * The schemas implementing the [Specification of the Asset Administration Shell: Part 1](https://industrialdigitaltwin.org/en/content-hub/aasspecifications).   Copyright: Industrial Digital Twin Association (IDTA) 2025
 *
 * API version: V3.1.1
 * Contact: info@idtwin.org
 */

// Package model provides data structures and utilities for the Asset Administration Shell metamodel.
// It implements the DotAAS Part 1 Metamodel Schemas according to the IDTA specification.//nolint:all
package model

import (
	"encoding/json"
	"errors"
	"io"
	"mime"
	"mime/multipart"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"reflect"
	"strconv"
	"strings"
	"time"
)

const errMsgRequiredMissing = "required parameter is missing"
const errMsgMinValueConstraint = "provided parameter is not respecting minimum value constraint"
const errMsgMaxValueConstraint = "provided parameter is not respecting maximum value constraint"

// Response creates an ImplResponse struct with the given status code and body.
func Response(code int, body interface{}) ImplResponse {
	return ImplResponse{
		Code: code,
		Body: body,
	}
}

// Redirect is a helper payload type that signals the response encoder to send an HTTP redirect.
type Redirect struct {
	Location string
}

func setSafeDownloadHeaders(wHeader http.Header, filename, contentType string) {
	if contentType == "" {
		contentType = "application/octet-stream"
	}

	wHeader.Set("Content-Type", contentType)
	wHeader.Set("X-Content-Type-Options", "nosniff")

	if filename == "" {
		wHeader.Set("Content-Disposition", "attachment")
		return
	}

	safeFilename := filepath.Base(filename)
	contentDisposition := mime.FormatMediaType("attachment", map[string]string{"filename": safeFilename})
	wHeader.Set("Content-Disposition", contentDisposition)
}

// ResponseWithHeaders builds an ImplResponse and converts a Location header into a Redirect payload
// so the encoder can set the Location header on the actual HTTP response.
func ResponseWithHeaders(code int, payload interface{}, headers map[string]string) ImplResponse {
	// If a Location header is provided and no payload is required, return Redirect so encoder can act.
	if headers != nil {
		if loc, ok := headers["Location"]; ok {
			// Use Redirect as body to signal encoder to set Location header and status.
			return Response(code, Redirect{Location: loc})
		}
	}
	// fallback to normal response
	return Response(code, payload)
}

// IsZeroValue checks if the val is the zero-ed value.
func IsZeroValue(val interface{}) bool {
	return val == nil || reflect.DeepEqual(val, reflect.Zero(reflect.TypeOf(val)).Interface())
}

// AssertRecurseInterfaceRequired recursively checks each struct in a slice against the callback.
// This method traverse nested slices in a preorder fashion.
func AssertRecurseInterfaceRequired[T any](obj interface{}, callback func(T) error) error {
	return AssertRecurseValueRequired(reflect.ValueOf(obj), callback)
}

// AssertRecurseValueRequired checks each struct in the nested slice against the callback.
// This method traverse nested slices in a preorder fashion. ErrTypeAssertionError is thrown if
// the underlying struct does not match type T.
func AssertRecurseValueRequired[T any](value reflect.Value, callback func(T) error) error {
	switch value.Kind() {
	// If it is a struct we check using callback
	case reflect.Struct:
		obj, ok := value.Interface().(T)
		if !ok {
			return ErrTypeAssertionError
		}

		if err := callback(obj); err != nil {
			return err
		}

	// If it is a slice we continue recursion
	case reflect.Slice:
		for i := 0; i < value.Len(); i++ {
			if err := AssertRecurseValueRequired(value.Index(i), callback); err != nil {
				return err
			}
		}
	}
	return nil
}

// EncodeJSONResponse uses the json encoder to write an interface to the http response with an optional status code
func EncodeJSONResponse(i interface{}, status *int, w http.ResponseWriter) error {
	wHeader := w.Header()

	// Handle Redirect payloads: set Location header and write status without a body.
	if i != nil {
		var redirect *Redirect
		switch r := i.(type) {
		case Redirect:
			redirect = &r
		case *Redirect:
			redirect = r
		}
		if redirect != nil {
			if status != nil {
				wHeader.Set("Location", redirect.Location)
				w.WriteHeader(*status)
			} else {
				wHeader.Set("Location", redirect.Location)
				w.WriteHeader(http.StatusFound)
			}
			return nil
		}
	}

	f, ok := i.(*os.File)
	if ok {
		data, err := io.ReadAll(f)
		if err != nil {
			return err
		}
		setSafeDownloadHeaders(wHeader, f.Name(), "application/octet-stream")
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

// ReadFormFileToTempFile reads file data from a request form and writes it to a temporary file
func ReadFormFileToTempFile(r *http.Request, key string) (*os.File, error) {
	_, fileHeader, err := r.FormFile(key)
	if err != nil {
		return nil, err
	}

	return readFileHeaderToTempFile(fileHeader)
}

// ReadFormFilesToTempFiles reads files array data from a request form and writes it to a temporary files
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

// parseTime parses a string parameter into a time.Time using the RFC3339 format.
// This function is currently unused but kept for potential future use.
// nolint:unused
func parseTime(param string) (time.Time, error) {
	if param == "" {
		return time.Time{}, nil
	}
	return time.Parse(time.RFC3339, param)
}

// Number is a type constraint that allows numeric types (int32, int64, float32, float64).
// It's used in generic functions for parsing and validating numeric parameters.
type Number interface {
	~int32 | ~int64 | ~float32 | ~float64
}

// ParseString is a function type for parsing string values into various types.
// It takes a string value and returns the parsed value of type T and an error if parsing fails.
type ParseString[T Number | string | bool] func(v string) (T, error)

// parseFloat64 parses a string parameter to an float64.
// nolint:unused
func parseFloat64(param string) (float64, error) {
	if param == "" {
		return 0, nil
	}

	return strconv.ParseFloat(param, 64)
}

// parseFloat32 parses a string parameter to an float32.
// nolint:unused
func parseFloat32(param string) (float32, error) {
	if param == "" {
		return 0, nil
	}

	v, err := strconv.ParseFloat(param, 32)
	return float32(v), err
}

// parseInt64 parses a string parameter to an int64.
// nolint:unused
func parseInt64(param string) (int64, error) {
	if param == "" {
		return 0, nil
	}

	return strconv.ParseInt(param, 10, 64)
}

// parseInt32 parses a string parameter to an int32.
// nolint:unused
func parseInt32(param string) (int32, error) {
	if param == "" {
		return 0, nil
	}

	val, err := strconv.ParseInt(param, 10, 32)
	return int32(val), err
}

// parseBool parses a string parameter to an bool.
// nolint:unused
func parseBool(param string) (bool, error) {
	if param == "" {
		return false, nil
	}

	return strconv.ParseBool(param)
}

// HOperation is a function type that handles parameter parsing with optional default values.
// It returns the parsed value, a boolean indicating if a default was used, and any parsing error.
type HOperation[T Number | string | bool] func(actual string) (T, bool, error)

// WithRequire creates an HOperation that requires a non-empty parameter value.
// It returns an error if the parameter is empty, otherwise parses the value using the provided parser.
func WithRequire[T Number | string | bool](parse ParseString[T]) HOperation[T] {
	var empty T
	return func(actual string) (T, bool, error) {
		if actual == "" {
			return empty, false, errors.New(errMsgRequiredMissing)
		}

		v, err := parse(actual)
		return v, false, err
	}
}

// WithDefaultOrParse creates an HOperation that uses a default value when the parameter is empty,
// otherwise parses the parameter using the provided parser.
func WithDefaultOrParse[T Number | string | bool](def T, parse ParseString[T]) HOperation[T] {
	return func(actual string) (T, bool, error) {
		if actual == "" {
			return def, true, nil
		}

		v, err := parse(actual)
		return v, false, err
	}
}

// WithParse returns a HOperation that only parses the value without additional checks.
func WithParse[T Number | string | bool](parse ParseString[T]) HOperation[T] {
	return func(actual string) (T, bool, error) {
		v, err := parse(actual)
		return v, false, err
	}
}

// Constraint defines a function type for validating a value of type T.
type Constraint[T Number | string | bool] func(actual T) error

// WithMinimum returns a constraint that checks if the actual value is greater than or equal to the expected value.
func WithMinimum[T Number](expected T) Constraint[T] {
	return func(actual T) error {
		if actual < expected {
			return errors.New(errMsgMinValueConstraint)
		}

		return nil
	}
}

// WithMaximum returns a constraint that checks if the actual value is less than or equal to the expected value.
func WithMaximum[T Number](expected T) Constraint[T] {
	return func(actual T) error {
		if actual > expected {
			return errors.New(errMsgMaxValueConstraint)
		}

		return nil
	}
}

// parseNumericParameter parses a numeric parameter to its respective type.
// nolint:unused
func parseNumericParameter[T Number](param string, fn HOperation[T], checks ...Constraint[T]) (T, error) {
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
// nolint:unused
func parseBoolParameter(param string, fn HOperation[bool]) (bool, error) {
	v, _, err := fn(param)
	return v, err
}

// parseNumericArrayParameter parses a string parameter containing array of values to its respective type.
// nolint:unused
func parseNumericArrayParameter[T Number](param, delim string, required bool, fn HOperation[T], checks ...Constraint[T]) ([]T, error) {
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
// nolint:unused
func parseQuery(rawQuery string) (url.Values, error) {
	return url.ParseQuery(rawQuery)
}
