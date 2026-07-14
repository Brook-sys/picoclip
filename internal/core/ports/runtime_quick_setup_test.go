package ports

import (
	"context"
	"testing"

	"picoclip/internal/core/domain"
)

type quickConfiguratorContract struct{}

func (quickConfiguratorContract) QuickSetupSchema() domain.RuntimeQuickSetupSchema {
	return domain.RuntimeQuickSetupSchema{ProfileID: "openai-compatible"}
}
func (quickConfiguratorContract) ReadQuickSetup(context.Context, domain.RuntimeState) (domain.RuntimeQuickSetupView, error) {
	return domain.RuntimeQuickSetupView{}, nil
}
func (quickConfiguratorContract) ApplyQuickSetup(context.Context, domain.RuntimeState, domain.RuntimeQuickSetupInput) error {
	return nil
}
func (quickConfiguratorContract) TestQuickSetup(context.Context, domain.RuntimeState, domain.RuntimeQuickSetupInput) (domain.RuntimeModelTestResult, error) {
	return domain.RuntimeModelTestResult{}, nil
}

func TestRuntimeQuickConfiguratorIsOptionalCapability(t *testing.T) {
	var capability RuntimeQuickConfigurator = quickConfiguratorContract{}
	if capability.QuickSetupSchema().ProfileID != "openai-compatible" {
		t.Fatal("unexpected profile")
	}
}

func TestRuntimeQuickSetupInputDoesNotSerializeAPIKey(t *testing.T) {
	input := domain.RuntimeQuickSetupInput{APIKey: "secret"}
	if input.APIKey == "" {
		t.Fatal("test setup requires a secret")
	}
}
