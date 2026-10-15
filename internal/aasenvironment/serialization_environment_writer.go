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
	"bufio"
	"encoding/json"
	"encoding/xml"
	"errors"
	"io"

	"github.com/FriedJannik/aas-go-sdk/jsonization"
	aastypes "github.com/FriedJannik/aas-go-sdk/types"
	aasxmlization "github.com/FriedJannik/aas-go-sdk/xmlization"
	"github.com/eclipse-basyx/basyx-go-components/internal/common"
)

const (
	environmentSectionAssetAdministrationShells = "assetAdministrationShells"
	environmentSectionSubmodels                 = "submodels"
	environmentSectionConceptDescriptions       = "conceptDescriptions"

	// environmentStreamChunkSize matches the staging copy buffer so that each
	// staged database write receives a full chunk.
	environmentStreamChunkSize = 32 << 10
)

// environmentStreamWriter serializes an AAS environment one identifiable at a
// time so that memory use is bounded by the largest single identifiable
// instead of the whole environment.
//
// Identifiables must be written grouped by section in the order
// assetAdministrationShells, submodels, conceptDescriptions. Empty sections are
// omitted as required by the AAS JSON and XML schemas.
type environmentStreamWriter interface {
	writeIdentifiable(section string, instance aastypes.IClass) error
	close() error
}

func newEnvironmentStreamWriter(specContentType string, destination io.Writer) (environmentStreamWriter, error) {
	buffered := bufio.NewWriterSize(destination, environmentStreamChunkSize)
	switch specContentType {
	case serializationContentTypeJSON:
		return &jsonEnvironmentStreamWriter{destination: buffered}, nil
	case serializationContentTypeXML:
		encoder := xml.NewEncoder(flushIsolatedWriter{buffered})
		encoder.Indent("", "\t")
		return &xmlEnvironmentStreamWriter{destination: buffered, encoder: encoder}, nil
	}
	return nil, common.NewInternalServerError("AASENV-NEWENVWRITER-UNSUPPORTED unsupported environment content type")
}

// flushIsolatedWriter hides the chunk buffer from xml.NewEncoder, which would
// otherwise reuse it and pass every per-property SDK flush on to the stager.
type flushIsolatedWriter struct {
	io.Writer
}

type jsonEnvironmentStreamWriter struct {
	destination *bufio.Writer
	started     bool
	section     string
}

func (w *jsonEnvironmentStreamWriter) writeIdentifiable(section string, instance aastypes.IClass) error {
	jsonable, err := jsonization.ToJsonable(instance)
	if err != nil {
		return environmentWriterError("AASENV-JSONENVWRITER-TOJSONABLE", err)
	}
	payload, err := json.Marshal(jsonable)
	if err != nil {
		return environmentWriterError("AASENV-JSONENVWRITER-MARSHAL", err)
	}
	if _, err = w.destination.WriteString(w.separatorFor(section)); err != nil {
		return err
	}
	_, err = w.destination.Write(payload)
	return err
}

func (w *jsonEnvironmentStreamWriter) separatorFor(section string) string {
	separator := ""
	if !w.started {
		w.started = true
		separator = "{"
	}
	if section == w.section {
		return separator + ","
	}
	if w.section != "" {
		separator += "],"
	}
	w.section = section
	return separator + `"` + section + `":[`
}

func (w *jsonEnvironmentStreamWriter) close() error {
	closing := "}"
	if !w.started {
		closing = "{}"
	} else if w.section != "" {
		closing = "]}"
	}
	if _, err := w.destination.WriteString(closing); err != nil {
		return err
	}
	return w.destination.Flush()
}

type xmlEnvironmentStreamWriter struct {
	destination *bufio.Writer
	encoder     *xml.Encoder
	started     bool
	section     string
}

func (w *xmlEnvironmentStreamWriter) writeIdentifiable(section string, instance aastypes.IClass) error {
	if err := w.enterSection(section); err != nil {
		return err
	}
	if err := aasxmlization.Marshal(w.encoder, instance, false); err != nil {
		return environmentWriterError("AASENV-XMLENVWRITER-MARSHAL", err)
	}
	return nil
}

func (w *xmlEnvironmentStreamWriter) enterSection(section string) error {
	if err := w.start(); err != nil {
		return err
	}
	if section == w.section {
		return nil
	}
	if err := w.leaveSection(); err != nil {
		return err
	}
	w.section = section
	return w.encoder.EncodeToken(xml.StartElement{Name: xml.Name{Local: section}})
}

func (w *xmlEnvironmentStreamWriter) leaveSection() error {
	if w.section == "" {
		return nil
	}
	return w.encoder.EncodeToken(xml.EndElement{Name: xml.Name{Local: w.section}})
}

func (w *xmlEnvironmentStreamWriter) start() error {
	if w.started {
		return nil
	}
	w.started = true
	return w.encoder.EncodeToken(xml.StartElement{Name: xmlEnvironmentName()})
}

func (w *xmlEnvironmentStreamWriter) close() error {
	if err := w.start(); err != nil {
		return err
	}
	if err := w.leaveSection(); err != nil {
		return err
	}
	if err := w.encoder.EncodeToken(xml.EndElement{Name: xmlEnvironmentName()}); err != nil {
		return err
	}
	if err := w.encoder.Flush(); err != nil {
		return err
	}
	return w.destination.Flush()
}

func xmlEnvironmentName() xml.Name {
	return xml.Name{Space: aasxmlization.Namespace, Local: "environment"}
}

// environmentWriterError codes serialization failures but keeps a closed pipe
// recognizable, since it only signals that the consumer stopped reading.
func environmentWriterError(code string, err error) error {
	if errors.Is(err, io.ErrClosedPipe) {
		return err
	}
	return common.NewInternalServerError(code + " " + err.Error())
}
