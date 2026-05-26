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

// Package basyxconfigurationservice contains orchestration for configuration startup sequences.
package basyxconfigurationservice

import (
	"fmt"
	"log"

	"github.com/eclipse-basyx/basyx-go-components/internal/basyxconfigurationservice/sequences"
)

// SchemaInitializer stores and executes initialization steps in sequence.
type SchemaInitializer struct {
	initializationSteps []sequences.Sequence
}

// NewSchemaInitializer creates an empty step registry.
func NewSchemaInitializer() *SchemaInitializer {
	return &SchemaInitializer{initializationSteps: make([]sequences.Sequence, 0)}
}

// Register appends a step to the execution pipeline.
func (sr *SchemaInitializer) Register(step sequences.Sequence) {
	sr.initializationSteps = append(sr.initializationSteps, step)
}

// Execute runs all registered steps in order.
func (sr *SchemaInitializer) Execute() error {
	for idx, step := range sr.initializationSteps {
		stepIndex := idx + 1
		if err := sr.executeSequence(step, stepIndex); err != nil {
			return err
		}
	}
	return nil
}

func (sr *SchemaInitializer) executeSequence(step sequences.Sequence, sequenceIndex int) error {
	log.Println(step.GetDescription(sequenceIndex))
	statusCode, err := step.Execute(sequenceIndex)
	if err != nil {
		return fmt.Errorf("BASYXCFG-INIT-EXECSTEP: step %d failed with status %d: %w", sequenceIndex, statusCode, err)
	}
	log.Printf("[Step %d] completed with status %d\n", sequenceIndex, statusCode)
	return nil
}
