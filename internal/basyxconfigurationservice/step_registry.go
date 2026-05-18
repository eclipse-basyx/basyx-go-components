package basyxconfigurationservice

import (
	"fmt"
	"log"

	"github.com/eclipse-basyx/basyx-go-components/internal/basyxconfigurationservice/steps"
)

// StepRegistry stores and executes initialization steps in sequence.
type StepRegistry struct {
	initializationSteps []steps.Step
}

// NewStepRegistry creates an empty step registry.
func NewStepRegistry() *StepRegistry {
	return &StepRegistry{initializationSteps: make([]steps.Step, 0)}
}

// Register appends a step to the execution pipeline.
func (sr *StepRegistry) Register(step steps.Step) {
	sr.initializationSteps = append(sr.initializationSteps, step)
}

// Execute runs all registered steps in order.
func (sr *StepRegistry) Execute() error {
	for idx, step := range sr.initializationSteps {
		stepIndex := idx + 1
		if err := sr.executeStep(step, stepIndex); err != nil {
			return err
		}
	}
	return nil
}

func (sr *StepRegistry) executeStep(step steps.Step, stepIndex int) error {
	log.Println(step.GetDescription(stepIndex))
	statusCode, err := step.Execute(stepIndex)
	if err != nil {
		return fmt.Errorf("BASYXCFG-REGISTRY-EXECSTEP: step %d failed with status %d: %w", stepIndex, statusCode, err)
	}
	log.Printf("[Step %d] completed with status %d\n", stepIndex, statusCode)
	return nil
}
