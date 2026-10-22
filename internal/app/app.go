package app

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"

	"boAIrd/internal/agent"
	"boAIrd/internal/api"
	"boAIrd/internal/brain"
	"boAIrd/internal/config"
	"boAIrd/internal/conversation"
	"boAIrd/internal/credential"
	"boAIrd/internal/mcpserver"
	"boAIrd/internal/project"
	"boAIrd/internal/run"
	"boAIrd/internal/storage"
	"boAIrd/internal/task"
)

// Run opens service dependencies and serves HTTP until the context is cancelled.
func Run(ctx context.Context, cfg config.Config, logger *slog.Logger) (result error) {
	if err := cfg.Validate(); err != nil {
		return fmt.Errorf("validate configuration: %w", err)
	}
	defer clear(cfg.MasterKey[:])
	defer clear(cfg.LocalAuthToken[:])
	defer func() {
		for i := range cfg.MCPAgentTokens {
			clear(cfg.MCPAgentTokens[i].Token[:])
		}
	}()

	database, err := storage.OpenSQLite(ctx, cfg.DatabasePath)
	if err != nil {
		return fmt.Errorf("initialize storage: %w", err)
	}
	defer func() {
		if err := database.Close(); err != nil {
			result = errors.Join(result, fmt.Errorf("close SQLite database: %w", err))
		}
	}()

	logger.Info("SQLite database ready", "path", cfg.DatabasePath)
	projectRepository, err := storage.NewProjectRepository(database)
	if err != nil {
		return fmt.Errorf("initialize project repository: %w", err)
	}
	projectManager, err := project.NewManager(projectRepository)
	if err != nil {
		return fmt.Errorf("initialize project manager: %w", err)
	}
	credentialRepository, err := storage.NewCredentialRepository(database)
	if err != nil {
		return fmt.Errorf("initialize credential repository: %w", err)
	}
	credentialManager, err := credential.NewManager(credentialRepository, cfg.MasterKey[:])
	if err != nil {
		return fmt.Errorf("initialize credential manager: %w", err)
	}
	agentRepository, err := storage.NewAgentRepository(database)
	if err != nil {
		return fmt.Errorf("initialize agent repository: %w", err)
	}
	agentManager, err := agent.NewManager(agentRepository)
	if err != nil {
		return fmt.Errorf("initialize agent manager: %w", err)
	}
	brainRepository, err := storage.NewBrainRepository(database)
	if err != nil {
		return fmt.Errorf("initialize Brain repository: %w", err)
	}
	runRepository, err := storage.NewRunRepository(database)
	if err != nil {
		return fmt.Errorf("initialize run repository: %w", err)
	}
	runManager, err := run.NewManager(runRepository)
	if err != nil {
		return fmt.Errorf("initialize run manager: %w", err)
	}
	brainService, err := brain.NewService(brainRepository, projectManager, agentManager, runRepository, credentialManager)
	if err != nil {
		return fmt.Errorf("initialize Brain service: %w", err)
	}
	taskRepository, err := storage.NewTaskRepository(database)
	if err != nil {
		return fmt.Errorf("initialize task repository: %w", err)
	}
	taskService, err := task.NewService(taskRepository, projectManager, agentManager, brainRepository)
	if err != nil {
		return fmt.Errorf("initialize task service: %w", err)
	}
	conversationRepository, err := storage.NewConversationRepository(database)
	if err != nil {
		return fmt.Errorf("initialize conversation repository: %w", err)
	}
	conversationService, err := conversation.NewService(conversationRepository, taskService, projectManager, agentManager, brainRepository)
	if err != nil {
		return fmt.Errorf("initialize conversation service: %w", err)
	}
	var mcpHandler http.Handler
	if len(cfg.MCPAgentTokens) > 0 {
		mcp, err := mcpserver.New(ctx, mcpserver.Services{Agents: agentManager, Projects: projectManager, Tasks: taskService, Conversations: conversationService, Brains: brainService, Runs: runManager}, cfg.MCPAgentTokens)
		if err != nil {
			return fmt.Errorf("initialize MCP server: %w", err)
		}
		mcpHandler = mcp.Handler()
		logger.Info("MCP endpoint enabled", "path", "/mcp", "registered_agents", len(cfg.MCPAgentTokens))
	}
	if err := api.Serve(ctx, cfg.Address(), api.NewHandlerWithConversations(database, projectManager, credentialManager, agentManager, brainService, runManager, taskService, conversationService, cfg.LocalAuthToken[:], mcpHandler), logger); err != nil {
		return fmt.Errorf("run HTTP API: %w", err)
	}
	return nil
}
