package llm_test

import (
	"testing"

	"github.com/duragraph/duragraph-go/llm"
)

func TestRequestConfig(t *testing.T) {
	cfg := &llm.RequestConfig{}

	llm.WithModel("gpt-4o")(cfg)
	llm.WithTemperature(0.5)(cfg)
	llm.WithMaxTokens(100)(cfg)
	llm.WithTopP(0.9)(cfg)
	llm.WithStop([]string{"END"})(cfg)
	llm.WithTools([]llm.Tool{{Name: "search"}})(cfg)

	if cfg.Model != "gpt-4o" {
		t.Errorf("Model = %q, want gpt-4o", cfg.Model)
	}
	if cfg.Temperature != 0.5 {
		t.Errorf("Temperature = %f, want 0.5", cfg.Temperature)
	}
	if cfg.MaxTokens != 100 {
		t.Errorf("MaxTokens = %d, want 100", cfg.MaxTokens)
	}
	if cfg.TopP != 0.9 {
		t.Errorf("TopP = %f, want 0.9", cfg.TopP)
	}
	if len(cfg.Stop) != 1 || cfg.Stop[0] != "END" {
		t.Errorf("Stop = %v, want [END]", cfg.Stop)
	}
	if len(cfg.Tools) != 1 || cfg.Tools[0].Name != "search" {
		t.Errorf("Tools = %v, want [{search}]", cfg.Tools)
	}
}

func TestRegistryProviderForModel(t *testing.T) {
	called := false
	llm.RegisterProvider("test-model-", func() llm.Provider {
		called = true
		return nil
	})

	p, ok := llm.ProviderForModel("test-model-v1")
	if !ok {
		t.Fatal("expected provider to be found")
	}
	if !called {
		t.Fatal("expected factory to be called")
	}
	_ = p

	_, ok = llm.ProviderForModel("nonexistent-model")
	if ok {
		t.Fatal("expected no provider for unknown model")
	}
}
