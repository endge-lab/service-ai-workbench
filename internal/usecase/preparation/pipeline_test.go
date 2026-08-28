package preparation

import (
	"context"
	"testing"

	"github.com/endge-lab/service-ai-workbench/internal/domain/entities"
	"github.com/endge-lab/service-ai-workbench/internal/usecase/ports"
)

type promptCatalogStub struct{}

func (promptCatalogStub) Render(_ context.Context, id entities.PromptID, _ any) (entities.RenderedPrompt, error) {
	return entities.RenderedPrompt{ID: id, Version: "v1", SHA256: "hash", Content: string(id)}, nil
}

type sequenceInvoker struct {
	responses [][]byte
	calls     int
}

func (i *sequenceInvoker) Invoke(context.Context, ports.StructuredModelRequest) ([]byte, error) {
	response := i.responses[i.calls]
	i.calls++
	return response, nil
}

func TestNormalizationIsIdempotent(t *testing.T) {
	normalizer := Normalizer{}
	first := normalizer.Normalize("  ПОКАЖИ   «Тестовую Ёлку»  ")
	second := normalizer.Normalize(first.NormalizedText)
	if first.NormalizedText != second.NormalizedText || first.NormalizedText != "покажи «тестовую елку»" {
		t.Fatalf("normalization is not idempotent: %#v %#v", first, second)
	}
}

func TestPlanRejectsCyclesAndUnknownContracts(t *testing.T) {
	cycle := entities.TaskPlan{Tasks: []entities.PlannedTask{
		{ID: "a", Intent: entities.IntentFindEntity, SourceMode: entities.SourceDomain, DependsOn: []string{"b"}},
		{ID: "b", Intent: entities.IntentFindEntity, SourceMode: entities.SourceDomain, DependsOn: []string{"a"}},
	}}
	if validatePlan(cycle) == nil {
		t.Fatal("cycle must be rejected")
	}
	invalid := entities.TaskPlan{Tasks: []entities.PlannedTask{{ID: "a", Intent: "invented", SourceMode: entities.SourceNone}}}
	if validatePlan(invalid) == nil {
		t.Fatal("unknown intent must be rejected")
	}
}

func TestPlannerUsesOneRepairForInvalidStructuredOutput(t *testing.T) {
	invoker := &sequenceInvoker{responses: [][]byte{
		[]byte(`not-json`),
		[]byte(`{"tasks":[{"id":"task-1","intent":"explain_documentation","sourceMode":"documentation","mentions":[],"expectedTypes":[],"requestedAspects":[],"dependsOn":[],"confidence":1,"status":"planned"}]}`),
	}}
	planner := NewPlanner(promptCatalogStub{}, invoker)
	trace := entities.PreparationTrace{}
	plan, err := planner.Plan(context.Background(), entities.NormalizedRequest{NormalizedText: "объясни синтаксис. затем объясни документацию"}, nil, entities.RunInput{}, &modelCallBudget{limit: 2}, &trace)
	if err != nil {
		t.Fatal(err)
	}
	if invoker.calls != 2 || len(plan.Tasks) != 1 || plan.Tasks[0].Intent != entities.IntentExplainDocumentation {
		t.Fatalf("planner repair was not applied: calls=%d plan=%#v", invoker.calls, plan)
	}
}

func TestSourceRoutingStaysTaskScoped(t *testing.T) {
	plan := entities.TaskPlan{Tasks: []entities.PlannedTask{
		{ID: "docs", SourceMode: entities.SourceDocumentation},
		{ID: "domain", SourceMode: entities.SourceDomain},
		{ID: "mixed", SourceMode: entities.SourceMixed},
	}}
	routes := (SourceRouter{}).Route(plan)
	if routes["docs"] != entities.SourceDocumentation || routes["domain"] != entities.SourceDomain || routes["mixed"] != entities.SourceMixed {
		t.Fatalf("unexpected routes: %#v", routes)
	}
}

