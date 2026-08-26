package v1

import (
	"context"
	"errors"

	workbenchv1 "github.com/endge-lab/service-ai-workbench/api/workbench/v1"
	"github.com/endge-lab/service-ai-workbench/internal/domain/entities"
	domainerrors "github.com/endge-lab/service-ai-workbench/internal/domain/errors"
	workbenchusecase "github.com/endge-lab/service-ai-workbench/internal/usecase/workbench"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/timestamppb"
)

type Server struct {
	workbenchv1.UnimplementedWorkbenchServiceServer
	usecase *workbenchusecase.UseCase
}

func NewServer(usecase *workbenchusecase.UseCase) *Server {
	return &Server{usecase: usecase}
}

func (s *Server) GetCapabilities(context.Context, *workbenchv1.GetCapabilitiesRequest) (*workbenchv1.GetCapabilitiesResponse, error) {
	return &workbenchv1.GetCapabilitiesResponse{Adapters: s.usecase.Capabilities()}, nil
}

func (s *Server) ListConversations(ctx context.Context, request *workbenchv1.ListConversationsRequest) (*workbenchv1.ListConversationsResponse, error) {
	items, cursor, err := s.usecase.ListConversations(ctx, request.GetActorId(), request.GetWorkspaceId(), request.GetIncludeArchived(), int(request.GetLimit()), request.GetCursor())
	if err != nil {
		return nil, mapError(err)
	}
	response := &workbenchv1.ListConversationsResponse{Items: make([]*workbenchv1.Conversation, 0, len(items)), NextCursor: cursor}
	for _, item := range items {
		response.Items = append(response.Items, conversationToProto(item))
	}
	return response, nil
}

func (s *Server) CreateConversation(ctx context.Context, request *workbenchv1.CreateConversationRequest) (*workbenchv1.CreateConversationResponse, error) {
	conversation, err := s.usecase.CreateConversation(ctx, actorFromProto(request.GetActor()), workspaceFromProto(request.GetWorkspace()), modelFromProto(request.GetModel()))
	if err != nil {
		return nil, mapError(err)
	}
	return &workbenchv1.CreateConversationResponse{Conversation: conversationToProto(conversation)}, nil
}

func (s *Server) ResetConversation(ctx context.Context, request *workbenchv1.ResetConversationRequest) (*workbenchv1.ResetConversationResponse, error) {
	conversation, err := s.usecase.ResetConversation(ctx, actorFromProto(request.GetActor()), workspaceFromProto(request.GetWorkspace()), request.GetCurrentConversationId(), modelFromProto(request.GetModel()))
	if err != nil {
		return nil, mapError(err)
	}
	return &workbenchv1.ResetConversationResponse{Conversation: conversationToProto(conversation)}, nil
}

func (s *Server) UpdateConversationModel(ctx context.Context, request *workbenchv1.UpdateConversationModelRequest) (*workbenchv1.UpdateConversationModelResponse, error) {
	conversation, err := s.usecase.UpdateConversationModel(ctx, request.GetActorId(), request.GetWorkspaceId(), request.GetConversationId(), modelFromProto(request.GetModel()))
	if err != nil {
		return nil, mapError(err)
	}
	return &workbenchv1.UpdateConversationModelResponse{Conversation: conversationToProto(conversation)}, nil
}

func (s *Server) ListMessages(ctx context.Context, request *workbenchv1.ListMessagesRequest) (*workbenchv1.ListMessagesResponse, error) {
	items, cursor, err := s.usecase.ListMessages(ctx, request.GetActorId(), request.GetWorkspaceId(), request.GetConversationId(), int(request.GetLimit()), request.GetCursor())
	if err != nil {
		return nil, mapError(err)
	}
	response := &workbenchv1.ListMessagesResponse{Items: make([]*workbenchv1.Message, 0, len(items)), NextCursor: cursor}
	for _, item := range items {
		response.Items = append(response.Items, messageToProto(item))
	}
	return response, nil
}

