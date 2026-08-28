package workbench

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/endge-lab/service-ai-workbench/internal/domain/entities"
	"github.com/endge-lab/service-ai-workbench/internal/usecase/ports"
)

type repositoryStub struct{ started entities.RunInput }

func (r *repositoryStub) ListConversations(context.Context, string, string, bool, int, *time.Time) ([]entities.Conversation, *time.Time, error) {
	return nil, nil, nil
}
func (r *repositoryStub) CreateConversation(context.Context, entities.Actor, entities.Workspace, entities.ModelSnapshot) (entities.Conversation, error) {
	return entities.Conversation{}, nil
}
func (r *repositoryStub) ResetConversation(context.Context, entities.Actor, entities.Workspace, string, entities.ModelSnapshot) (entities.Conversation, error) {
	return entities.Conversation{}, nil
}
func (r *repositoryStub) UpdateConversationModel(context.Context, string, string, string, entities.ModelSnapshot) (entities.Conversation, error) {
	return entities.Conversation{}, nil
}
func (r *repositoryStub) ListMessages(context.Context, string, string, string, int, *int64) ([]entities.Message, *int64, error) {
	return nil, nil, nil
}
func (r *repositoryStub) StartRun(_ context.Context, input entities.RunInput) (entities.Run, error) {
	r.started = input
	return entities.Run{}, nil
}
func (r *repositoryStub) CompleteRun(context.Context, string, string, string) (entities.Message, error) {
	return entities.Message{}, nil
}
func (r *repositoryStub) FailRun(context.Context, string, string, string, string) error { return nil }

type resolverStub struct{}

func (resolverStub) Resolve(string) (ports.Generator, bool) { return nil, true }

type failingGenerator struct{}

func (failingGenerator) Generate(_ context.Context, _ entities.GenerationRequest, emit func(string) error) error {
	if err := emit("provisional text"); err != nil {
		return err
	}
	return errors.New("provider failed after a partial response")
}

func TestRunRejectsSnapshotChecksumMismatchBeforePersistence(t *testing.T) {
	repository := &repositoryStub{}
	usecase := NewUseCase(repository, resolverStub{})
	input := entities.RunInput{
		RequestID: "247b2db9-f273-41a0-ae42-4d20c43fc3e0", Actor: entities.Actor{ID: "actor"}, Workspace: entities.Workspace{ID: "workspace"},
		ConversationID: "49f0ecb7-9389-4926-be55-091d87ab7a82", Prompt: "prompt", Snapshot: []byte("payload"), SnapshotSHA256: "wrong", Generation: "generation-1",
		Model:          entities.ModelSnapshot{ProfileID: "37273431-38ad-418a-9244-46ff3c279b43", ConnectionID: "8cfb0256-8125-426d-8f99-974a735ac07a", Adapter: "ollama", ProviderModelID: "llama", DisplayName: "Local"},
		ProviderAccess: entities.ProviderAccess{ConnectionID: "8cfb0256-8125-426d-8f99-974a735ac07a", BaseURL: "https://ollama.example"},
	}
	if err := usecase.Run(context.Background(), input, func(Event) error { return nil }); err == nil {
		t.Fatal("expected checksum mismatch")
	}
	if repository.started.RequestID != "" {
		t.Fatal("run must not be persisted for invalid snapshot")
	}
}

func TestBufferedGenerationDoesNotExposePartialResponseAfterFailure(t *testing.T) {
	content, err := generateBuffered(context.Background(), failingGenerator{}, entities.GenerationRequest{}, 1024)
	if err == nil {
		t.Fatal("provider failure was expected")
	}
	if content != "" {
		t.Fatalf("partial provider response escaped the buffer: %q", content)
	}
}
