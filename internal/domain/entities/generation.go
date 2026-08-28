package entities

type ModelMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type ModelRequest struct {
	Model        ModelSnapshot  `json:"model"`
	SystemPrompt string         `json:"system"`
	Messages     []ModelMessage `json:"messages"`
}

type GenerationRequest struct {
	ModelRequest   ModelRequest
	ProviderAccess ProviderAccess
}