func (s *Server) Run(request *workbenchv1.RunRequest, stream workbenchv1.WorkbenchService_RunServer) error {
	snapshot := request.GetSnapshot()
	input := entities.RunInput{
		RequestID: request.GetRequestId(), Actor: actorFromProto(request.GetActor()), Workspace: workspaceFromProto(request.GetWorkspace()),
		ConversationID: request.GetConversationId(), Prompt: request.GetPrompt(), Model: modelFromProto(request.GetModel()),
	}
	if snapshot != nil {
		input.Snapshot = snapshot.GetPayload()
		input.Generation = snapshot.GetGeneration()
		input.HeadRevisionID = snapshot.GetHeadRevisionId()
		input.SnapshotSHA256 = snapshot.GetSha256()
	}
	err := s.usecase.Run(stream.Context(), input, func(event workbenchusecase.Event) error {
		return stream.Send(&workbenchv1.RunResponse{
			Type: eventTypeToProto(event.Type), RunId: event.RunID, MessageId: event.MessageID,
			Delta: event.Delta, ErrorCode: event.ErrorCode, ErrorMessage: event.ErrorMessage,
			CreatedAt: timestamppb.New(event.CreatedAt),
		})
	})
	return mapError(err)
}

func actorFromProto(value *workbenchv1.Actor) entities.Actor {
	if value == nil {
		return entities.Actor{}
	}
	return entities.Actor{ID: value.GetId(), Username: value.GetUsername(), DisplayName: value.GetDisplayName()}
}

func workspaceFromProto(value *workbenchv1.Workspace) entities.Workspace {
	if value == nil {
		return entities.Workspace{}
	}
	return entities.Workspace{ID: value.GetId(), Name: value.GetName()}
}

func modelFromProto(value *workbenchv1.ModelSnapshot) entities.ModelSnapshot {
	if value == nil {
		return entities.ModelSnapshot{}
	}
	return entities.ModelSnapshot{
		ProfileID: value.GetProfileId(), ConnectionID: value.GetConnectionId(), Adapter: value.GetAdapter(),
		ProviderModelID: value.GetProviderModelId(), DisplayName: value.GetDisplayName(),
	}
}

func conversationToProto(value entities.Conversation) *workbenchv1.Conversation {
	return &workbenchv1.Conversation{
		Id: value.ID, ActorId: value.ActorID, WorkspaceId: value.WorkspaceID, Model: modelToProto(value.Model),
		Archived: value.Archived, MessageCount: value.MessageCount,
		CreatedAt: timestamppb.New(value.CreatedAt), UpdatedAt: timestamppb.New(value.UpdatedAt),
	}
}

func modelToProto(value entities.ModelSnapshot) *workbenchv1.ModelSnapshot {
	return &workbenchv1.ModelSnapshot{
		ProfileId: value.ProfileID, ConnectionId: value.ConnectionID, Adapter: value.Adapter,
		ProviderModelId: value.ProviderModelID, DisplayName: value.DisplayName,
	}
}

func messageToProto(value entities.Message) *workbenchv1.Message {
	role := workbenchv1.MessageRole_MESSAGE_ROLE_USER
	if value.Role == "assistant" {
		role = workbenchv1.MessageRole_MESSAGE_ROLE_ASSISTANT
	}
	return &workbenchv1.Message{
		Id: value.ID, ConversationId: value.ConversationID, Role: role, Content: value.Content,
		Sequence: value.Sequence, CreatedAt: timestamppb.New(value.CreatedAt),
	}
}

func eventTypeToProto(value int) workbenchv1.RunEventType {
	switch value {
	case workbenchusecase.EventStarted:
		return workbenchv1.RunEventType_RUN_EVENT_TYPE_STARTED
	case workbenchusecase.EventContentDelta:
		return workbenchv1.RunEventType_RUN_EVENT_TYPE_CONTENT_DELTA
	case workbenchusecase.EventCompleted:
		return workbenchv1.RunEventType_RUN_EVENT_TYPE_COMPLETED
	case workbenchusecase.EventFailed:
		return workbenchv1.RunEventType_RUN_EVENT_TYPE_FAILED
	default:
		return workbenchv1.RunEventType_RUN_EVENT_TYPE_UNSPECIFIED
	}
}

func mapError(err error) error {
	if err == nil {
		return nil
	}
	switch {
	case errors.Is(err, domainerrors.ErrInvalidInput):
		return status.Error(codes.InvalidArgument, "invalid request")
	case errors.Is(err, domainerrors.ErrNotFound):
		return status.Error(codes.NotFound, "resource not found")
	case errors.Is(err, domainerrors.ErrConflict):
		return status.Error(codes.FailedPrecondition, "resource state conflict")
	case errors.Is(err, context.Canceled):
		return status.Error(codes.Canceled, "request cancelled")
	case errors.Is(err, context.DeadlineExceeded):
		return status.Error(codes.DeadlineExceeded, "request deadline exceeded")
	default:
		return status.Error(codes.Internal, "workbench operation failed")
	}
}
