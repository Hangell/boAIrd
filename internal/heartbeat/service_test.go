package heartbeat_test

import (
	"context"
	"database/sql"
	"errors"
	"path/filepath"
	"testing"
	"time"

	"boAIrd/internal/agent"
	"boAIrd/internal/heartbeat"
	"boAIrd/internal/mcptoken"
	"boAIrd/internal/storage"
)

type fixture struct {
	t         *testing.T
	database  *sql.DB
	service   *heartbeat.Service
	agents    *agent.Manager
	tokens    *mcptoken.Service
	now       time.Time
	agentID   string
	otherID   string
	tokenID   string
	otherTok  string
	projectID string
}

func newFixture(t *testing.T) *fixture {
	t.Helper()
	ctx := context.Background()
	database, err := storage.OpenSQLite(ctx, filepath.Join(t.TempDir(), "boaird.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := database.Close(); err != nil {
			t.Errorf("close test database: %v", err)
		}
	})

	agentRepository, err := storage.NewAgentRepository(database)
	if err != nil {
		t.Fatal(err)
	}
	activityRepository, err := storage.NewAgentActivityRepository(database)
	if err != nil {
		t.Fatal(err)
	}
	policy, err := heartbeat.NewPolicy(heartbeat.DefaultStaleAfter, heartbeat.DefaultOfflineAfter)
	if err != nil {
		t.Fatal(err)
	}
	values := map[string]string{"agent-one": "External One", "agent-two": "External Two"}
	for id, name := range values {
		value, err := agent.NewConfigured(id, agent.Configuration{Name: name, Role: "worker", Runtime: agent.RuntimeMCP})
		if err != nil {
			t.Fatal(err)
		}
		if err := agentRepository.Create(ctx, value); err != nil {
			t.Fatal(err)
		}
	}
	tokenRepository, err := storage.NewMCPTokenRepository(database)
	if err != nil {
		t.Fatal(err)
	}
	tokenAgentManager, err := agent.NewManager(agentRepository)
	if err != nil {
		t.Fatal(err)
	}
	tokens, err := mcptoken.NewService(tokenRepository, tokenAgentManager)
	if err != nil {
		t.Fatal(err)
	}
	first, _, err := tokens.Create(ctx, "agent-one", time.Hour, "local-administrator")
	if err != nil {
		t.Fatal(err)
	}
	second, _, err := tokens.Create(ctx, "agent-two", time.Hour, "local-administrator")
	if err != nil {
		t.Fatal(err)
	}
	if first.AgentID != "agent-one" || second.AgentID != "agent-two" {
		t.Fatalf("issued tokens = %+v / %+v", first, second)
	}

	if _, err := database.ExecContext(ctx, `INSERT INTO projects(id, name, description, status, human_review_required, created_at, updated_at)
		VALUES ('project-1', 'Heartbeat project', '', 'CREATED', 0, '2026-10-22T12:00:00Z', '2026-10-22T12:00:00Z')`); err != nil {
		t.Fatal(err)
	}
	f := &fixture{
		t: t, database: database, tokens: tokens, now: time.Date(2026, 10, 22, 12, 0, 0, 0, time.UTC),
		agentID: "agent-one", otherID: "agent-two",
		tokenID: first.ID, otherTok: second.ID, projectID: "project-1",
	}
	service, err := heartbeat.NewService(activityRepository, policy, heartbeat.WithClock(func() time.Time { return f.now }))
	if err != nil {
		t.Fatal(err)
	}
	manager, err := agent.NewManager(agentRepository, service)
	if err != nil {
		t.Fatal(err)
	}
	f.service, f.agents = service, manager
	return f
}

func (f *fixture) advance(value time.Duration) {
	f.now = f.now.Add(value)
}

func (f *fixture) identity() heartbeat.Identity {
	return heartbeat.Identity{AgentID: f.agentID, TokenID: f.tokenID}
}

func (f *fixture) heartbeat() agent.Observed {
	f.t.Helper()
	observed, err := f.service.Heartbeat(context.Background(), f.identity())
	if err != nil {
		f.t.Fatalf("Heartbeat() error = %v", err)
	}
	return observed
}

func (f *fixture) status(agentID string) agent.Status {
	f.t.Helper()
	values, err := f.service.Statuses(context.Background(), []string{agentID})
	if err != nil {
		f.t.Fatalf("Statuses() error = %v", err)
	}
	return values[agentID].Status
}

func (f *fixture) storedStatus(agentID string) string {
	f.t.Helper()
	var value string
	if err := f.database.QueryRowContext(context.Background(), "SELECT status FROM agents WHERE id = ?", agentID).Scan(&value); err != nil {
		f.t.Fatal(err)
	}
	return value
}

