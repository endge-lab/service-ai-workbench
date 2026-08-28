package preparation

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"strings"
	"unicode"

	"github.com/endge-lab/service-ai-workbench/internal/config"
	"github.com/endge-lab/service-ai-workbench/internal/domain/entities"
	"github.com/endge-lab/service-ai-workbench/internal/usecase/ports"
)

type Input struct {
	RunInput     entities.RunInput
	Interaction  entities.Interaction
	Conversation entities.ConversationContext
}

type Coordinator struct {
	normalizer Normalizer
	planner    Planner
	router     SourceRouter
	resolver   Resolver
	adequacy   ContextAdequacyValidator
	assembler  ModelRequestAssembler
	knowledge  ports.KnowledgeRetriever
	domain     ports.DomainContextSelector
	prompts    ports.PromptCatalog
	models     ports.StructuredModelInvoker
	maxCalls   int
}

func NewCoordinator(
	knowledge ports.KnowledgeRetriever,
	domain ports.DomainContextSelector,
	prompts ports.PromptCatalog,
	models ports.StructuredModelInvoker,
	cfg *config.Config,
) *Coordinator {
	return &Coordinator{
		normalizer: Normalizer{}, planner: NewPlanner(prompts, models), router: SourceRouter{},
		resolver: NewResolver(prompts, models, cfg.Preparation.MaxCandidates, cfg.Preparation.RerankerMinConfidence),
		adequacy: NewContextAdequacyValidator(cfg.Context.ModelMaxChars), assembler: NewModelRequestAssembler(prompts),
		knowledge: knowledge, domain: domain, prompts: prompts, models: models, maxCalls: cfg.Preparation.MaxModelCalls,
	}
}

