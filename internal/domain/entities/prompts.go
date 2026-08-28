package entities

type PromptID string

const (
	PromptFinalAnswerSystem              PromptID = "final-answer.system.v1"
	PromptFinalAnswerRequest             PromptID = "final-answer.request.v1"
	PromptPlannerSystem                  PromptID = "planner.system.v1"
	PromptPlannerRequest                 PromptID = "planner.request.v1"
	PromptQueryExpanderSystem            PromptID = "query-expander.system.v1"
	PromptQueryExpanderRequest           PromptID = "query-expander.request.v1"
	PromptRerankerSystem                 PromptID = "reranker.system.v1"
	PromptRerankerRequest                PromptID = "reranker.request.v1"
	PromptClarificationClassifierSystem  PromptID = "clarification-classifier.system.v1"
	PromptClarificationClassifierRequest PromptID = "clarification-classifier.request.v1"
	PromptRepairSystem                   PromptID = "repair.system.v1"
	PromptRepairRequest                  PromptID = "repair.request.v1"
)

type RenderedPrompt struct {
	ID      PromptID
	Version string
	SHA256  string
	Content string
}