func (f *fixture) addTask(id string, status string) {
	f.t.Helper()
	stamp := f.now.UTC().Format(time.RFC3339Nano)
	if _, err := f.database.ExecContext(context.Background(), `INSERT INTO tasks
		(id, project_id, title, description, status, priority, created_by_type, created_by, assigned_to, human_review, created_at, updated_at, last_actor_type, last_actor_id, last_reason, version)
		VALUES (?, ?, 'Task', '', ?, 'NORMAL', 'AGENT', ?, ?, 0, ?, ?, 'AGENT', ?, 'work', 1)`,
		id, f.projectID, status, f.agentID, f.agentID, stamp, stamp, f.agentID); err != nil {
		f.t.Fatal(err)
	}
}

func (f *fixture) setTaskStatus(id, status string) {
	f.t.Helper()
	stamp := f.now.UTC().Format(time.RFC3339Nano)
	if _, err := f.database.ExecContext(context.Background(), "UPDATE tasks SET status = ?, updated_at = ? WHERE id = ?", status, stamp, id); err != nil {
		f.t.Fatal(err)
	}
}

func (f *fixture) events(agentID string) []heartbeat.StatusEvent {
	f.t.Helper()
	events, err := f.service.Events(context.Background(), agentID, 50)
	if err != nil {
		f.t.Fatalf("Events() error = %v", err)
	}
	return events
}

func (f *fixture) sessionCount(agentID string) int {
	f.t.Helper()
	var count int
	if err := f.database.QueryRowContext(context.Background(), "SELECT COUNT(*) FROM agent_sessions WHERE agent_id = ?", agentID).Scan(&count); err != nil {
		f.t.Fatal(err)
	}
	return count
}

func (f *fixture) sessionID(tokenID string) string {
	f.t.Helper()
	var id string
	if err := f.database.QueryRowContext(context.Background(), "SELECT id FROM agent_sessions WHERE agent_id = ? AND token_id = ? AND ended_at IS NULL", f.agentID, tokenID).Scan(&id); err != nil {
		f.t.Fatal(err)
	}
	return id
}

func TestHeartbeatOpensSessionAndRecordsTransition(t *testing.T) {
	f := newFixture(t)
	if status := f.status(f.agentID); status != agent.StatusIdle {
		t.Fatalf("status before first heartbeat = %s, want IDLE", status)
	}
	observed := f.heartbeat()
	if observed.Status != agent.StatusOnline || !observed.SessionOpen || observed.LastSeenAt == nil {
		t.Fatalf("first heartbeat = %+v", observed)
	}
	if status := f.storedStatus(f.agentID); status != string(agent.StatusOnline) {
		t.Fatalf("stored status = %s, want ONLINE", status)
	}
	events := f.events(f.agentID)
	if len(events) != 1 || events[0].From != agent.StatusIdle || events[0].To != agent.StatusOnline || events[0].Reason != heartbeat.ReasonSessionOpened {
		t.Fatalf("events = %+v", events)
	}

	f.advance(10 * time.Second)
	if observed := f.heartbeat(); observed.Status != agent.StatusOnline {
		t.Fatalf("refreshed heartbeat = %+v", observed)
	}
	if count := f.sessionCount(f.agentID); count != 1 {
		t.Fatalf("session count = %d, want the same session refreshed", count)
	}
	if events := f.events(f.agentID); len(events) != 1 {
		t.Fatalf("unchanged status recorded %d events, want 1", len(events))
	}
}