func (c *Coordinator) Prepare(ctx context.Context, input Input) (entities.PreparationResult, entities.KnowledgeRetrieval, entities.DomainContext, error) {
	trace := entities.PreparationTrace{Routing: map[string]string{}, Blocks: []entities.RetrievedBlock{}, Warnings: []string{}, PromptUsage: []entities.PromptUsage{}}
	trace.Normalized = c.normalizer.Normalize(input.RunInput.Prompt)
	budget := &modelCallBudget{limit: c.maxCalls}

	plan := input.Interaction.Plan
	var err error
	if len(plan.Tasks) == 0 {
		plan, err = c.planner.Plan(ctx, trace.Normalized, input.Conversation.Messages, input.RunInput, budget, &trace)
		if err != nil {
			return entities.PreparationResult{}, entities.KnowledgeRetrieval{}, entities.DomainContext{}, err
		}
	}
	trace.Plan = plan
	for taskID, source := range c.router.Route(plan) {
		trace.Routing[taskID] = string(source)
	}
	if containsUnsupported(plan) {
		return entities.PreparationResult{
			Status: entities.PreparationUnsupported, Plan: plan, Trace: trace,
			UnsupportedMessage: localized(input.RunInput.Prompt,
				"Сейчас я умею объяснять документацию, находить и показывать сущности Workspace, перечислять сущности и учитывать папку как область поиска.",
				"I can currently explain documentation, find and inspect Workspace entities, list entities, and apply a folder scope."),
		}, entities.KnowledgeRetrieval{}, entities.DomainContext{}, nil
	}

	var knowledge entities.KnowledgeRetrieval
	var domainContext entities.DomainContext
	for taskIndex := range plan.Tasks {
		task := &plan.Tasks[taskIndex]
		if usesDocumentation(task.SourceMode) {
			retrieved, err := c.retrieveDocumentation(ctx, input.RunInput, *task, budget, &trace)
			if err != nil {
				return entities.PreparationResult{}, knowledge, domainContext, err
			}
			knowledge = mergeKnowledge(knowledge, retrieved)
			for matchIndex, match := range retrieved.Matches {
				trace.Blocks = append(trace.Blocks, entities.RetrievedBlock{
					SourceKind: "documentation", SourceKey: match.ChunkID, TaskIDs: []string{task.ID}, Score: float64(match.Score),
					Mandatory: matchIndex == 0, Content: match.Content,
				})
			}
		}
		if usesDomain(task.SourceMode) {
			if task.ResolvedEntity != nil && input.Interaction.WorkspaceSnapshotSHA256 == input.RunInput.SnapshotSHA256 && len(task.ResolvedEntity.Snapshot) > 0 {
				task.Status = "resolved"
				c.addDomainBlocks(task, entities.DomainContext{}, plan, &trace)
				continue
			}
			queryText := strings.Join(task.Mentions, " ")
			if task.ResolvedEntity != nil {
				queryText = task.ResolvedEntity.Identity
			} else if folderIdentity := resolvedFolderIdentity(*task, plan); folderIdentity != "" {
				queryText = folderIdentity
			}
			selected := c.domain.Select(ctx, entities.DomainSelectionInput{
				WorkspaceID: input.RunInput.Workspace.ID, Generation: input.RunInput.Generation,
				SnapshotSHA256: input.RunInput.SnapshotSHA256, Snapshot: input.RunInput.Snapshot, Query: domainSearchQuery(queryText),
			})
			domainContext = mergeDomain(domainContext, selected)
			clarification, err := c.resolver.Resolve(ctx, input.RunInput, task, selected, budget, &trace)
			if err != nil {
				return entities.PreparationResult{}, knowledge, domainContext, err
			}
			if clarification != nil {
				trace.Plan = plan
				trace.ModelCalls = budget.used
				return entities.PreparationResult{Status: entities.PreparationNeedsClarification, Plan: plan, Clarification: clarification, Trace: trace}, knowledge, domainContext, nil
			}
			c.addDomainBlocks(task, selected, plan, &trace)
		}
	}
	trace.Plan = plan
	trace.ModelCalls = budget.used
	overheadChars, err := c.assembler.OverheadChars(ctx, input, plan)
	if err != nil {
		return entities.PreparationResult{}, knowledge, domainContext, err
	}
	fittedBlocks, clarification := c.adequacy.Validate(input.RunInput.Prompt, plan, trace.Blocks, overheadChars)
	if clarification != nil {
		return entities.PreparationResult{Status: entities.PreparationNeedsClarification, Plan: plan, Clarification: clarification, Trace: trace}, knowledge, domainContext, nil
	}
	trace.Blocks = fittedBlocks

	request, usages, err := c.assembler.Assemble(ctx, input, plan, fittedBlocks)
	if err != nil {
		return entities.PreparationResult{}, knowledge, domainContext, err
	}
	trace.PromptUsage = append(trace.PromptUsage, usages...)
	return entities.PreparationResult{Status: entities.PreparationReady, Plan: plan, ModelRequest: &request, Trace: trace}, knowledge, domainContext, nil
}

func (c *Coordinator) retrieveDocumentation(ctx context.Context, input entities.RunInput, task entities.PlannedTask, budget *modelCallBudget, trace *entities.PreparationTrace) (entities.KnowledgeRetrieval, error) {
	query := strings.Join(task.Mentions, " ")
	if query == "" {
		query = input.Prompt
	}
	result := c.knowledge.Retrieve(ctx, query, 0)
	if len(result.Matches) > 0 || !result.Available {
		return result, nil
	}
	payload, err := json.Marshal(map[string]any{"request": input.Prompt, "task": task})
	if err != nil {
		return result, err
	}
	raw, usages, err := invokeStructured(ctx, c.prompts, c.models, input, entities.PromptQueryExpanderSystem, entities.PromptQueryExpanderRequest, payload, budget)
	trace.PromptUsage = append(trace.PromptUsage, usages...)
	trace.ModelCalls = budget.used
	if err != nil {
		trace.Warnings = append(trace.Warnings, "query expansion skipped: "+err.Error())
		return result, nil
	}
	var expanded struct {
		Queries []string `json:"queries"`
	}
	if json.Unmarshal(extractJSONObject(raw), &expanded) != nil || len(expanded.Queries) == 0 {
		return result, nil
	}
	if len(expanded.Queries) > 5 {
		expanded.Queries = expanded.Queries[:5]
	}
	return c.knowledge.Retrieve(ctx, strings.Join(expanded.Queries, " "), 0), nil
}

