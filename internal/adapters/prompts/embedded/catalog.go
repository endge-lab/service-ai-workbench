package embedded

import (
	"bytes"
	"context"
	"crypto/sha256"
	"embed"
	"encoding/hex"
	"fmt"
	"text/template"

	"github.com/endge-lab/service-ai-workbench/internal/domain/entities"
	"github.com/endge-lab/service-ai-workbench/internal/usecase/ports"
)

//go:embed templates
var templateFS embed.FS

type entry struct {
	version  string
	hash     string
	template *template.Template
}

type Catalog struct {
	entries map[entities.PromptID]entry
}

var _ ports.PromptCatalog = (*Catalog)(nil)

var paths = map[entities.PromptID]string{
	entities.PromptFinalAnswerSystem:              "templates/final-answer/v1/system.md",
	entities.PromptFinalAnswerRequest:             "templates/final-answer/v1/request.tmpl",
	entities.PromptPlannerSystem:                  "templates/planner/v1/system.md",
	entities.PromptPlannerRequest:                 "templates/planner/v1/request.tmpl",
	entities.PromptQueryExpanderSystem:            "templates/query-expander/v1/system.md",
	entities.PromptQueryExpanderRequest:           "templates/query-expander/v1/request.tmpl",
	entities.PromptRerankerSystem:                 "templates/reranker/v1/system.md",
	entities.PromptRerankerRequest:                "templates/reranker/v1/request.tmpl",
	entities.PromptClarificationClassifierSystem:  "templates/clarification-classifier/v1/system.md",
	entities.PromptClarificationClassifierRequest: "templates/clarification-classifier/v1/request.tmpl",
	entities.PromptRepairSystem:                   "templates/repair/v1/system.md",
	entities.PromptRepairRequest:                  "templates/repair/v1/request.tmpl",
}

func NewCatalog() (*Catalog, error) {
	result := &Catalog{entries: make(map[entities.PromptID]entry, len(paths))}
	for id, path := range paths {
		content, err := templateFS.ReadFile(path)
		if err != nil {
			return nil, fmt.Errorf("read prompt %s: %w", id, err)
		}
		parsed, err := template.New(string(id)).Option("missingkey=error").Parse(string(content))
		if err != nil {
			return nil, fmt.Errorf("parse prompt %s: %w", id, err)
		}
		digest := sha256.Sum256(content)
		result.entries[id] = entry{version: "v1", hash: hex.EncodeToString(digest[:]), template: parsed}
	}
	return result, nil
}

func (c *Catalog) Render(_ context.Context, id entities.PromptID, data any) (entities.RenderedPrompt, error) {
	item, ok := c.entries[id]
	if !ok {
		return entities.RenderedPrompt{}, fmt.Errorf("unknown prompt id %q", id)
	}
	var output bytes.Buffer
	if err := item.template.Execute(&output, data); err != nil {
		return entities.RenderedPrompt{}, fmt.Errorf("render prompt %s: %w", id, err)
	}
	return entities.RenderedPrompt{ID: id, Version: item.version, SHA256: item.hash, Content: output.String()}, nil
}
