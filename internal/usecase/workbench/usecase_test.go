package workbench

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"testing"
	"time"

	"github.com/endge-lab/service-ai-workbench/internal/domain/entities"
	"github.com/endge-lab/service-ai-workbench/internal/usecase/ports"
)

type repositoryStub struct {
	started   entities.RunInput
	completed string
	failed    bool
}

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
	return entities.Run{
		ID:                  "87e5e523-5555-4ed3-a85d-e9f2234c7e88",
		RequestID:           input.RequestID,
		UserMessageID:       "b31be9b8-61d6-449b-bef7-49a874ce4778",
		UserMessageSequence: 1,
	}, nil
}
func (r *repositoryStub) CompleteRun(_ context.Context, _, conversationID, content string) (entities.Message, error) {
	r.completed = content
	return entities.Message{ID: "ee4c83d6-6589-42df-9f66-8284dd9f1ee4", ConversationID: conversationID}, nil
}
func (r *repositoryStub) FailRun(context.Context, string, string, string, string) error {
	r.failed = true
	return nil
}

type generatorStub struct{}

func (generatorStub) Generate(_ context.Context, _ entities.GenerationRequest, emit func(string) error) error {
	if err := emit("hard"); err != nil {
		return err
	}
	return emit("coded")
}

type failingGeneratorStub struct{}

func (failingGeneratorStub) Generate(context.Context, entities.GenerationRequest, func(string) error) error {
	return context.Canceled
}

type failingResolverStub struct{}

func (failingResolverStub) Resolve(string) (ports.Generator, bool) {
	return failingGeneratorStub{}, true
}

type resolverStub struct{}

func (resolverStub) Resolve(adapter string) (ports.Generator, bool) {
	return generatorStub{}, adapter == "ollama"
}

func TestRunPersistsOnlyCompletedAssistantMessage(t *testing.T) {
	repository := &repositoryStub{}
	usecase := NewUseCase(repository, resolverStub{})
	snapshot := []byte(`{"workspace":"current"}`)
	digest := sha256.Sum256(snapshot)
	input := entities.RunInput{
		RequestID: "247b2db9-f273-41a0-ae42-4d20c43fc3e0",
		Actor:     entities.Actor{ID: "actor"}, Workspace: entities.Workspace{ID: "workspace"},
		ConversationID: "49f0ecb7-9389-4926-be55-091d87ab7a82", Prompt: "Что умеет проект?",
		Model: entities.ModelSnapshot{
			ProfileID: "37273431-38ad-418a-9244-46ff3c279b43", ConnectionID: "8cfb0256-8125-426d-8f99-974a735ac07a",
			Adapter: "ollama", ProviderModelID: "llama", DisplayName: "Local",
		},
		Snapshot: snapshot, SnapshotSHA256: hex.EncodeToString(digest[:]), Generation: "generation-1",
		ProviderAccess: entities.ProviderAccess{ConnectionID: "8cfb0256-8125-426d-8f99-974a735ac07a", BaseURL: "https://ollama.com"},
	}
	events := make([]Event, 0, 4)
	if err := usecase.Run(context.Background(), input, func(event Event) error {
		events = append(events, event)
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	if repository.completed != "hardcoded" || repository.failed {
		t.Fatalf("unexpected repository state: completed=%q failed=%v", repository.completed, repository.failed)
	}
	if len(events) != 4 || events[0].Type != EventStarted || events[3].Type != EventCompleted {
		t.Fatalf("unexpected event sequence: %#v", events)
	}
}

func TestRunRejectsSnapshotChecksumMismatchBeforePersistence(t *testing.T) {
	repository := &repositoryStub{}
	usecase := NewUseCase(repository, resolverStub{})
	input := entities.RunInput{
		RequestID: "247b2db9-f273-41a0-ae42-4d20c43fc3e0", Actor: entities.Actor{ID: "actor"}, Workspace: entities.Workspace{ID: "workspace"},
		ConversationID: "49f0ecb7-9389-4926-be55-091d87ab7a82", Prompt: "prompt", Snapshot: []byte("payload"), SnapshotSHA256: "wrong", Generation: "generation-1",
		Model:          entities.ModelSnapshot{ProfileID: "37273431-38ad-418a-9244-46ff3c279b43", ConnectionID: "8cfb0256-8125-426d-8f99-974a735ac07a", Adapter: "ollama", ProviderModelID: "llama", DisplayName: "Local"},
		ProviderAccess: entities.ProviderAccess{ConnectionID: "8cfb0256-8125-426d-8f99-974a735ac07a", BaseURL: "https://ollama.com"},
	}
	if err := usecase.Run(context.Background(), input, func(Event) error { return nil }); err == nil {
		t.Fatal("expected checksum mismatch")
	}
	if repository.started.RequestID != "" {
		t.Fatal("run must not be persisted for invalid snapshot")
	}
}

func TestRunFailureDoesNotPersistPartialAssistantMessage(t *testing.T) {
	repository := &repositoryStub{}
	usecase := NewUseCase(repository, failingResolverStub{})
	snapshot := []byte(`{"workspace":"current"}`)
	digest := sha256.Sum256(snapshot)
	input := entities.RunInput{
		RequestID: "247b2db9-f273-41a0-ae42-4d20c43fc3e0", Actor: entities.Actor{ID: "actor"}, Workspace: entities.Workspace{ID: "workspace"},
		ConversationID: "49f0ecb7-9389-4926-be55-091d87ab7a82", Prompt: "prompt", Snapshot: snapshot, SnapshotSHA256: hex.EncodeToString(digest[:]), Generation: "generation-1",
		Model:          entities.ModelSnapshot{ProfileID: "37273431-38ad-418a-9244-46ff3c279b43", ConnectionID: "8cfb0256-8125-426d-8f99-974a735ac07a", Adapter: "ollama", ProviderModelID: "llama", DisplayName: "Local"},
		ProviderAccess: entities.ProviderAccess{ConnectionID: "8cfb0256-8125-426d-8f99-974a735ac07a", BaseURL: "https://ollama.com"},
	}
	if err := usecase.Run(context.Background(), input, func(Event) error { return nil }); err == nil {
		t.Fatal("expected generator failure")
	}
	if repository.completed != "" || !repository.failed {
		t.Fatalf("partial assistant message was persisted: completed=%q failed=%v", repository.completed, repository.failed)
	}
}