func TestSilenceBecomesStaleThenOfflineAndReconnects(t *testing.T) {
	f := newFixture(t)
	f.heartbeat()

	f.advance(heartbeat.DefaultStaleAfter)
	if err := f.service.Reconcile(context.Background()); err != nil {
		t.Fatalf("Reconcile() error = %v", err)
	}
	if status := f.storedStatus(f.agentID); status != string(agent.StatusStale) {
		t.Fatalf("stored status after stale threshold = %s, want STALE", status)
	}

	f.advance(heartbeat.DefaultOfflineAfter - heartbeat.DefaultStaleAfter)
	if err := f.service.Reconcile(context.Background()); err != nil {
		t.Fatalf("Reconcile() error = %v", err)
	}
	if status := f.storedStatus(f.agentID); status != string(agent.StatusOffline) {
		t.Fatalf("stored status after offline threshold = %s, want OFFLINE", status)
	}
	var open int
	if err := f.database.QueryRowContext(context.Background(), "SELECT COUNT(*) FROM agent_sessions WHERE agent_id = ? AND ended_at IS NULL", f.agentID).Scan(&open); err != nil {
		t.Fatal(err)
	}
	if open != 0 {
		t.Fatalf("open sessions after expiry = %d, want 0", open)
	}
	if err := f.service.Reconcile(context.Background()); err != nil {
		t.Fatalf("second Reconcile() error = %v", err)
	}
	if events := f.events(f.agentID); len(events) != 3 {
		t.Fatalf("events after expiry = %d, want 3 (open, stale, offline): %+v", len(events), events)
	}

	f.advance(time.Minute)
	observed := f.heartbeat()
	if observed.Status != agent.StatusOnline {
		t.Fatalf("reconnect status = %s, want ONLINE", observed.Status)
	}
	if count := f.sessionCount(f.agentID); count != 2 {
		t.Fatalf("session count after reconnect = %d, want 2", count)
	}
	events := f.events(f.agentID)
	if events[0].From != agent.StatusOffline || events[0].To != agent.StatusOnline || events[0].Reason != heartbeat.ReasonSessionResumed {
		t.Fatalf("reconnect event = %+v", events[0])
	}
	var expiredSessionID string
	if err := f.database.QueryRowContext(context.Background(), "SELECT id FROM agent_sessions WHERE agent_id = ? AND token_id = ? AND ended_at IS NOT NULL ORDER BY started_at LIMIT 1", f.agentID, f.tokenID).Scan(&expiredSessionID); err != nil {
		t.Fatal(err)
	}
	if events[1].Reason != heartbeat.ReasonSessionExpired || events[1].SessionID != expiredSessionID {
		t.Fatalf("expiry event = %+v, want session %s and reason %s", events[1], expiredSessionID, heartbeat.ReasonSessionExpired)
	}
}

func TestReconcileAttributesWorkTransitionToStillOpenSession(t *testing.T) {
	f := newFixture(t)
	f.heartbeat()
	firstSessionID := f.sessionID(f.tokenID)

	secondToken, _, err := f.tokens.Create(context.Background(), f.agentID, time.Hour, "local-administrator")
	if err != nil {
		t.Fatal(err)
	}
	f.advance(time.Second)
	if _, err := f.service.Heartbeat(context.Background(), heartbeat.Identity{AgentID: f.agentID, TokenID: secondToken.ID}); err != nil {
		t.Fatalf("second credential heartbeat: %v", err)
	}
	secondSessionID := f.sessionID(secondToken.ID)
	if secondSessionID == firstSessionID {
		t.Fatalf("sessions share ID %q", secondSessionID)
	}

	// Expire only the first credential while the second remains fresh.
	expiredAt := f.now.Add(-heartbeat.DefaultOfflineAfter).UTC().Format(time.RFC3339Nano)
	if _, err := f.database.ExecContext(context.Background(), "UPDATE agent_sessions SET last_seen_at = ? WHERE id = ?", expiredAt, firstSessionID); err != nil {
		t.Fatal(err)
	}
	f.addTask("task-working", "WORKING")
	if err := f.service.Reconcile(context.Background()); err != nil {
		t.Fatalf("Reconcile() error = %v", err)
	}

	events := f.events(f.agentID)
	if len(events) < 2 {
		t.Fatalf("events = %+v, want the initial and reconciled transitions", events)
	}
	got := events[0]
	if got.From != agent.StatusOnline || got.To != agent.StatusBusy || got.Reason != heartbeat.ReasonWorkChanged || got.SessionID != secondSessionID {
		t.Fatalf("reconciled event = %+v, want ONLINE -> BUSY, reason %s, session %s", got, heartbeat.ReasonWorkChanged, secondSessionID)
	}
}

func TestSessionIsBoundToAgentAndCredential(t *testing.T) {
	f := newFixture(t)
	f.heartbeat()

	// A second credential for the same Agent opens a distinct session, never
	// an alias for the first one.
	otherToken, _, err := f.tokens.Create(context.Background(), f.agentID, time.Hour, "local-administrator")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := f.service.Heartbeat(context.Background(), heartbeat.Identity{AgentID: f.agentID, TokenID: otherToken.ID}); err != nil {
		t.Fatal(err)
	}
	if count := f.sessionCount(f.agentID); count != 2 {
		t.Fatalf("session count = %d, want 2 distinct credentials", count)
	}
	if count := f.sessionCount(f.otherID); count != 0 {
		t.Fatalf("unrelated Agent sessions = %d, want 0", count)
	}

	// An identity without the matching token never reaches another Agent's session.
	_, err = f.service.Heartbeat(context.Background(), heartbeat.Identity{AgentID: f.otherID, TokenID: "missing-token"})
	if err == nil {
		t.Fatal("Heartbeat() accepted an identity without a matching credential")
	}
	if count := f.sessionCount(f.otherID); count != 0 {
		t.Fatalf("spoofed identity created %d sessions", count)
	}
}

