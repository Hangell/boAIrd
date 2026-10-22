package storage

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"boAIrd/internal/conversation"
)

type ConversationRepository struct{ database *sql.DB }

func NewConversationRepository(database *sql.DB) (*ConversationRepository, error) {
	if database == nil {
		return nil, errors.New("database is required")
	}
	return &ConversationRepository{database: database}, nil
}

func (r *ConversationRepository) GetByTask(ctx context.Context, taskID string) (conversation.Conversation, error) {
	var id, projectID string
	err := r.database.QueryRowContext(ctx, "SELECT id, project_id FROM conversations WHERE task_id=?", taskID).Scan(&id, &projectID)
	if errors.Is(err, sql.ErrNoRows) {
		return conversation.Conversation{}, conversation.ErrNotFound
	}
	if err != nil {
		return conversation.Conversation{}, fmt.Errorf("read task conversation: %w", err)
	}
	value, err := conversation.New(id, projectID, taskID)
	if err != nil {
		return conversation.Conversation{}, fmt.Errorf("restore conversation: %w", err)
	}
	return value, nil
}

func (r *ConversationRepository) AppendMessage(ctx context.Context, value conversation.Message) error {
	_, err := r.database.ExecContext(ctx, `INSERT INTO conversation_messages(id, conversation_id, sender_type, sender_id, content, content_type, metadata, created_at)
		VALUES(?,?,?,?,?,?,?,?)`, value.ID(), value.ConversationID(), value.SenderType(), value.SenderID(), value.Content(), value.ContentType(),
		string(value.Metadata()), formatDatabaseTime(value.CreatedAt()))
	if err != nil {
		return fmt.Errorf("insert conversation message: %w", err)
	}
	return nil
}

func (r *ConversationRepository) ListMessages(ctx context.Context, conversationID string) ([]conversation.Message, error) {
	rows, err := r.database.QueryContext(ctx, `SELECT id, conversation_id, sender_type, sender_id, content, content_type, metadata, created_at
		FROM conversation_messages WHERE conversation_id=? ORDER BY created_at, id`, conversationID)
	if err != nil {
		return nil, fmt.Errorf("query conversation messages: %w", err)
	}
	defer rows.Close()
	values := make([]conversation.Message, 0)
	for rows.Next() {
		var id, parentID, senderID, content, contentType, metadata, created string
		var senderType conversation.SenderType
		if err := rows.Scan(&id, &parentID, &senderType, &senderID, &content, &contentType, &metadata, &created); err != nil {
			return nil, fmt.Errorf("scan conversation message: %w", err)
		}
		createdAt, err := parseDatabaseTime(created)
		if err != nil {
			return nil, fmt.Errorf("parse conversation message time: %w", err)
		}
		value, err := conversation.NewMessageWithMetadata(id, parentID, senderType, senderID, content, contentType, []byte(metadata), createdAt)
		if err != nil {
			return nil, fmt.Errorf("restore conversation message: %w", err)
		}
		values = append(values, value)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate conversation messages: %w", err)
	}
	return values, nil
}
