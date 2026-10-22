package conversation

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"
	"time"

	"boAIrd/internal/agent"
	"boAIrd/internal/brain"
	"boAIrd/internal/project"
	"boAIrd/internal/task"
)

var (
	ErrNotFound        = errors.New("conversation not found")
	ErrForbidden       = errors.New("conversation sender is not a project participant")
	ErrInvalidInput    = errors.New("invalid conversation input")
	ErrProjectArchived = errors.New("project is archived")
)

type Repository interface {
	GetByTask(context.Context, string) (Conversation, error)
	AppendMessage(context.Context, Message) error
	ListMessages(context.Context, string) ([]Message, error)
}

type TaskReader interface {
	Get(context.Context, string) (task.Task, error)
}
type ProjectReader interface {
	Get(context.Context, string) (project.Project, error)
}
type AgentReader interface {
	Get(context.Context, string) (agent.Agent, error)
}
type BrainReader interface {
	GetActiveByProject(context.Context, string) (brain.Brain, error)
}

type Service struct {
	repository Repository
	tasks      TaskReader
	projects   ProjectReader
	agents     AgentReader
	brains     BrainReader
}

type MessageInput struct {
	Content     string
	ContentType string
	Metadata    []byte
}

func NewService(repository Repository, tasks TaskReader, projects ProjectReader, agents AgentReader, brains BrainReader) (*Service, error) {
	if repository == nil || tasks == nil || projects == nil || agents == nil || brains == nil {
		return nil, errors.New("conversation service dependencies are required")
	}
	return &Service{repository: repository, tasks: tasks, projects: projects, agents: agents, brains: brains}, nil
}

func (s *Service) Get(ctx context.Context, taskID string) (Conversation, []Message, error) {
	work, err := s.tasks.Get(ctx, strings.TrimSpace(taskID))
	if err != nil {
		return Conversation{}, nil, mapTaskError(err)
	}
	value, err := s.repository.GetByTask(ctx, work.ID())
	if err != nil {
		return Conversation{}, nil, err
	}
	messages, err := s.repository.ListMessages(ctx, value.ID())
	if err != nil {
		return Conversation{}, nil, err
	}
	return value, messages, nil
}

// Append trusts sender identity only when supplied by an in-process caller; HTTP handlers never accept it from request data.
func (s *Service) Append(ctx context.Context, taskID string, senderType SenderType, senderID string, input MessageInput) (Message, error) {
	work, err := s.tasks.Get(ctx, strings.TrimSpace(taskID))
	if err != nil {
		return Message{}, mapTaskError(err)
	}
	p, err := s.projects.Get(ctx, work.ProjectID())
	if err != nil {
		if errors.Is(err, project.ErrNotFound) {
			return Message{}, task.ErrProjectNotFound
		}
		return Message{}, err
	}
	if p.Status() == project.StatusArchived {
		return Message{}, ErrProjectArchived
	}
	if err := s.authorizeSender(ctx, work, senderType, strings.TrimSpace(senderID)); err != nil {
		return Message{}, err
	}
	value, err := s.repository.GetByTask(ctx, work.ID())
	if err != nil {
		return Message{}, err
	}
	messageID, err := newConversationID()
	if err != nil {
		return Message{}, fmt.Errorf("generate message ID: %w", err)
	}
	message, err := NewMessageWithMetadata(messageID, value.ID(), senderType, senderID, input.Content, input.ContentType, input.Metadata, time.Now().UTC())
	if err != nil {
		return Message{}, fmt.Errorf("%w: %w", ErrInvalidInput, err)
	}
	if err := s.repository.AppendMessage(ctx, message); err != nil {
		return Message{}, err
	}
	return message, nil
}

func (s *Service) authorizeSender(ctx context.Context, value task.Task, senderType SenderType, senderID string) error {
	if senderID == "" {
		return ErrForbidden
	}
	switch senderType {
	case SenderUser:
		if senderID == "local-api" {
			return nil
		}
	case SenderAgent:
		if senderID != value.AssignedTo() {
			return ErrForbidden
		}
		if _, err := s.agents.Get(ctx, senderID); err != nil {
			return ErrForbidden
		}
		return nil
	case SenderBrain:
		configured, err := s.brains.GetActiveByProject(ctx, value.ProjectID())
		if err == nil && configured.AgentID() == senderID {
			return nil
		}
	case SenderSystem:
		return ErrForbidden
	}
	return ErrForbidden
}

func mapTaskError(err error) error {
	if errors.Is(err, task.ErrNotFound) {
		return ErrNotFound
	}
	return err
}

func newConversationID() (string, error) {
	var value [16]byte
	if _, err := rand.Read(value[:]); err != nil {
		return "", err
	}
	return hex.EncodeToString(value[:]), nil
}
