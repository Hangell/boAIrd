package storage

import (
	"context"
	"errors"
	"path/filepath"
	"strings"
	"testing"
)

func TestNotificationsRedactAndDeduplicateSecurityEvents(t *testing.T) {
	ctx := context.Background()
	database, err := openTestDatabase(t, ctx, filepath.Join(t.TempDir(), "notifications.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = database.Close() })
	if _, err := database.ExecContext(ctx, `INSERT INTO projects(id,name,status,human_review_required) VALUES ('p1','Project','CREATED',0)`); err != nil {
		t.Fatal(err)
	}
	if _, err := database.ExecContext(ctx, `INSERT INTO authorization_denials(project_id, principal_id, resource, reason) VALUES ('p1','admin','/api?token=secret','secret_payload')`); err != nil {
		t.Fatal(err)
	}
	var count int
	if err := database.QueryRowContext(ctx, `SELECT COUNT(*) FROM notifications WHERE project_id='p1'`).Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != 1 {
		t.Fatalf("notification count = %d, want 1", count)
	}
	repository, err := NewNotificationRepository(database)
	if err != nil {
		t.Fatal(err)
	}
	items, err := repository.List(ctx, "p1", 100, 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 1 || items[0].Severity != "CRITICAL" || !strings.Contains(items[0].Summary, "security policy") {
		t.Fatalf("unexpected security notification: %#v", items)
	}
	if strings.Contains(items[0].Summary, "secret") || strings.Contains(items[0].ResourceID, "secret") {
		t.Fatalf("notification leaked sensitive data: %#v", items[0])
	}
	if err := repository.Acknowledge(ctx, "p1", items[0].ID); err != nil {
		t.Fatal(err)
	}
	items, err = repository.List(ctx, "p1", 100, 0)
	if err != nil {
		t.Fatal(err)
	}
	if items[0].AcknowledgedAt == nil || items[0].ReadAt == nil {
		t.Fatalf("acknowledgement state not persisted: %#v", items[0])
	}
	var auditCount int
	if err := database.QueryRowContext(ctx, `SELECT COUNT(*) FROM audit_events WHERE resource_type='notification' AND resource_id=? AND event_type='notification.acknowledged'`, items[0].ID).Scan(&auditCount); err != nil {
		t.Fatal(err)
	}
	if auditCount != 1 {
		t.Fatalf("acknowledgement audit count = %d, want 1", auditCount)
	}
}

func TestNotificationRetriesAreBoundedAndCannotDuplicate(t *testing.T) {
	ctx := context.Background()
	database, err := openTestDatabase(t, ctx, filepath.Join(t.TempDir(), "notifications.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = database.Close() })
	if _, err := database.ExecContext(ctx, `INSERT INTO projects(id,name,status,human_review_required) VALUES ('p1','Project','CREATED',0)`); err != nil {
		t.Fatal(err)
	}
	if _, err := database.ExecContext(ctx, `INSERT INTO audit_events(id,project_id,actor_type,actor_id,event_type,resource_type,resource_id,to_status) VALUES ('event-1','p1','system','test','task.status.changed','task','task-1','BLOCKED')`); err != nil {
		t.Fatal(err)
	}
	if _, err := database.ExecContext(ctx, `INSERT INTO notifications(project_id,source_key,event_type,severity,summary) VALUES ('p1','audit:event-1','BLOCKED','WARNING','duplicate') ON CONFLICT DO NOTHING`); err != nil {
		t.Fatal(err)
	}
	repository, err := NewNotificationRepository(database)
	if err != nil {
		t.Fatal(err)
	}
	items, err := repository.List(ctx, "p1", 100, 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 1 {
		t.Fatalf("notification count = %d, want 1", len(items))
	}
	for range 3 {
		if err := repository.RecordDelivery(ctx, items[0].ID, errors.New("private subscriber error")); err != nil {
			t.Fatal(err)
		}
	}
	items, err = repository.List(ctx, "p1", 100, 0)
	if err != nil {
		t.Fatal(err)
	}
	if items[0].DeliveryState != "FAILED" || items[0].DeliveryTries != 3 || items[0].DeliveryError != "subscriber_unavailable" {
		t.Fatalf("retry policy state = %#v", items[0])
	}
	if err := repository.Retry(ctx, "p1", items[0].ID); err != nil {
		t.Fatal(err)
	}
	if err := repository.RecordDelivery(ctx, items[0].ID, errors.New("private subscriber error")); err != nil {
		t.Fatal(err)
	}
	if err := repository.Retry(ctx, "p1", items[0].ID); err != nil {
		t.Fatal(err)
	}
	if err := repository.RecordDelivery(ctx, items[0].ID, errors.New("private subscriber error")); err != nil {
		t.Fatal(err)
	}
	items, err = repository.List(ctx, "p1", 100, 0)
	if err != nil {
		t.Fatal(err)
	}
	if items[0].DeliveryState != "FAILED" || items[0].DeliveryTries != 5 {
		t.Fatalf("manual retry cap state = %#v", items[0])
	}
	if err := repository.Retry(ctx, "p1", items[0].ID); !errors.Is(err, ErrNotificationRetryLimit) {
		t.Fatalf("Retry() error = %v, want limit error", err)
	}
	if err := repository.Acknowledge(ctx, "p1", items[0].ID); err != nil {
		t.Fatal(err)
	}
}

func TestNotificationSuccessfulRetryMarksDelivered(t *testing.T) {
	ctx := context.Background()
	database, err := openTestDatabase(t, ctx, filepath.Join(t.TempDir(), "notifications.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = database.Close() })
	if _, err := database.ExecContext(ctx, `INSERT INTO projects(id,name,status,human_review_required) VALUES ('p1','Project','CREATED',0)`); err != nil {
		t.Fatal(err)
	}
	if _, err := database.ExecContext(ctx, `INSERT INTO audit_events(project_id,actor_type,actor_id,event_type,resource_type,resource_id,to_status) VALUES ('p1','system','test','task.status.changed','task','task-1','HUMAN_REVIEW')`); err != nil {
		t.Fatal(err)
	}
	repository, err := NewNotificationRepository(database)
	if err != nil {
		t.Fatal(err)
	}
	items, err := repository.List(ctx, "p1", 100, 0)
	if err != nil {
		t.Fatal(err)
	}
	for range 3 {
		if err := repository.RecordDelivery(ctx, items[0].ID, errors.New("disconnected")); err != nil {
			t.Fatal(err)
		}
	}
	if err := repository.Retry(ctx, "p1", items[0].ID); err != nil {
		t.Fatal(err)
	}
	pending, err := repository.Pending(ctx, 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(pending) != 1 {
		t.Fatalf("pending count = %d, want 1", len(pending))
	}
	if err := repository.RecordDelivery(ctx, items[0].ID, nil); err != nil {
		t.Fatal(err)
	}
	items, err = repository.List(ctx, "p1", 100, 0)
	if err != nil {
		t.Fatal(err)
	}
	if items[0].DeliveryState != "DELIVERED" || items[0].DeliveryTries != 4 || items[0].DeliveredAt == nil {
		t.Fatalf("delivery retry state = %#v", items[0])
	}
}

func TestTaskRunAndAgentFailuresCreateProjectNotifications(t *testing.T) {
	ctx := context.Background()
	database, err := openTestDatabase(t, ctx, filepath.Join(t.TempDir(), "notifications.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = database.Close() })
	statements := []string{
		`INSERT INTO projects(id,name,status,human_review_required) VALUES ('p1','Project','CREATED',0)`,
		`INSERT INTO audit_events(project_id,actor_type,actor_id,event_type,resource_type,resource_id,to_status) VALUES ('p1','system','test','task.status.changed','task','task-failed','FAILED')`,
		`INSERT INTO audit_events(project_id,actor_type,actor_id,event_type,resource_type,resource_id,to_status) VALUES ('p1','agent','a1','run.status.changed','run','run-failed','FAILED')`,
		`INSERT INTO agents(id,name,role,runtime) VALUES ('a1','Worker','worker','MCP')`,
		`INSERT INTO agent_projects(agent_id,project_id) VALUES ('a1','p1')`,
		`INSERT INTO agent_status_events(agent_id,session_id,from_status,to_status,reason,created_at) VALUES ('a1','session-1','BUSY','ERROR','status.reconciled',CURRENT_TIMESTAMP)`,
	}
	for _, statement := range statements {
		if _, err := database.ExecContext(ctx, statement); err != nil {
			t.Fatalf("execute %q: %v", statement, err)
		}
	}
	repository, err := NewNotificationRepository(database)
	if err != nil {
		t.Fatal(err)
	}
	items, err := repository.List(ctx, "p1", 100, 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 3 {
		t.Fatalf("notification count = %d, want 3", len(items))
	}
	eventTypes := map[string]bool{}
	for _, item := range items {
		eventTypes[item.EventType] = true
	}
	for _, eventType := range []string{"FAILED", "run.failed", "agent.failed"} {
		if !eventTypes[eventType] {
			t.Errorf("missing %s notification", eventType)
		}
	}
}

func TestActiveBrainStatusCreatesProjectNotificationWithoutMembership(t *testing.T) {
	for _, status := range []string{"OFFLINE", "ERROR"} {
		t.Run(status, func(t *testing.T) {
			ctx := context.Background()
			database, err := openTestDatabase(t, ctx, filepath.Join(t.TempDir(), "notifications.db"))
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() { _ = database.Close() })
			// brain-b (the Brain) is configured with agent-a, whose ID differs
			// from the Brain ID, and agent-a has no agent_projects row.
			statements := []string{
				`INSERT INTO agents(id,name,role,runtime) VALUES ('agent-a','External Brain','coordinator','MCP')`,
				`INSERT INTO agent_capabilities(agent_id,capability) VALUES ('agent-a','brain.execute')`,
				`INSERT INTO projects(id,name,status,brain_id,human_review_required) VALUES ('brain-project','Project','CREATED','brain-b',0)`,
				`INSERT INTO brains(id,project_id,active) VALUES ('brain-b','brain-project',1)`,
				`INSERT INTO brain_configuration(brain_id,agent_id,autonomy) VALUES ('brain-b','agent-a','MANUAL')`,
				`INSERT INTO agent_status_events(agent_id,session_id,from_status,to_status,reason,created_at) VALUES ('agent-a','session-1','BUSY','` + status + `','status.reconciled',CURRENT_TIMESTAMP)`,
			}
			for _, statement := range statements {
				if _, err := database.ExecContext(ctx, statement); err != nil {
					t.Fatalf("execute %q: %v", statement, err)
				}
			}
			repository, err := NewNotificationRepository(database)
			if err != nil {
				t.Fatal(err)
			}
			items, err := repository.List(ctx, "brain-project", 100, 0)
			if err != nil {
				t.Fatal(err)
			}
			wantEvent := "agent.disconnected"
			if status == "ERROR" {
				wantEvent = "agent.failed"
			}
			if len(items) != 1 || items[0].EventType != wantEvent || items[0].ResourceID != "agent-a" {
				t.Fatalf("Brain status notifications = %#v, want one %s notification for agent-a", items, wantEvent)
			}
		})
	}
}
