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

package model

import (
	"errors"
	"io"
	"mime/multipart"
	"net/http"
	"strings"
)

// HandleMultipartFileStream streams the first multipart part named fileKey
// without staging it on disk. The file name and content type are taken from
// that part's headers; all other parts are discarded.
func HandleMultipartFileStream(
	r *http.Request,
	fileKey string,
	handleFile func(fileName string, contentType string, file io.Reader) error,
) error {
	reader, err := r.MultipartReader()
	if err != nil {
		return &ParsingError{Err: err}
	}
	for {
		part, nextErr := reader.NextPart()
		if nextErr != nil {
			return multipartNextPartError(fileKey, nextErr)
		}
		if part.FormName() != fileKey {
			discardMultipartPart(part)
			continue
		}
		return handleStreamingMultipartFile(part, fileKey, handleFile)
	}
}

func multipartNextPartError(fileKey string, err error) error {
	if errors.Is(err, io.EOF) {
		return &ParsingError{Param: fileKey, Err: errors.New("multipart file field is required")}
	}
	if maxBytesErr := maxBytesErrorFromError(err); maxBytesErr != nil {
		return &ParsingError{Param: fileKey, Err: maxBytesErr}
	}
	return &ParsingError{Err: err}
}

func handleStreamingMultipartFile(
	part *multipart.Part,
	fileKey string,
	handleFile func(fileName string, contentType string, file io.Reader) error,
) error {
	fileReader := &maxBytesTrackingReader{reader: part}
	handleErr := handleFile(
		strings.TrimSpace(part.FileName()),
		strings.TrimSpace(part.Header.Get("Content-Type")),
		fileReader,
	)
	closeErr := part.Close()
	if maxBytesErr := fileReader.MaxBytesError(); maxBytesErr != nil {
		return &ParsingError{Param: fileKey, Err: maxBytesErr}
	}
	if handleErr != nil {
		return handleErr
	}
	if closeErr != nil {
		return multipartCloseError(fileKey, closeErr)
	}
	return nil
}

type maxBytesTrackingReader struct {
	reader      io.Reader
	maxBytesErr *http.MaxBytesError
}

func (r *maxBytesTrackingReader) Read(target []byte) (int, error) {
	readCount, err := r.reader.Read(target)
	if maxBytesErr := maxBytesErrorFromError(err); maxBytesErr != nil {
		r.maxBytesErr = maxBytesErr
	}
	return readCount, err
}

func (r *maxBytesTrackingReader) MaxBytesError() *http.MaxBytesError {
	return r.maxBytesErr
}

func maxBytesErrorFromError(err error) *http.MaxBytesError {
	var maxBytesErr *http.MaxBytesError
	if errors.As(err, &maxBytesErr) {
		return maxBytesErr
	}
	return nil
}

func multipartCloseError(fileKey string, err error) error {
	if maxBytesErr := maxBytesErrorFromError(err); maxBytesErr != nil {
		return &ParsingError{Param: fileKey, Err: maxBytesErr}
	}
	return &ParsingError{Param: fileKey, Err: err}
}

func discardMultipartPart(part *multipart.Part) {
	_, _ = io.Copy(io.Discard, part)
	_ = part.Close()
}