func TestFolderScopeRejectsDocumentsOutsideResolvedFolder(t *testing.T) {
	plan := entities.TaskPlan{Tasks: []entities.PlannedTask{
		{ID: "folder", ResolvedEntity: &entities.ResolvedEntity{Identity: "folder-a"}},
		{ID: "children", FolderMention: "Folder A", DependsOn: []string{"folder"}},
	}}
	inScope := entities.DomainContextMatch{Snapshot: []byte(`{"folderIdentity":"folder-a"}`)}
	outOfScope := entities.DomainContextMatch{Snapshot: []byte(`{"folderIdentity":"folder-b"}`)}
	if !matchesFolderScope(inScope, plan.Tasks[1].FolderMention, &plan.Tasks[1], plan) {
		t.Fatal("child inside resolved folder must match")
	}
	if matchesFolderScope(outOfScope, plan.Tasks[1].FolderMention, &plan.Tasks[1], plan) {
		t.Fatal("child outside resolved folder must not match")
	}
}

func TestExactResolutionDoesNotSelectFuzzyCandidate(t *testing.T) {
	matches := []entities.DomainContextMatch{
		{DocumentType: "compositions", Identity: "sample-one", DisplayName: "Sample One"},
		{DocumentType: "compositions", Identity: "sample-two", DisplayName: "Sample Two"},
	}
	if got := exactMatches(matches, normalizeText("Sample One")); len(got) != 1 || got[0].Identity != "sample-one" {
		t.Fatalf("exact display name was not resolved: %#v", got)
	}
	if got := exactMatches(matches, normalizeText("Sample")); len(got) != 0 {
		t.Fatalf("partial match must remain ambiguous: %#v", got)
	}
}

func TestRerankerCannotSelectCandidateOutsideClosedSet(t *testing.T) {
	invoker := &sequenceInvoker{responses: [][]byte{[]byte(`{"selectedCandidateId":"outside","confidence":0.99,"requiresClarification":false}`)}}
	resolver := NewResolver(promptCatalogStub{}, invoker, 5, 0.8)
	task := entities.PlannedTask{ID: "task-1", Intent: entities.IntentFindEntity, Mentions: []string{"sample"}}
	clarification, err := resolver.Resolve(context.Background(), entities.RunInput{Prompt: "find sample"}, &task, entities.DomainContext{Matches: []entities.DomainContextMatch{
		{DocumentType: "examples", Identity: "sample-a", DisplayName: "Sample A"},
		{DocumentType: "examples", Identity: "sample-b", DisplayName: "Sample B"},
	}}, &modelCallBudget{limit: 1}, &entities.PreparationTrace{})
	if err != nil {
		t.Fatal(err)
	}
	if clarification == nil || task.ResolvedEntity != nil || len(clarification.Candidates) != 2 {
		t.Fatalf("out-of-set reranker result must require clarification: %#v", clarification)
	}
}

func TestResponseValidationRejectsHallucinatedIdentity(t *testing.T) {
	prepared := entities.PreparationResult{Plan: entities.TaskPlan{Tasks: []entities.PlannedTask{{
		ID: "task-1", ResolvedEntity: &entities.ResolvedEntity{DocumentType: "compositions", Identity: "known-id"},
	}}}}
	validation := validateStructuredResponse([]byte(`{"answer":"ok","entityCitations":[{"documentType":"compositions","identity":"unknown-id"}],"documentationCitations":[],"limitations":[]}`), prepared)
	if validation.Valid || !hasFatalValidationError(validation.Errors) {
		t.Fatalf("hallucinated identity must be fatal: %#v", validation)
	}
}

func TestResponseValidationRemovesUnconfirmedDocumentationCitation(t *testing.T) {
	prepared := entities.PreparationResult{Trace: entities.PreparationTrace{Blocks: []entities.RetrievedBlock{{SourceKind: "documentation", SourceKey: "reference/example"}}}}
	validation := validateStructuredResponse([]byte(`{"answer":"ok","entityCitations":[],"documentationCitations":["reference/example","reference/unknown"],"limitations":[]}`), prepared)
	if !validation.Valid || len(validation.Response.DocumentationCitations) != 1 || validation.Response.DocumentationCitations[0] != "reference/example" {
		t.Fatalf("documentation citations were not filtered: %#v", validation)
	}
	if len(validation.Response.Limitations) == 0 {
		t.Fatal("removed citation must be disclosed as a limitation")
	}
}

func TestMandatoryContextMustFitBudget(t *testing.T) {
	all := []entities.RetrievedBlock{{SourceKind: "domain", SourceKey: "one", Mandatory: true, Content: "12345"}}
	if allMandatoryBlocksFit(all, fitBlocks(all, 4)) {
		t.Fatal("mandatory block outside budget must fail")
	}
}
