package entities

import "encoding/json"

var (
	FinalAnswerSchema = json.RawMessage(`{
  "type":"object","additionalProperties":false,
  "required":["answer","entityCitations","documentationCitations","limitations"],
  "properties":{
    "answer":{"type":"string"},
    "entityCitations":{"type":"array","items":{"type":"object","additionalProperties":false,"required":["documentType","identity"],"properties":{"documentType":{"type":"string"},"identity":{"type":"string"}}}},
    "documentationCitations":{"type":"array","items":{"type":"string"}},
    "limitations":{"type":"array","items":{"type":"string"}}
  }
}`)
	TaskPlanSchema = json.RawMessage(`{
  "type":"object","additionalProperties":false,"required":["tasks"],
  "properties":{"tasks":{"type":"array","minItems":1,"items":{"type":"object","additionalProperties":false,
    "required":["id","intent","sourceMode","mentions","expectedTypes","requestedAspects","dependsOn","confidence","status"],
    "properties":{
      "id":{"type":"string"},"intent":{"enum":["explain_documentation","find_entity","inspect_entity","list_entities","unsupported"]},
      "sourceMode":{"enum":["documentation","domain","mixed","conversation","none"]},
      "mentions":{"type":"array","items":{"type":"string"}},"expectedTypes":{"type":"array","items":{"type":"string"}},
      "requestedAspects":{"type":"array","items":{"type":"string"}},"dependsOn":{"type":"array","items":{"type":"string"}},
      "folderMention":{"type":"string"},"confidence":{"type":"number","minimum":0,"maximum":1},"status":{"type":"string"}
    }
  }}}
}`)
	QueryExpansionSchema          = json.RawMessage(`{"type":"object","additionalProperties":false,"required":["queries"],"properties":{"queries":{"type":"array","maxItems":5,"items":{"type":"string"}}}}`)
	RerankerSchema                = json.RawMessage(`{"type":"object","additionalProperties":false,"required":["selectedCandidateId","confidence","requiresClarification","reason"],"properties":{"selectedCandidateId":{"type":"string"},"confidence":{"type":"number","minimum":0,"maximum":1},"requiresClarification":{"type":"boolean"},"reason":{"type":"string"}}}`)
	ClarificationClassifierSchema = json.RawMessage(`{"type":"object","additionalProperties":false,"required":["kind","confidence"],"properties":{"kind":{"enum":["answer","correction","new_request","cancel","unclear"]},"confidence":{"type":"number","minimum":0,"maximum":1}}}`)
)