func (c *Coordinator) addDomainBlocks(task *entities.PlannedTask, domain entities.DomainContext, plan entities.TaskPlan, trace *entities.PreparationTrace) {
	if task.ResolvedEntity != nil {
		content, _ := json.Marshal(task.ResolvedEntity)
		trace.Blocks = append(trace.Blocks, entities.RetrievedBlock{SourceKind: "domain", SourceKey: task.ResolvedEntity.DocumentType + "/" + task.ResolvedEntity.Identity, TaskIDs: []string{task.ID}, Score: 100, Mandatory: true, Content: string(content)})
		return
	}
	for _, match := range domain.Matches {
		if !matchesFolderScope(match, task.FolderMention, task, plan) {
			continue
		}
		trace.Blocks = append(trace.Blocks, entities.RetrievedBlock{SourceKind: "domain", SourceKey: match.DocumentType + "/" + match.Identity, TaskIDs: []string{task.ID}, Score: float64(match.Score), Mandatory: task.Intent == entities.IntentListEntities, Content: string(match.Snapshot)})
	}
}

func containsUnsupported(plan entities.TaskPlan) bool {
	for _, task := range plan.Tasks {
		if task.Intent == entities.IntentUnsupported {
			return true
		}
	}
	return false
}

func exactMatches(matches []entities.DomainContextMatch, mention string) []entities.DomainContextMatch {
	result := make([]entities.DomainContextMatch, 0)
	for _, match := range matches {
		if normalizeText(match.Identity) == mention || normalizeText(match.DisplayName) == mention {
			result = append(result, match)
		}
	}
	return result
}

func filterExpectedTypes(matches []entities.DomainContextMatch, expected []string) []entities.DomainContextMatch {
	if len(expected) == 0 {
		return matches
	}
	result := make([]entities.DomainContextMatch, 0)
	for _, match := range matches {
		for _, expectedType := range expected {
			if normalizeText(match.DocumentType) == normalizeText(expectedType) || strings.Contains(normalizeText(match.DocumentType), strings.TrimSuffix(normalizeText(expectedType), "s")) {
				result = append(result, match)
				break
			}
		}
	}
	return result
}

func setResolved(task *entities.PlannedTask, match entities.DomainContextMatch, snapshotHash string) {
	task.ResolvedEntity = &entities.ResolvedEntity{DocumentType: match.DocumentType, Identity: match.Identity, DisplayName: match.DisplayName, Snapshot: match.Snapshot, SnapshotHash: snapshotHash}
	task.Candidates = nil
	task.UnresolvedSlot = ""
	task.Status = "resolved"
}

func clarificationFor(task *entities.PlannedTask, matches []entities.DomainContextMatch, question string, limit int) *entities.Clarification {
	if len(matches) > limit {
		matches = matches[:limit]
	}
	candidates := make([]entities.ClarificationCandidate, 0, len(matches))
	for _, match := range matches {
		candidates = append(candidates, entities.ClarificationCandidate{CandidateID: candidateID(match), DocumentType: match.DocumentType, Identity: match.Identity, DisplayName: match.DisplayName, Snapshot: match.Snapshot})
	}
	task.Candidates = candidates
	task.UnresolvedSlot = "entity"
	task.Status = "awaiting_clarification"
	return &entities.Clarification{TaskID: task.ID, Slot: "entity", Question: question, Candidates: candidates}
}

