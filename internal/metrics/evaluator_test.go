package metrics

import (
	"context"
	"testing"
	"time"
)

type fakeEvaluator struct {
	provider string
}

func (e fakeEvaluator) Provider() string { return e.provider }

func (e fakeEvaluator) Evaluate(_ context.Context, _ RuleConfig, _ time.Time) ([]Sample, error) {
	return []Sample{}, nil
}

func TestEvaluatorFactoryGet(t *testing.T) {
	factory := NewEvaluatorFactory(
		fakeEvaluator{provider: ProviderPrometheus},
	)

	_, err := factory.Get(ProviderPrometheus)
	if err != nil {
		t.Fatalf("factory.Get(prometheus) unexpected error: %v", err)
	}

	_, err = factory.Get(ProviderGCPMonitoring)
	if err == nil {
		t.Fatalf("factory.Get(gcp_monitoring) expected error")
	}
}
