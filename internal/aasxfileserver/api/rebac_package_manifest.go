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
* SPDX-License-Identifier: MIT
******************************************************************************/

package api

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"io"

	aasx "github.com/aas-core-works/aas-package3-golang/v2"
	"github.com/eclipse-basyx/basyx-go-components/internal/aasenvironment"
	"github.com/eclipse-basyx/basyx-go-components/internal/common"
	auth "github.com/eclipse-basyx/basyx-go-components/internal/common/security"
)

func prepareReBACPackageManifest(ctx context.Context, file io.ReadSeeker) (*auth.ReBACPackageManifest, error) {
	cfg, ok := common.ConfigFromContext(ctx)
	if !ok || !cfg.ReBAC.Enabled {
		return nil, nil
	}
	if _, err := file.Seek(0, io.SeekStart); err != nil {
		return nil, common.NewInternalServerError("AASXFS-REBACMANIFEST-SEEK " + err.Error())
	}
	hash := sha256.New()
	if _, err := io.Copy(hash, file); err != nil {
		return nil, common.NewInternalServerError("AASXFS-REBACMANIFEST-HASH " + err.Error())
	}
	if _, err := file.Seek(0, io.SeekStart); err != nil {
		return nil, common.NewInternalServerError("AASXFS-REBACMANIFEST-RESEEK " + err.Error())
	}
	reader, err := aasx.NewPackaging().OpenReadFromStream(file, common.AASXLimitsFromContext(ctx).ReaderOptions()...)
	if err != nil {
		return nil, common.NewErrBadRequest("AASXFS-REBACMANIFEST-OPEN " + err.Error())
	}
	defer func() { _ = reader.Close() }()
	_, environment, err := aasenvironment.ReadEnvironmentFromAASXSpec(reader, "AASX package")
	if err != nil {
		return nil, err
	}
	members := make([]auth.ReBACPackageManifestMember, 0, len(environment.AssetAdministrationShells())+len(environment.Submodels())+len(environment.ConceptDescriptions()))
	for _, aas := range environment.AssetAdministrationShells() {
		members = append(members, auth.ReBACPackageManifestMember{Kind: "aas", Identifier: aas.ID()})
	}
	for _, submodel := range environment.Submodels() {
		members = append(members, auth.ReBACPackageManifestMember{Kind: "submodel", Identifier: submodel.ID()})
	}
	for _, conceptDescription := range environment.ConceptDescriptions() {
		members = append(members, auth.ReBACPackageManifestMember{Kind: "concept_description", Identifier: conceptDescription.ID()})
	}
	if len(members) == 0 {
		return nil, common.NewErrBadRequest("AASXFS-REBACMANIFEST-EMPTY environment has no source resources")
	}
	if _, err := file.Seek(0, io.SeekStart); err != nil {
		return nil, common.NewInternalServerError("AASXFS-REBACMANIFEST-FINALSEEK " + err.Error())
	}
	return &auth.ReBACPackageManifest{ContentSHA256: hex.EncodeToString(hash.Sum(nil)), Members: members}, nil
}