func candidateID(match entities.DomainContextMatch) string {
	digest := sha256.Sum256([]byte(match.DocumentType + "\x00" + match.Identity + "\x00" + match.DisplayName))
	return hex.EncodeToString(digest[:12])
}

func publicCandidates(matches []entities.DomainContextMatch) []entities.ClarificationCandidate {
	result := make([]entities.ClarificationCandidate, 0, len(matches))
	for _, match := range matches {
		result = append(result, entities.ClarificationCandidate{CandidateID: candidateID(match), DocumentType: match.DocumentType, Identity: match.Identity, DisplayName: match.DisplayName})
	}
	return result
}

func validateAdequacy(prompt string, plan entities.TaskPlan, blocks []entities.RetrievedBlock) *entities.Clarification {
	for _, task := range plan.Tasks {
		if task.Intent == entities.IntentUnsupported {
			continue
		}
		found := false
		for _, block := range blocks {
			for _, taskID := range block.TaskIDs {
				if taskID == task.ID {
					found = true
				}
			}
		}
		if !found && task.SourceMode != entities.SourceConversation && task.SourceMode != entities.SourceNone {
			return &entities.Clarification{TaskID: task.ID, Slot: "context", Question: localized(prompt, "Недостаточно подтверждённого контекста. Уточните запрос.", "There is not enough verified context. Please clarify the request.")}
		}
	}
	return nil
}

func localized(prompt, russian, english string) string {
	for _, character := range prompt {
		if unicode.In(character, unicode.Cyrillic) {
			return russian
		}
	}
	return english
}

func mergeKnowledge(current, next entities.KnowledgeRetrieval) entities.KnowledgeRetrieval {
	if current.BundleID == "" {
		return next
	}
	seen := make(map[string]struct{}, len(current.Matches)+len(next.Matches))
	for _, match := range current.Matches {
		seen[match.ChunkID] = struct{}{}
	}
	for _, match := range next.Matches {
		if _, exists := seen[match.ChunkID]; exists {
			continue
		}
		seen[match.ChunkID] = struct{}{}
		current.Matches = append(current.Matches, match)
	}
	return current
}

func mergeDomain(current, next entities.DomainContext) entities.DomainContext {
	if current.Kind == "" {
		return next
	}
	seen := make(map[string]struct{}, len(current.Matches)+len(next.Matches))
	for _, match := range current.Matches {
		seen[match.DocumentType+"\x00"+match.Identity] = struct{}{}
	}
	for _, match := range next.Matches {
		key := match.DocumentType + "\x00" + match.Identity
		if _, exists := seen[key]; exists {
			continue
		}
		seen[key] = struct{}{}
		current.Matches = append(current.Matches, match)
	}
	return current
}

func matchesFolderScope(match entities.DomainContextMatch, folderMention string, task *entities.PlannedTask, plan entities.TaskPlan) bool {
	if folderMention == "" {
		return true
	}
	var folderIdentity string
	for _, dependency := range task.DependsOn {
		for _, candidate := range plan.Tasks {
			if candidate.ID == dependency && candidate.ResolvedEntity != nil {
				folderIdentity = candidate.ResolvedEntity.Identity
			}
		}
	}
	if folderIdentity == "" {
		return false
	}
	var value map[string]any
	if json.Unmarshal(match.Snapshot, &value) != nil {
		return false
	}
	for _, key := range []string{"folderIdentity", "folderId", "parentFolderIdentity", "parentIdentity"} {
		if fmt.Sprint(value[key]) == folderIdentity {
			return true
		}
	}
	return false
}

func resolvedFolderIdentity(task entities.PlannedTask, plan entities.TaskPlan) string {
	for _, dependency := range task.DependsOn {
		for _, candidate := range plan.Tasks {
			if candidate.ID == dependency && candidate.ResolvedEntity != nil {
				return candidate.ResolvedEntity.Identity
			}
		}
	}
	return ""
}
