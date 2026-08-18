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
* MERCHANTABILITY, FITNESS FOR A PARTICULAR PURPOSE AND NONINFRINGEMENT.
* IN NO EVENT SHALL THE AUTHORS OR COPYRIGHT HOLDERS BE LIABLE FOR ANY
* CLAIM, DAMAGES OR OTHER LIABILITY, WHETHER IN AN ACTION OF CONTRACT,
* TORT OR OTHERWISE, ARISING FROM, OUT OF OR IN CONNECTION WITH THE
* SOFTWARE OR THE USE OR OTHER DEALINGS IN THE SOFTWARE.
*
* SPDX-License-Identifier: MIT
******************************************************************************/

package api

import (
	"context"
	"database/sql"
	"errors"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/eclipse-basyx/basyx-go-components/internal/aasxfileserver/persistence"
	openapi "github.com/eclipse-basyx/basyx-go-components/pkg/aasxfileserverapi/go"
	"github.com/stretchr/testify/require"
)

const (
	aasxFileServerAPIFunctionPrefix         = "github.com/eclipse-basyx/basyx-go-components/internal/aasxfileserver/api."
	aasxFileServerPersistenceFunctionPrefix = "github.com/eclipse-basyx/basyx-go-components/internal/aasxfileserver/persistence."
)

var errAsyncPromotionIntercepted = errors.New("async staged upload promotion intercepted")

type asyncPackagePoster interface {
	PostAsyncAASXPackage(context.Context, openapi.StagedUpload, []string) (openapi.ImplResponse, error)
}

type workerReadTrackingUpload struct {
	io.ReadSeeker
	size         int64
	promoted     chan struct{}
	closed       chan struct{}
	promoteOnce  sync.Once
	closeOnce    sync.Once
	readByWorker atomic.Bool
}

func (upload *workerReadTrackingUpload) Read(destination []byte) (int, error) {
	if readOriginatesInAsyncWorker() {
		upload.readByWorker.Store(true)
	}
	return upload.ReadSeeker.Read(destination)
}

func (upload *workerReadTrackingUpload) Size() int64 {
	return upload.size
}

func (upload *workerReadTrackingUpload) Promote(context.Context, func(context.Context, *sql.Tx, int64, int64) error) error {
	upload.promoteOnce.Do(func() { close(upload.promoted) })
	return errAsyncPromotionIntercepted
}

func (upload *workerReadTrackingUpload) Close() error {
	upload.closeOnce.Do(func() { close(upload.closed) })
	return nil
}

func TestAsyncPackageWorkerHandsStagedUploadToPersistenceWithoutPreReading(t *testing.T) {
	filePath := filepath.Clean("../../aasenvironment/integration_tests/testdata/IESEDriveMotorDM3000.aasx")
	file, err := os.Open(filePath)
	require.NoError(t, err)
	defer func() { require.NoError(t, file.Close()) }()
	fileInfo, err := file.Stat()
	require.NoError(t, err)

	database, databaseMock, err := sqlmock.New()
	require.NoError(t, err)
	databaseMock.ExpectClose()
	defer func() { require.NoError(t, database.Close()) }()
	backend, err := persistence.NewAASXFileServerDatabaseFromDB(database)
	require.NoError(t, err)
	service := NewAASXFileServerAPIAPIService(backend)
	asyncService, implemented := any(service).(asyncPackagePoster)
	require.True(t, implemented, "AASX service must implement the generated async package API")

	upload := &workerReadTrackingUpload{
		ReadSeeker: file,
		size:       fileInfo.Size(),
		promoted:   make(chan struct{}),
		closed:     make(chan struct{}),
	}
	response, err := asyncService.PostAsyncAASXPackage(t.Context(), upload, nil)
	require.NoError(t, err)
	require.Equal(t, http.StatusAccepted, response.Code)

	select {
	case <-upload.promoted:
	case <-time.After(5 * time.Second):
		t.Fatal("async worker did not hand the staged upload to persistence")
	}
	require.False(t, upload.readByWorker.Load(), "async worker read the staged package before persistence received it")
	select {
	case <-upload.closed:
	case <-time.After(5 * time.Second):
		t.Fatal("async worker did not release the staged upload after persistence failed")
	}
}

func readOriginatesInAsyncWorker() bool {
	callers := make([]uintptr, 32)
	count := runtime.Callers(2, callers)
	frames := runtime.CallersFrames(callers[:count])
	for {
		frame, more := frames.Next()
		if strings.Contains(frame.Function, "workerReadTrackingUpload") || strings.Contains(frame.Function, "readOriginatesInAsyncWorker") {
			if !more {
				return false
			}
			continue
		}
		if strings.HasPrefix(frame.Function, aasxFileServerPersistenceFunctionPrefix) {
			return false
		}
		if strings.HasPrefix(frame.Function, aasxFileServerAPIFunctionPrefix) {
			return true
		}
		if !more {
			return false
		}
	}
}
