package embedded

import (
	"context"
	"testing"

	"github.com/endge-lab/service-ai-workbench/internal/domain/entities"
)

func TestCatalogResolvesStablePromptsAndUsesStrictTemplates(t *testing.T) {
	catalog, err := NewCatalog()
	if err != nil {
		t.Fatal(err)
	}
	first, err := catalog.Render(context.Background(), entities.PromptFinalAnswerRequest, map[string]string{"PayloadJSON": `{}`})
	if err != nil {
		t.Fatal(err)
	}
	second, err := catalog.Render(context.Background(), entities.PromptFinalAnswerRequest, map[string]string{"PayloadJSON": `{}`})
	if err != nil {
		t.Fatal(err)
	}
	if first.ID != entities.PromptFinalAnswerRequest || first.Version != "v1" || first.SHA256 == "" || first.SHA256 != second.SHA256 {
		t.Fatalf("unstable prompt metadata: %#v %#v", first, second)
	}
	if _, err := catalog.Render(context.Background(), entities.PromptFinalAnswerRequest, map[string]string{}); err == nil {
		t.Fatal("missing template key must fail")
	}
}
