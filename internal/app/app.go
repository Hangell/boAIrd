package app

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/netip"
	"time"

	"boAIrd/internal/agent"
	"boAIrd/internal/api"
	"boAIrd/internal/brain"
	"boAIrd/internal/config"
	"boAIrd/internal/conversation"
	"boAIrd/internal/credential"
	"boAIrd/internal/heartbeat"
	"boAIrd/internal/mcpserver"
	"boAIrd/internal/mcptoken"
	"boAIrd/internal/memory"
	"boAIrd/internal/notification"
	"boAIrd/internal/orchestrator"
	"boAIrd/internal/profile"
	"boAIrd/internal/project"
	"boAIrd/internal/provider"
	"boAIrd/internal/run"
	"boAIrd/internal/runtime"
	"boAIrd/internal/storage"
	"boAIrd/internal/task"
	"boAIrd/internal/template"
	governedtool "boAIrd/internal/tool"
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
	memoryRepository, err := storage.NewMemoryRepository(database)
	if err != nil {
		return fmt.Errorf("initialize memory repository: %w", err)
	}
	memoryService, err := memory.NewService(memoryRepository, projectManager, memory.Config{})
	if err != nil {
		return fmt.Errorf("initialize memory service: %w", err)
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
	agentActivityRepository, err := storage.NewAgentActivityRepository(database)
	if err != nil {
		return fmt.Errorf("initialize agent activity repository: %w", err)
	}
	agentPolicy, err := heartbeat.NewPolicy(cfg.AgentStaleAfter, cfg.AgentOfflineAfter)
	if err != nil {
		return fmt.Errorf("configure agent liveness policy: %w", err)
	}
	heartbeatService, err := heartbeat.NewService(agentActivityRepository, agentPolicy)
	if err != nil {
		return fmt.Errorf("initialize heartbeat service: %w", err)
	}
	agentManager, err := agent.NewManager(agentRepository, heartbeatService)
	if err != nil {
		return fmt.Errorf("initialize agent manager: %w", err)
	}
	mcpTokenRepository, err := storage.NewMCPTokenRepository(database)
	if err != nil {
		return fmt.Errorf("initialize MCP token repository: %w", err)
	}
	mcpTokenService, err := mcptoken.NewService(mcpTokenRepository, agentManager)
	if err != nil {
		return fmt.Errorf("initialize MCP token service: %w", err)
	}
	legacyTokenCount := len(cfg.MCPAgentTokens)
	bootstrapped, err := mcpTokenService.Bootstrap(ctx, cfg.MCPAgentTokens, project.LocalAdministratorID)
	if err != nil {
		return fmt.Errorf("bootstrap legacy MCP tokens: %w", err)
	}
	for i := range cfg.MCPAgentTokens {
		clear(cfg.MCPAgentTokens[i].Token[:])
	}
	cfg.MCPAgentTokens = nil
	if bootstrapped && legacyTokenCount > 0 {
		logger.Info("legacy MCP tokens imported with a 90-day expiration", "registered_agents", legacyTokenCount)
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
	endpointPolicy, err := provider.NewEndpointPolicy(cfg.ProviderAllowedHosts, cfg.ProviderAllowedIPRanges)
	if err != nil {
		return fmt.Errorf("configure provider endpoint policy: %w", err)
	}
	brainService, err := brain.NewService(brainRepository, projectManager, agentManager, runRepository, credentialManager, endpointPolicy)
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
	go reconcileAgentStatus(ctx, heartbeatService, cfg.AgentStatusInterval, logger)
	runtimeRegistry := runtime.NewRegistry()
	apiExecutor, err := runtime.NewAPIExecutor(agentManager, credentialManager, endpointPolicy)
	if err != nil {
		return fmt.Errorf("initialize API agent executor: %w", err)
	}
	mcpRuntime, err := runtime.NewMCPRuntime(agentManager)
	if err != nil {
		return fmt.Errorf("initialize MCP agent runtime: %w", err)
	}
	localRuntime, err := runtime.NewLocalRuntime(agentManager)
	if err != nil {
		return fmt.Errorf("initialize local agent runtime: %w", err)
	}
	remoteRuntime, err := runtime.NewRemoteRuntime(agentManager)
	if err != nil {
		return fmt.Errorf("initialize remote agent runtime: %w", err)
	}
	for _, adapter := range []runtime.AgentRuntime{apiExecutor, mcpRuntime, localRuntime, remoteRuntime} {
		if err := runtimeRegistry.Register(adapter.Runtime(), adapter); err != nil {
			return fmt.Errorf("register agent runtime: %w", err)
		}
	}
	// The orchestrator owns the per-execution deadline; the enforcer applies
	// scope, capability, and error-mapping policy before adapter execution.
	executor, err := runtime.NewEnforcer(runtimeRegistry, agentManager, 0, runtimeExecutionScope{tasks: taskService})
	if err != nil {
		return fmt.Errorf("initialize agent runtime enforcer: %w", err)
	}
	auditRepository, err := storage.NewAuditRepository(database)
	if err != nil {
		return fmt.Errorf("initialize audit repository: %w", err)
	}
	var shellExecutor mcpserver.ToolExecutor
	if cfg.ShellEnabled {
		registry := governedtool.NewRegistry()
		shellTool, err := governedtool.NewShellTool(governedtool.ShellConfig{
			Enabled: true, AllowedExecutables: cfg.ShellAllowedExecutables,
			ResolveWorkspace: func(ctx context.Context, projectID string) (string, error) {
				value, err := projectManager.Get(ctx, projectID)
				if err != nil {
					return "", err
				}
				return value.WorkspacePath(), nil
			},
		})
		if err != nil {
			return fmt.Errorf("configure shell tool: %w", err)
		}
		if err := registry.Register(shellTool); err != nil {
			return fmt.Errorf("register shell tool: %w", err)
		}
		shellExecutor, err = governedtool.NewEnforcer(registry, auditRepository, shellScopeAuthorizer{projects: projectManager})
		if err != nil {
			return fmt.Errorf("configure shell tool enforcement: %w", err)
		}
	}
	mcp, err := mcpserver.New(mcpserver.Services{
		Agents: agentManager, Projects: projectManager, Tasks: taskService, Denials: taskRepository,
		Conversations: conversationService, Brains: brainService, Runs: runManager,
		Heartbeats: heartbeatService, Tools: shellExecutor,
	}, mcpTokenService)
	if err != nil {
		return fmt.Errorf("initialize MCP server: %w", err)
	}
	logger.Info("MCP endpoint initialized", "path", "/mcp")
	profileRepository, err := storage.NewProfileRepository(database)
	if err != nil {
		return fmt.Errorf("initialize profile repository: %w", err)
	}
	profileService, err := profile.NewService(profileRepository, projectManager)
	if err != nil {
		return fmt.Errorf("initialize profile service: %w", err)
	}
	notificationRepository, err := storage.NewNotificationRepository(database)
	if err != nil {
		return fmt.Errorf("initialize notification repository: %w", err)
	}
	notificationService, err := notification.NewService(notificationRepository)
	if err != nil {
		return fmt.Errorf("initialize notification service: %w", err)
	}
	go notificationService.Run(ctx)
	templateRepository, err := storage.NewTemplateRepository(database)
	if err != nil {
		return fmt.Errorf("initialize template repository: %w", err)
	}
	templateService, err := template.NewService(templateRepository, projectManager)
	if err != nil {
		return fmt.Errorf("initialize template service: %w", err)
	}
	orchestratorService, err := orchestrator.New(orchestrator.Dependencies{
		Tasks: taskService, Runs: runRepository, Agents: agentManager, Projects: projectManager,
		Brains: brainRepository, Executor: executor, Conversations: conversationService, ContextReader: conversationService,
		Memory: memoryService, Summarizer: brainService,
		Escalations: auditRepository, Advisor: brainAdvisor{service: brainService},
		Config: orchestrator.Config{
			MaxAttempts: cfg.OrchestratorMaxAttempts, ExecutionTimeout: cfg.OrchestratorTimeout,
			ContextMaxBytes: cfg.ContextMaxBytes, ContextMaxTokens: cfg.ContextMaxTokens,
		},
	})
	if err != nil {
		return fmt.Errorf("initialize orchestrator: %w", err)
	}
	policy, err := apiSecurityPolicy(cfg)
	if err != nil {
		return fmt.Errorf("configure HTTP security policy: %w", err)
	}
	handler := api.NewHandlerWithSecurityNotificationsAndTemplates(database, projectManager, credentialManager, agentManager, brainService, runManager, taskService, conversationService, cfg.LocalAuthToken[:], mcpTokenService, orchestratorService, profileService, auditRepository, notificationService, templateService, policy, mcp.Handler())
	if cfg.TLSCertFile != "" {
		if err := api.ServeTLS(ctx, cfg.Address(), cfg.TLSCertFile, cfg.TLSKeyFile, handler, logger); err != nil {
			return fmt.Errorf("run HTTPS API: %w", err)
		}
		return nil
	}
	if err := api.Serve(ctx, cfg.Address(), handler, logger); err != nil {
		return fmt.Errorf("run HTTP API: %w", err)
	}
	return nil
}

// apiSecurityPolicy translates remote-mode configuration into request-boundary
// protections. Forwarded headers are never trusted unless explicitly allowed.
func apiSecurityPolicy(cfg config.Config) (api.SecurityPolicy, error) {
	proxies := make([]netip.Prefix, 0, len(cfg.TrustedProxies))
	for _, value := range cfg.TrustedProxies {
		prefix, err := netip.ParsePrefix(value)
		if err != nil {
			address, addrErr := netip.ParseAddr(value)
			if addrErr != nil {
				return api.SecurityPolicy{}, fmt.Errorf("parse trusted proxy %q: %w", value, err)
			}
			prefix = netip.PrefixFrom(address, address.BitLen())
		}
		proxies = append(proxies, prefix)
	}
	return api.SecurityPolicy{
		TrustedProxies:    proxies,
		RateLimitRequests: cfg.RateLimitRequests,
		RateLimitWindow:   cfg.RateLimitWindow,
	}, nil
}

// reconcileAgentStatus expires silent sessions and records observed Agent
// status transitions on a bounded, cancellable schedule.
func reconcileAgentStatus(ctx context.Context, service *heartbeat.Service, interval time.Duration, logger *slog.Logger) {
	if err := service.Reconcile(ctx); err != nil && ctx.Err() == nil {
		logger.Error("reconcile agent status", "error", err)
	}
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			if err := service.Reconcile(ctx); err != nil && ctx.Err() == nil {
				logger.Error("reconcile agent status", "error", err)
			}
		}
	}
}

// brainAdvisor lets the configured project Brain analyze worker failures before a retry.
type brainAdvisor struct{ service *brain.Service }

func (a brainAdvisor) Analyze(ctx context.Context, projectID, taskID, objective string) (string, error) {
	execution, err := a.service.AnalyzeTaskFailure(ctx, projectID, taskID, objective, 1024)
	if err != nil {
		return "", err
	}
	return execution.Content, nil
}
