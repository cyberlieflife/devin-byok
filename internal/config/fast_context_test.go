package config

import "testing"

func TestFeatureModelIDFastContext(t *testing.T) {
	f := &File{}
	f.Upstream.Models = []ModelEntry{
		{ID: "grok-4.5-byok-medium", Label: "med", UpstreamModel: "grok"},
		{ID: "grok-4.5-byok-high", Label: "high", UpstreamModel: "grok"},
	}
	f.Features.FastContextModel = "grok-4.5-byok-high"
	f.applyDefaults()
	got := f.FeatureModelID("fast_context")
	if got != "grok-4.5-byok-high" {
		t.Fatalf("got %s", got)
	}
	got2 := f.FeatureModelID("find_code_context")
	if got2 != "grok-4.5-byok-high" {
		t.Fatalf("alias got %s", got2)
	}
}
