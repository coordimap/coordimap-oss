package metrics

import (
	"context"
	"fmt"
	"time"
)

// Sample is a normalized metric sample emitted by provider evaluators.
type Sample struct {
	Value          float64
	Labels         map[string]string
	ResourceLabels map[string]string
}

// Evaluator evaluates one metric rule against one provider backend.
type Evaluator interface {
	Provider() string
	Evaluate(ctx context.Context, rule RuleConfig, evalTime time.Time) ([]Sample, error)
}

// EvaluatorFactory resolves a provider evaluator using strategy lookup by provider name.
type EvaluatorFactory struct {
	evaluators map[string]Evaluator
}

// NewEvaluatorFactory creates a new evaluator factory.
func NewEvaluatorFactory(evaluators ...Evaluator) *EvaluatorFactory {
	byProvider := map[string]Evaluator{}
	for _, evaluator := range evaluators {
		if evaluator == nil {
			continue
		}

		byProvider[evaluator.Provider()] = evaluator
	}

	return &EvaluatorFactory{evaluators: byProvider}
}

// Get returns an evaluator for a provider.
func (factory *EvaluatorFactory) Get(provider string) (Evaluator, error) {
	evaluator, exists := factory.evaluators[provider]
	if !exists {
		return nil, fmt.Errorf("no evaluator configured for provider %q", provider)
	}

	return evaluator, nil
}
