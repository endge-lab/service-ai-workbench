package snapshot

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/endge-lab/service-ai-workbench/internal/config"
	"github.com/endge-lab/service-ai-workbench/internal/domain/entities"
)

func TestSelectorRestrictsTypeAndFolderBeforeLimiting(t *testing.T) {
	snapshot, err := json.Marshal(portableSnapshot{
		Kind: workspaceSnapshotKind, SchemaVersion: supportedSchema,
		Documents: map[string][]json.RawMessage{
			"actions": {json.RawMessage(`{"identity":"sample-page","displayName":"Sample page"}`)},
			"compositions": {
				json.RawMessage(`{"identity":"page-a","displayName":"Page A","folderIdentity":"folder-a","source":"large source"}`),
				json.RawMessage(`{"identity":"page-b","displayName":"Page B","folderIdentity":"folder-b"}`),
			},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	selector := NewSelector(&config.Config{Context: config.ContextConfig{DomainMaxResults: 20}})
	result := selector.Select(context.Background(), entities.DomainSelectionInput{
		WorkspaceID: "workspace", Generation: "1", SnapshotSHA256: "hash", Snapshot: snapshot,
		ExpectedTypes: []string{"compositions"}, FolderIdentity: "folder-a", IncludeAll: true,
	})
	if !result.Available || result.TotalDocuments != 1 || len(result.Matches) != 1 || result.Matches[0].Identity != "page-a" {
		t.Fatalf("unexpected selection: %#v", result)
	}
	if len(result.Matches[0].Summary) == 0 || string(result.Matches[0].Summary) == string(result.Matches[0].Snapshot) || containsJSONField(result.Matches[0].Summary, "source") {
		t.Fatalf("summary is missing or contains full document fields: summary=%s snapshot=%s", result.Matches[0].Summary, result.Matches[0].Snapshot)
	}
}

func containsJSONField(raw json.RawMessage, field string) bool {
	var value map[string]any
	_ = json.Unmarshal(raw, &value)
	_, exists := value[field]
	return exists
}

func TestSelectorKeepsExactDisplayNameInsideExpectedType(t *testing.T) {
	snapshot := []byte(`{"kind":"workspace-snapshot","schemaVersion":1,"documents":{"actions":[{"identity":"other","displayName":"Shared name"}],"compositions":[{"identity":"page","displayName":"Shared name"}]}}`)
	selector := NewSelector(&config.Config{Context: config.ContextConfig{DomainMaxResults: 5}})
	result := selector.Select(context.Background(), entities.DomainSelectionInput{
		WorkspaceID: "workspace", Generation: "1", SnapshotSHA256: "hash", Snapshot: snapshot,
		ExpectedTypes: []string{"compositions"}, Query: entities.KnowledgeSearchQuery{Terms: []string{"shared", "name"}, Phrases: []string{"shared name"}},
	})
	if len(result.Matches) != 1 || result.Matches[0].DocumentType != "compositions" {
		t.Fatalf("type restriction was applied too late: %#v", result.Matches)
	}
}

func TestSelectorReturnsFuzzyDisplayNameCandidateWithoutSelectingIt(t *testing.T) {
	snapshot := []byte(`{"kind":"workspace-snapshot","schemaVersion":1,"documents":{"compositions":[{"identity":"sample-page","displayName":"Тестовая страница"}]}}`)
	selector := NewSelector(&config.Config{Context: config.ContextConfig{DomainMaxResults: 5}})
	result := selector.Select(context.Background(), entities.DomainSelectionInput{
		WorkspaceID: "workspace", Generation: "1", SnapshotSHA256: "hash", Snapshot: snapshot,
		ExpectedTypes: []string{"compositions"}, Query: entities.KnowledgeSearchQuery{NormalizedPrompt: "тестовая странца", Terms: []string{"тестовая", "странца"}, Phrases: []string{"тестовая странца"}},
	})
	if len(result.Matches) != 1 || result.Matches[0].Identity != "sample-page" || result.Matches[0].Score <= 0 {
		t.Fatalf("fuzzy candidate was not recalled: %#v", result.Matches)
	}
}
