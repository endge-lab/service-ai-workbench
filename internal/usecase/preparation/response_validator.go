package preparation

import (
	"context"
	"encoding/json"
	"strings"

	"github.com/endge-lab/service-ai-workbench/internal/domain/entities"
	"github.com/endge-lab/service-ai-workbench/internal/usecase/ports"
)

type ResponseValidator struct {
	prompts ports.PromptCatalog
	models  ports.StructuredModelInvoker
}

func NewResponseValidator(prompts ports.PromptCatalog, models ports.StructuredModelInvoker) *ResponseValidator {
	return &ResponseValidator{prompts: prompts, models: models}
}

func (v *ResponseValidator) Validate(ctx context.Context, input entities.RunInput, preparation entities.PreparationResult, raw []byte) (entities.ResponseValidation, []entities.PromptUsage, error) {
	validation := validateStructuredResponse(raw, preparation)
	if validation.Valid || hasFatalValidationError(validation.Errors) || !hasSchemaValidationError(validation.Errors) {
		return validation, nil, nil
	}
	budget := &modelCallBudget{limit: 1}
	repaired, usages, err := repairStructured(ctx, v.prompts, v.models, input, raw, strings.Join(validation.Errors, "; "), budget)
	if err != nil {
		return validation, usages, err
	}
	validation = validateStructuredResponse(repaired, preparation)
	validation.Repaired = true
	return validation, usages, nil
}

func hasSchemaValidationError(errors []string) bool {
	for _, code := range errors {
		if code == "response_schema_invalid" {
			return true
		}
	}
	return false
}

func validateStructuredResponse(raw []byte, preparation entities.PreparationResult) entities.ResponseValidation {
	result := entities.ResponseValidation{}
	if err := json.Unmarshal(extractJSONObject(raw), &result.Response); err != nil {
		result.Errors = append(result.Errors, "response_schema_invalid")
		return result
	}
	if strings.TrimSpace(result.Response.Answer) == "" {
		result.Errors = append(result.Errors, "answer_empty")
	}
	allowedEntities := map[string]struct{}{}
	allowedDocs := map[string]struct{}{}
	for _, task := range preparation.Plan.Tasks {
		if task.ResolvedEntity != nil {
			allowedEntities[task.ResolvedEntity.DocumentType+"\x00"+task.ResolvedEntity.Identity] = struct{}{}
		}
	}
	for _, block := range preparation.Trace.Blocks {
		if block.SourceKind == "documentation" {
			allowedDocs[block.SourceKey] = struct{}{}
		}
	}
	for _, citation := range result.Response.EntityCitations {
		if _, ok := allowedEntities[citation.DocumentType+"\x00"+citation.Identity]; !ok {
			result.Errors = append(result.Errors, "unconfirmed_identity")
		}
	}
	filteredDocs := make([]string, 0, len(result.Response.DocumentationCitations))
	removedDocs := false
	for _, citation := range result.Response.DocumentationCitations {
		if _, ok := allowedDocs[citation]; ok {
			filteredDocs = append(filteredDocs, citation)
		} else {
			removedDocs = true
		}
	}
	result.Response.DocumentationCitations = filteredDocs
	if removedDocs {
		result.Response.Limitations = append(result.Response.Limitations, localized(result.Response.Answer,
			"Часть ссылок на документацию удалена, потому что не была подтверждена контекстом.",
			"Some documentation citations were removed because the supplied context did not confirm them."))
	}
	if containsMutationClaim(result.Response.Answer) {
		result.Errors = append(result.Errors, "mutation_claim_forbidden")
	}
	result.Valid = len(result.Errors) == 0
	return result
}

func hasFatalValidationError(errors []string) bool {
	for _, code := range errors {
		if code == "unconfirmed_identity" || code == "mutation_claim_forbidden" {
			return true
		}
	}
	return false
}

func containsMutationClaim(answer string) bool {
	value := normalizeText(answer)
	for _, phrase := range []string{"я изменил", "я удалил", "я создал", "изменения применены", "i changed", "i deleted", "i created", "changes applied"} {
		if strings.Contains(value, phrase) {
			return true
		}
	}
	return false
}

func RenderValidatedAnswer(response entities.StructuredResponse) string {
	answer := strings.TrimSpace(response.Answer)
	if len(response.Limitations) > 0 {
		answer += "\n\n" + localized(answer, "Ограничения:", "Limitations:") + "\n- " + strings.Join(response.Limitations, "\n- ")
	}
	return answer
}