func TestWorkSignalsDriveBusyWaitingAndError(t *testing.T) {
	f := newFixture(t)
	f.addTask("task-1", "WORKING")
	if observed := f.heartbeat(); observed.Status != agent.StatusBusy {
		t.Fatalf("WORKING task status = %s, want BUSY", observed.Status)
	}

	f.setTaskStatus("task-1", "REVIEW")
	if observed := f.heartbeat(); observed.Status != agent.StatusWaiting {
		t.Fatalf("REVIEW task status = %s, want WAITING", observed.Status)
	}

	f.setTaskStatus("task-1", "FAILED")
	if observed := f.heartbeat(); observed.Status != agent.StatusError {
		t.Fatalf("FAILED task status = %s, want ERROR", observed.Status)
	}

	f.setTaskStatus("task-1", "DONE")
	f.advance(heartbeat.DefaultStaleAfter)
	if observed := f.heartbeat(); observed.Status != agent.StatusOnline {
		t.Fatalf("DONE task status = %s, want ONLINE", observed.Status)
	}

	events := f.events(f.agentID)
	transitions := make([]string, 0, len(events))
	for _, event := range events {
		transitions = append(transitions, string(event.From)+"->"+string(event.To))
	}
	want := []string{"ERROR->ONLINE", "WAITING->ERROR", "BUSY->WAITING", "IDLE->BUSY"}
	if len(transitions) != len(want) {
		t.Fatalf("transitions = %v, want %v", transitions, want)
	}
	for i := range want {
		if transitions[i] != want[i] {
			t.Fatalf("transitions = %v, want %v", transitions, want)
		}
	}
}

func TestHeartbeatRejectsMalformedIdentity(t *testing.T) {
	f := newFixture(t)
	for name, identity := range map[string]heartbeat.Identity{
		"missing agent":   {TokenID: "token"},
		"missing token":   {AgentID: "agent-one"},
		"padded agent":    {AgentID: " agent-one", TokenID: "token"},
		"oversized agent": {AgentID: string(make([]byte, 201)), TokenID: "token"},
		"oversized token": {AgentID: "agent-one", TokenID: string(make([]byte, 201))},
	} {
		t.Run(name, func(t *testing.T) {
			if _, err := f.service.Heartbeat(context.Background(), identity); !errors.Is(err, heartbeat.ErrInvalidInput) {
				t.Fatalf("Heartbeat() error = %v, want ErrInvalidInput", err)
			}
		})
	}
}

func TestEventsAreBounded(t *testing.T) {
	f := newFixture(t)
	f.heartbeat()
	if _, err := f.service.Events(context.Background(), f.agentID, 0); err != nil {
		t.Fatalf("Events(limit 0) error = %v", err)
	}
	if _, err := f.service.Events(context.Background(), f.agentID, 101); !errors.Is(err, heartbeat.ErrInvalidInput) {
		t.Fatalf("Events(limit 101) error = %v, want ErrInvalidInput", err)
	}
	if _, err := f.service.Events(context.Background(), "", 10); !errors.Is(err, heartbeat.ErrInvalidInput) {
		t.Fatalf("Events(empty agent) error = %v, want ErrInvalidInput", err)
	}
}

func TestStatusEventsAreImmutable(t *testing.T) {
	f := newFixture(t)
	f.heartbeat()
	if _, err := f.database.ExecContext(context.Background(), "UPDATE agent_status_events SET to_status = 'OFFLINE'"); err == nil {
		t.Fatal("status events were mutable")
	}
	if _, err := f.database.ExecContext(context.Background(), "DELETE FROM agent_status_events"); err == nil {
		t.Fatal("status events were deletable")
	}
}

func TestObservedStateIsExposedThroughAgentManager(t *testing.T) {
	f := newFixture(t)
	f.heartbeat()
	values, err := f.agents.Observe(context.Background(), []string{f.agentID})
	if err != nil {
		t.Fatal(err)
	}
	observed, ok := values[f.agentID]
	if !ok || observed.Status != agent.StatusOnline || !observed.SessionOpen {
		t.Fatalf("Observe() = %+v", values)
	}
}

func TestConfigurationUpdateKeepsObservedStatus(t *testing.T) {
	f := newFixture(t)
	f.heartbeat()
	name := "Renamed Agent"
	if _, err := f.agents.Update(context.Background(), f.agentID, agent.UpdateInput{Name: &name}); err != nil {
		t.Fatalf("Update() error = %v", err)
	}
	if status := f.storedStatus(f.agentID); status != string(agent.StatusOnline) {
		t.Fatalf("stored status after configuration update = %s, want ONLINE", status)
	}
}
