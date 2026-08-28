package interaction

import (
	"context"
	"errors"
	"testing"

	"github.com/endge-lab/service-ai-workbench/internal/domain/entities"
	domainerrors "github.com/endge-lab/service-ai-workbench/internal/domain/errors"
)

func TestCandidateSelectionIsRestrictedToClarificationSnapshot(t *testing.T) {
	coordinator := &Coordinator{}
	clarification := entities.Clarification{Candidates: []entities.ClarificationCandidate{{CandidateID: "candidate-a"}}}

	kind, err := coordinator.classify(context.Background(), entities.RunInput{SelectedCandidateID: "candidate-a"}, clarification)
	if err != nil || kind != "answer" {
		t.Fatalf("saved candidate must be accepted deterministically: kind=%q err=%v", kind, err)
	}
	_, err = coordinator.classify(context.Background(), entities.RunInput{SelectedCandidateID: "candidate-outside-snapshot"}, clarification)
	if !errors.Is(err, domainerrors.ErrConflict) {
		t.Fatalf("candidate outside the saved snapshot must conflict: %v", err)
	}
}

func TestFreeTextSelectsOnlyOneClosedCandidate(t *testing.T) {
	candidates := []entities.ClarificationCandidate{
		{CandidateID: "candidate-a", Identity: "sample-control-page", DisplayName: "Контроль объектов"},
		{CandidateID: "candidate-b", Identity: "sample-monitor-page", DisplayName: "Мониторинг объектов"},
	}
	selected, ok := candidateFromFreeText("Я имею в виду «Контроль объектов».", candidates)
	if !ok || selected != "candidate-a" {
		t.Fatalf("unique display name must select its closed candidate: selected=%q ok=%v", selected, ok)
	}
	if selected, ok := candidateFromFreeText("Речь об объектах", candidates); ok || selected != "" {
		t.Fatalf("ambiguous prose must not select a candidate: selected=%q ok=%v", selected, ok)
	}
}

func TestClarificationInvalidatesOnlyDependentTaskBranch(t *testing.T) {
	plan := entities.TaskPlan{Tasks: []entities.PlannedTask{
		{ID: "folder", Status: "awaiting_clarification"},
		{ID: "inside-folder", DependsOn: []string{"folder"}, Status: "resolved", ResolvedEntity: &entities.ResolvedEntity{Identity: "old-child"}},
		{ID: "independent", Status: "resolved", ResolvedEntity: &entities.ResolvedEntity{Identity: "kept"}},
	}}
	clarification := entities.Clarification{
		TaskID:     "folder",
		Candidates: []entities.ClarificationCandidate{{CandidateID: "candidate-a", DocumentType: "folders", Identity: "folder-a", DisplayName: "Folder A", Snapshot: []byte(`{"identity":"folder-a","name":"Folder A"}`)}},
	}

	applyClarification(&plan, clarification, entities.RunInput{SelectedCandidateID: "candidate-a"})

	if plan.Tasks[0].ResolvedEntity == nil || plan.Tasks[0].ResolvedEntity.Identity != "folder-a" {
		t.Fatal("clarified task was not resolved")
	}
	if len(plan.Tasks[0].ResolvedEntity.Snapshot) == 0 {
		t.Fatal("selected candidate must retain its closed snapshot for final context")
	}
	if plan.Tasks[1].ResolvedEntity != nil || plan.Tasks[1].Status != "planned" {
		t.Fatal("dependent task must be invalidated")
	}
	if plan.Tasks[2].ResolvedEntity == nil || plan.Tasks[2].ResolvedEntity.Identity != "kept" {
		t.Fatal("independent task must remain resolved")
	}
}
