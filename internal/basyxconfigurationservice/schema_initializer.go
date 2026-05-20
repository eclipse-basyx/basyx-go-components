// Package basyxconfigurationservice contains orchestration for configuration startup steps.
package basyxconfigurationservice

import (
	"fmt"
	"log"

	"github.com/eclipse-basyx/basyx-go-components/internal/basyxconfigurationservice/sequences"
)

// SchemaInitializer stores and executes initialization steps in sequence.
type SchemaInitializer struct {
	initializationSteps []steps.Sequence
}

// NewSchemaInitializer creates an empty step registry.
func NewSchemaInitializer() *SchemaInitializer {
	return &SchemaInitializer{initializationSteps: make([]steps.Sequence, 0)}
}

// Register appends a step to the execution pipeline.
func (sr *SchemaInitializer) Register(step steps.Sequence) {
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

func (sr *SchemaInitializer) executeSequence(step steps.Sequence, sequenceIndex int) error {
	log.Println(step.GetDescription(sequenceIndex))
	statusCode, err := step.Execute(sequenceIndex)
	if err != nil {
		return fmt.Errorf("BASYXCFG-INIT-EXECSTEP: step %d failed with status %d: %w", sequenceIndex, statusCode, err)
	}
	log.Printf("[Step %d] completed with status %d\n", sequenceIndex, statusCode)
	return nil
}
