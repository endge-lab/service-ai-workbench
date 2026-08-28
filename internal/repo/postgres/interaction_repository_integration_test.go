package postgres

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"os"
	"strings"
	"testing"

	"github.com/endge-lab/service-ai-workbench/internal/domain/entities"
	domainerrors "github.com/endge-lab/service-ai-workbench/internal/domain/errors"
	"github.com/endge-lab/service-ai-workbench/migrations"
	"github.com/endge-lab/service-kit-go/pkg/migrator"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/jackc/pgx/v5/stdlib"
	"go.uber.org/zap"
)

func TestInteractionRepositoryIntegration(t *testing.T) {
	databaseURL := strings.TrimSpace(os.Getenv("AI_WORKBENCH_TEST_DATABASE_URL"))
	if databaseURL == "" {
		t.Skip("AI_WORKBENCH_TEST_DATABASE_URL is not configured")
	}
	ctx := context.Background()
	admin, err := pgxpool.New(ctx, databaseURL)
	if err != nil {
		t.Fatal(err)
	}
	defer admin.Close()
	schema := "ai_workbench_test_" + strings.ReplaceAll(uuid.NewString(), "-", "")
	identifier := pgx.Identifier{schema}.Sanitize()
	if _, err := admin.Exec(ctx, "CREATE SCHEMA "+identifier); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _, _ = admin.Exec(context.Background(), "DROP SCHEMA "+identifier+" CASCADE") })

	configuration, err := pgxpool.ParseConfig(databaseURL)
	if err != nil {
		t.Fatal(err)
	}
	configuration.ConnConfig.RuntimeParams["search_path"] = schema + ",public"
	pool, err := pgxpool.NewWithConfig(ctx, configuration)
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()
	standardDB := stdlib.OpenDB(*configuration.ConnConfig)
	defer standardDB.Close()
	if err := migrator.NewMigrator(standardDB, migrations.FS, zap.NewNop()).Up(); err != nil {
		t.Fatal(err)
	}

	repository := NewWorkbenchRepository(pool)
	model := entities.ModelSnapshot{
		ProfileID: uuid.NewString(), ConnectionID: uuid.NewString(), Adapter: "ollama",
		ProviderModelID: "example-model", DisplayName: "Example Model",
	}
	actor := entities.Actor{ID: "actor-example", Username: "actor", DisplayName: "Example Actor"}
	workspace := entities.Workspace{ID: "workspace-example", Name: "Example Workspace"}
	conversation, err := repository.CreateConversation(ctx, actor, workspace, model)
	if err != nil {
		t.Fatal(err)
	}
	snapshot := []byte(`{"kind":"workspace-snapshot","schemaVersion":1,"documents":{}}`)
	digest := sha256.Sum256(snapshot)
	run, err := repository.StartRun(ctx, entities.RunInput{
		RequestID: uuid.NewString(), Actor: actor, Workspace: workspace, ConversationID: conversation.ID,
		Prompt: "Find an example entity", Model: model, Snapshot: snapshot, Generation: "1", SnapshotSHA256: hex.EncodeToString(digest[:]),
	})
	if err != nil {
		t.Fatal(err)
	}
	interaction, err := repository.CreateInteraction(ctx, run.ID, conversation.ID, run.UserMessageID, "1", hex.EncodeToString(digest[:]), "bundle-example")
	if err != nil {
		t.Fatal(err)
	}

	var attached string
	if err := pool.QueryRow(ctx, `SELECT interaction_id::text FROM runs WHERE id=$1`, run.ID).Scan(&attached); err != nil || attached != interaction.ID {
		t.Fatalf("interaction was not atomically attached: id=%q err=%v", attached, err)
	}
	var competingRootMessageID string
	if err := pool.QueryRow(ctx, `INSERT INTO messages (conversation_id, role, content) VALUES ($1,'user','competing request') RETURNING id::text`, conversation.ID).Scan(&competingRootMessageID); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `INSERT INTO interactions (conversation_id, root_message_id, status) VALUES ($1,$2,'planning')`, conversation.ID, competingRootMessageID); err == nil {
		t.Fatal("a conversation must not accept a second active interaction")
	}

	interaction.Plan = entities.TaskPlan{Tasks: []entities.PlannedTask{{ID: "task-1", Intent: entities.IntentFindEntity, SourceMode: entities.SourceDomain}}}
	clarification, _, err := repository.CreateClarification(ctx, run.ID, interaction, entities.Clarification{
		TaskID: "task-1", Slot: "entity", Question: "Which entity?",
		Candidates: []entities.ClarificationCandidate{{CandidateID: "candidate-1", DocumentType: "examples", Identity: "example-1", DisplayName: "Example One"}},
	})
	if err != nil {
		t.Fatal(err)
	}
	var competingQuestionMessageID string
	if err := pool.QueryRow(ctx, `INSERT INTO messages (conversation_id, role, content) VALUES ($1,'assistant','another question') RETURNING id::text`, conversation.ID).Scan(&competingQuestionMessageID); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `INSERT INTO clarifications (interaction_id, task_id, slot, question_message_id, status, plan_version) VALUES ($1,'task-1','entity',$2,'open',$3)`, interaction.ID, competingQuestionMessageID, clarification.PlanVersion); err == nil {
		t.Fatal("an interaction must not accept a second open clarification")
	}
	current, err := repository.GetInteraction(ctx, actor.ID, workspace.ID, conversation.ID, interaction.ID)
	if err != nil || current == nil {
		t.Fatalf("interaction could not be reloaded: %v", err)
	}
	_, err = repository.ApplyClarification(ctx, entities.ClarificationAnswer{
		InteractionID: interaction.ID, ClarificationID: clarification.ID, UserMessageID: run.UserMessageID,
		BasePlanVersion: clarification.PlanVersion - 1, Status: "answered",
	}, *current, current.PlanVersion)
	if !errors.Is(err, domainerrors.ErrConflict) {
		t.Fatalf("stale clarification plan must conflict: %v", err)
	}
	current.Status = entities.InteractionResolving
	updated, err := repository.ApplyClarification(ctx, entities.ClarificationAnswer{
		InteractionID: interaction.ID, ClarificationID: clarification.ID, UserMessageID: run.UserMessageID,
		BasePlanVersion: clarification.PlanVersion, Status: "answered",
	}, *current, current.PlanVersion)
	if err != nil {
		t.Fatal(err)
	}
	if updated.PlanVersion != current.PlanVersion+1 || updated.Status != entities.InteractionResolving {
		t.Fatalf("clarification patch was not atomic: %#v", updated)
	}

	failedConversation, err := repository.ResetConversation(ctx, actor, workspace, conversation.ID, model)
	if err != nil {
		t.Fatal(err)
	}
	superseded, err := repository.GetInteraction(ctx, actor.ID, workspace.ID, conversation.ID, interaction.ID)
	if err != nil || superseded == nil || superseded.Status != entities.InteractionSuperseded {
		t.Fatalf("reset must supersede the archived interaction: %#v err=%v", superseded, err)
	}
	failedRun, err := repository.StartRun(ctx, entities.RunInput{
		RequestID: uuid.NewString(), Actor: actor, Workspace: workspace, ConversationID: failedConversation.ID,
		Prompt: "Request that will fail", Model: model, Snapshot: snapshot, Generation: "2", SnapshotSHA256: hex.EncodeToString(digest[:]),
	})
	if err != nil {
		t.Fatal(err)
	}
	failedInteraction, err := repository.CreateInteraction(ctx, failedRun.ID, failedConversation.ID, failedRun.UserMessageID, "2", hex.EncodeToString(digest[:]), "bundle-example")
	if err != nil {
		t.Fatal(err)
	}
	if err := repository.FailInteraction(ctx, failedRun.ID, failedInteraction.ID, "failed", "provider_failed", "provider failed"); err != nil {
		t.Fatal(err)
	}
	var userMessages, assistantMessages int
	if err := pool.QueryRow(ctx, `SELECT count(*) FILTER (WHERE role='user'), count(*) FILTER (WHERE role='assistant') FROM messages WHERE conversation_id=$1`, failedConversation.ID).Scan(&userMessages, &assistantMessages); err != nil {
		t.Fatal(err)
	}
	if userMessages != 1 || assistantMessages != 0 {
		t.Fatalf("failed run persistence is invalid: user=%d assistant=%d", userMessages, assistantMessages)
	}

	if _, err := pool.Exec(ctx, `DELETE FROM conversations WHERE id=$1`, conversation.ID); err != nil {
		t.Fatal(err)
	}
	var interactions, clarifications int
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM interactions WHERE conversation_id=$1`, conversation.ID).Scan(&interactions); err != nil {
		t.Fatal(err)
	}
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM clarifications cl JOIN interactions i ON i.id=cl.interaction_id WHERE i.conversation_id=$1`, conversation.ID).Scan(&clarifications); err != nil {
		t.Fatal(err)
	}
	if interactions != 0 || clarifications != 0 {
		t.Fatalf("conversation cascade left rows: interactions=%d clarifications=%d", interactions, clarifications)
	}
}
