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

package aasenvironment

import (
	"context"
	"database/sql"
	"io"

	aastypes "github.com/FriedJannik/aas-go-sdk/types"
	aasx "github.com/aas-core-works/aas-package3-golang/v2"
	"github.com/eclipse-basyx/basyx-go-components/internal/common"
	auth "github.com/eclipse-basyx/basyx-go-components/internal/common/security"
)

type rebacImportTransactionKey struct{}
type rebacImportTransaction struct {
	tx      *sql.Tx
	members map[string]context.Context
}

func (s *uploadAPIService) processReBACAASXPackage(ctx context.Context, reader *aasx.PackageRead, spec *aasx.Part, environment aastypes.IEnvironment) (err error) {
	ctx, finish := auth.BeginReBACMutationAudit(ctx)
	defer func() { err = finish(err) }()
	return s.persistence.ExecuteInTransaction("AASENV-REBACPACKAGE-BEGIN", "AASENV-REBACPACKAGE-COMMIT", func(tx *sql.Tx) error {
		members, err := s.processReBACEnvironmentInTransaction(ctx, tx, environment)
		if err != nil {
			return err
		}
		ctx = context.WithValue(ctx, rebacImportTransactionKey{}, rebacImportTransaction{tx: tx, members: members})
		if err = s.storeAASXThumbnail(ctx, reader, spec, environment); err != nil {
			return err
		}
		return s.uploadSupplementaryFiles(ctx, reader, spec, environment)
	})
}

func (s *uploadAPIService) storeImportedThumbnail(ctx context.Context, id, name string, reader io.Reader) error {
	transaction, ok := ctx.Value(rebacImportTransactionKey{}).(rebacImportTransaction)
	if !ok {
		return s.persistence.AASRepository.PutThumbnailByAASIDReader(ctx, id, name, reader)
	}
	member, ok := transaction.members[rebacEnvironmentResourceContextKey("aas", id)]
	if !ok {
		return common.NewErrDenied("REBAC-IMPORT-THUMBNAIL thumbnail is outside the authorized inclusion set")
	}
	return s.persistence.AASRepository.PutThumbnailByAASIDReaderInTransaction(member, transaction.tx, id, name, reader)
}

func (s *uploadAPIService) storeImportedSupplementary(ctx context.Context, id, path string, reader io.Reader, name, contentType, fallbackContentType string) error {
	transaction, ok := ctx.Value(rebacImportTransactionKey{}).(rebacImportTransaction)
	if !ok {
		return s.persistence.SubmodelRepository.UploadFileAttachmentReaderWithHistory(ctx, id, path, reader, name, contentType, fallbackContentType)
	}
	member, ok := transaction.members[rebacEnvironmentResourceContextKey("submodel", id)]
	if !ok {
		return common.NewErrDenied("REBAC-IMPORT-SUPPLEMENTARY attachment is outside the authorized inclusion set")
	}
	return s.persistence.SubmodelRepository.UploadFileAttachmentReaderWithHistoryInTransaction(member, transaction.tx, id, path, reader, name, contentType, fallbackContentType)
}
