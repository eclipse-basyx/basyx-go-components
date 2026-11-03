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
	"bytes"
	"errors"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"net/textproto"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestHandleMultipartFileStreamStreamsBeforeTrailingPartsArrive(t *testing.T) {
	bodyReader, bodyWriter := io.Pipe()
	writer := multipart.NewWriter(bodyWriter)
	request := httptest.NewRequest(http.MethodPut, "/attachment", bodyReader)
	request.Header.Set("Content-Type", writer.FormDataContentType())
	handlerStarted := make(chan struct{})
	trailingPartWritten := make(chan struct{})

	go func() {
		header := textproto.MIMEHeader{}
		header.Set("Content-Disposition", `form-data; name="file"; filename="photo.png"`)
		header.Set("Content-Type", "image/png")
		if part, err := writer.CreatePart(header); err == nil {
			_, _ = part.Write([]byte("payload"))
		}
		select {
		case <-handlerStarted:
		case <-time.After(5 * time.Second):
		}
		_ = writer.WriteField("fileName", "ignored.png")
		close(trailingPartWritten)
		_ = writer.Close()
		_ = bodyWriter.Close()
	}()

	err := HandleMultipartFileStream(request, "file", func(fileName string, contentType string, file io.Reader) error {
		close(handlerStarted)
		select {
		case <-trailingPartWritten:
			return errors.New("file part was buffered until the request ended")
		default:
		}
		content, readErr := io.ReadAll(file)
		require.NoError(t, readErr)
		require.Equal(t, "photo.png", fileName)
		require.Equal(t, "image/png", contentType)
		require.Equal(t, "payload", string(content))
		return nil
	})
	require.NoError(t, err)
}

func TestHandleMultipartFileStreamPassesEmptyFileNameWhenPartHasNone(t *testing.T) {
	request := newMultipartRequest(t, func(writer *multipart.Writer) {
		require.NoError(t, writer.WriteField("fileName", "ignored.txt"))
		part := createFilePart(t, writer, `form-data; name="file"`, "text/plain")
		_, err := part.Write([]byte("Hello"))
		require.NoError(t, err)
	})

	var capturedFileName, capturedContent string
	err := HandleMultipartFileStream(request, "file", func(fileName string, _ string, file io.Reader) error {
		content, readErr := io.ReadAll(file)
		capturedFileName, capturedContent = fileName, string(content)
		return readErr
	})
	require.NoError(t, err)
	require.Empty(t, capturedFileName)
	require.Equal(t, "Hello", capturedContent)
}

func TestHandleMultipartFileStreamRequiresFilePart(t *testing.T) {
	request := newMultipartRequest(t, func(writer *multipart.Writer) {
		require.NoError(t, writer.WriteField("fileName", "manual.txt"))
	})

	err := HandleMultipartFileStream(request, "file", func(string, string, io.Reader) error {
		t.Fatal("handler must not be called without a file part")
		return nil
	})
	var parsingErr *ParsingError
	require.ErrorAs(t, err, &parsingErr)
	require.Equal(t, "file", parsingErr.Param)
}

func newMultipartRequest(t *testing.T, writeParts func(*multipart.Writer)) *http.Request {
	t.Helper()
	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	writeParts(writer)
	require.NoError(t, writer.Close())
	request := httptest.NewRequest(http.MethodPut, "/attachment", &body)
	request.Header.Set("Content-Type", writer.FormDataContentType())
	return request
}

func createFilePart(t *testing.T, writer *multipart.Writer, contentDisposition string, contentType string) io.Writer {
	t.Helper()
	header := textproto.MIMEHeader{}
	header.Set("Content-Disposition", contentDisposition)
	header.Set("Content-Type", contentType)
	part, err := writer.CreatePart(header)
	require.NoError(t, err)
	return part
}
