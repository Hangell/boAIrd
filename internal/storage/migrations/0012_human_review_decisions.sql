CREATE TRIGGER tasks_audit_human_review_decision
AFTER UPDATE OF status ON tasks
WHEN OLD.status = 'HUMAN_REVIEW' AND NEW.status IN ('DONE', 'WORKING')
BEGIN
    INSERT INTO audit_events(project_id, task_id, actor_type, actor_id, event_type, resource_type, resource_id, from_status, to_status, reason)
    VALUES (
        NEW.project_id,
        NEW.id,
        NEW.last_actor_type,
        NEW.last_actor_id,
        CASE WHEN NEW.status = 'DONE' THEN 'task.human_review.approved' ELSE 'task.human_review.rejected' END,
        'task',
        NEW.id,
        OLD.status,
        NEW.status,
        NEW.last_reason
    );
END;
