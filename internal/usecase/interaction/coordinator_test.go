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

func TestClarificationInvalidatesOnlyDependentTaskBranch(t *testing.T) {
	plan := entities.TaskPlan{Tasks: []entities.PlannedTask{
		{ID: "folder", Status: "awaiting_clarification"},
		{ID: "inside-folder", DependsOn: []string{"folder"}, Status: "resolved", ResolvedEntity: &entities.ResolvedEntity{Identity: "old-child"}},
		{ID: "independent", Status: "resolved", ResolvedEntity: &entities.ResolvedEntity{Identity: "kept"}},
	}}
	clarification := entities.Clarification{
		TaskID:     "folder",
		Candidates: []entities.ClarificationCandidate{{CandidateID: "candidate-a", DocumentType: "folders", Identity: "folder-a", DisplayName: "Folder A"}},
	}

	applyClarification(&plan, clarification, entities.RunInput{SelectedCandidateID: "candidate-a"})

	if plan.Tasks[0].ResolvedEntity == nil || plan.Tasks[0].ResolvedEntity.Identity != "folder-a" {
		t.Fatal("clarified task was not resolved")
	}
	if plan.Tasks[1].ResolvedEntity != nil || plan.Tasks[1].Status != "planned" {
		t.Fatal("dependent task must be invalidated")
	}
	if plan.Tasks[2].ResolvedEntity == nil || plan.Tasks[2].ResolvedEntity.Identity != "kept" {
		t.Fatal("independent task must remain resolved")
	}
}
