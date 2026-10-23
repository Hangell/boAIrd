package config

import (
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net"
	"net/netip"
	"os"
	"strconv"
	"strings"
	"time"
)

const (
	defaultHost         = "127.0.0.1"
	defaultPort         = 3110
	defaultDatabasePath = "./data/boaird.db"

	defaultAgentStaleAfter     = 90 * time.Second
	defaultAgentOfflineAfter   = 5 * time.Minute
	defaultAgentStatusInterval = 30 * time.Second

	defaultOrchestratorMaxAttempts      = 3
	maximumOrchestratorMaxAttempts      = 10
	defaultOrchestratorExecutionTimeout = 5 * time.Minute
	maximumOrchestratorExecutionTimeout = time.Hour
	defaultContextMaxBytes              = 16 << 10
	maximumContextMaxBytes              = 1 << 20
	defaultContextMaxTokens             = 4096
	maximumContextMaxTokens             = 100_000

	defaultRateLimitRequests = 60
	defaultRateLimitWindow   = time.Minute
	maximumRateLimitRequests = 1_000_000
)

// Config contains the settings required to start the local service.
type Config struct {
	Host                    string
	Port                    int
	DatabasePath            string
	MasterKey               MasterKey
	LocalAuthToken          LocalAuthToken
	MCPAgentTokens          []MCPAgentToken
	ProviderAllowedHosts    []string
	ProviderAllowedIPRanges []string
	AgentStaleAfter         time.Duration
	AgentOfflineAfter       time.Duration
	AgentStatusInterval     time.Duration
	OrchestratorMaxAttempts int
	OrchestratorTimeout     time.Duration
	ContextMaxBytes         int
	ContextMaxTokens        int
	ShellEnabled            bool
	ShellAllowedExecutables []string
	GitEnabled              bool
	GitAllowPush            bool
	GitAllowMerge           bool
	RemoteMode              bool
	TLSCertFile             string
	TLSKeyFile              string
	TrustedProxies          []string
	RateLimitRequests       int
	RateLimitWindow         time.Duration
}

// MCPAgentToken registers one MCP bearer token for an existing Agent identity.
type MCPAgentToken struct {
	AgentID string   `json:"agent_id"`
	Token   [32]byte `json:"-"`
}

func (MCPAgentToken) String() string   { return "[REDACTED]" }
func (MCPAgentToken) GoString() string { return "[REDACTED]" }

// MasterKey is the deployment-supplied key used to encrypt provider credentials.
type MasterKey [32]byte

func (MasterKey) String() string   { return "[REDACTED]" }
func (MasterKey) GoString() string { return "[REDACTED]" }

// LocalAuthToken authenticates the configured local administrator to sensitive conversation routes.
type LocalAuthToken [32]byte

func (LocalAuthToken) String() string   { return "[REDACTED]" }
func (LocalAuthToken) GoString() string { return "[REDACTED]" }

// Load reads service settings from the environment and applies local defaults.
func Load() (Config, error) {
	cfg := Config{
		Host:                envOrDefault("BOAIRD_HOST", defaultHost),
		Port:                defaultPort,
		DatabasePath:        envOrDefault("BOAIRD_DATABASE_PATH", defaultDatabasePath),
		AgentStaleAfter:     defaultAgentStaleAfter,
		AgentOfflineAfter:   defaultAgentOfflineAfter,
		AgentStatusInterval: defaultAgentStatusInterval,
		ContextMaxBytes:     defaultContextMaxBytes,
		ContextMaxTokens:    defaultContextMaxTokens,
	}

	if rawPort, ok := os.LookupEnv("BOAIRD_PORT"); ok {
		port, err := strconv.Atoi(rawPort)
		if err != nil {
			return Config{}, fmt.Errorf("parse BOAIRD_PORT: %w", err)
		}
		cfg.Port = port
	}
	rawMasterKey, ok := os.LookupEnv("BOAIRD_MASTER_KEY")
	if !ok || len(rawMasterKey) != 64 {
		return Config{}, fmt.Errorf("BOAIRD_MASTER_KEY must be 64 hexadecimal characters (32 bytes)")
	}
	key, err := hex.DecodeString(rawMasterKey)
	if err != nil {
		return Config{}, fmt.Errorf("BOAIRD_MASTER_KEY must be 64 hexadecimal characters (32 bytes)")
	}
	copy(cfg.MasterKey[:], key)
	clear(key)
	rawLocalAuthToken, ok := os.LookupEnv("BOAIRD_LOCAL_AUTH_TOKEN")
	if !ok || len(rawLocalAuthToken) != 64 {
		return Config{}, fmt.Errorf("BOAIRD_LOCAL_AUTH_TOKEN must be 64 hexadecimal characters (32 bytes)")
	}
	token, err := hex.DecodeString(rawLocalAuthToken)
	if err != nil {
		return Config{}, fmt.Errorf("BOAIRD_LOCAL_AUTH_TOKEN must be 64 hexadecimal characters (32 bytes)")
	}
	copy(cfg.LocalAuthToken[:], token)
	clear(token)
	if rawTokens, ok := os.LookupEnv("BOAIRD_MCP_AGENT_TOKENS"); ok && rawTokens != "" {
		var entries []struct {
			AgentID string `json:"agent_id"`
			Token   string `json:"token"`
		}
		if err := json.Unmarshal([]byte(rawTokens), &entries); err != nil {
			return Config{}, fmt.Errorf("BOAIRD_MCP_AGENT_TOKENS must be a JSON array of agent_id/token objects")
		}
		cfg.MCPAgentTokens = make([]MCPAgentToken, 0, len(entries))
		for _, entry := range entries {
			if len(entry.Token) != 64 {
				return Config{}, fmt.Errorf("each BOAIRD_MCP_AGENT_TOKENS token must be 64 hexadecimal characters")
			}
			decoded, err := hex.DecodeString(entry.Token)
			if err != nil {
				return Config{}, fmt.Errorf("each BOAIRD_MCP_AGENT_TOKENS token must be 64 hexadecimal characters")
			}
			var configured MCPAgentToken
			configured.AgentID = entry.AgentID
			copy(configured.Token[:], decoded)
			clear(decoded)
			cfg.MCPAgentTokens = append(cfg.MCPAgentTokens, configured)
		}
	}
	cfg.ProviderAllowedHosts = splitList(os.Getenv("BOAIRD_PROVIDER_ALLOWED_HOSTS"))
	cfg.ProviderAllowedIPRanges = splitList(os.Getenv("BOAIRD_PROVIDER_ALLOWED_IP_RANGES"))
	if rawEnabled, ok := os.LookupEnv("BOAIRD_SHELL_ENABLED"); ok {
		enabled, err := strconv.ParseBool(strings.TrimSpace(rawEnabled))
		if err != nil {
			return Config{}, fmt.Errorf("parse BOAIRD_SHELL_ENABLED: %w", err)
		}
		cfg.ShellEnabled = enabled
	}
	cfg.ShellAllowedExecutables = splitExactList(os.Getenv("BOAIRD_SHELL_ALLOWED_EXECUTABLES"))
	var boolErr error
	cfg.GitEnabled, boolErr = boolEnv("BOAIRD_GIT_ENABLED")
	if boolErr != nil {
		return Config{}, boolErr
	}
	cfg.GitAllowPush, boolErr = boolEnv("BOAIRD_GIT_ALLOW_PUSH")
	if boolErr != nil {
		return Config{}, boolErr
	}
	cfg.GitAllowMerge, boolErr = boolEnv("BOAIRD_GIT_ALLOW_MERGE")
	if boolErr != nil {
		return Config{}, boolErr
	}
	var durationErr error
	if cfg.AgentStaleAfter, durationErr = envDuration("BOAIRD_AGENT_STALE_AFTER", defaultAgentStaleAfter); durationErr != nil {
		return Config{}, durationErr
	}
	if cfg.AgentOfflineAfter, durationErr = envDuration("BOAIRD_AGENT_OFFLINE_AFTER", defaultAgentOfflineAfter); durationErr != nil {
		return Config{}, durationErr
	}
	if cfg.AgentStatusInterval, durationErr = envDuration("BOAIRD_AGENT_STATUS_INTERVAL", defaultAgentStatusInterval); durationErr != nil {
		return Config{}, durationErr
	}
	cfg.OrchestratorMaxAttempts = defaultOrchestratorMaxAttempts
	cfg.OrchestratorTimeout = defaultOrchestratorExecutionTimeout
	if rawAttempts, ok := os.LookupEnv("BOAIRD_ORCHESTRATOR_MAX_ATTEMPTS"); ok {
		attempts, err := strconv.Atoi(rawAttempts)
		if err != nil {
			return Config{}, fmt.Errorf("parse BOAIRD_ORCHESTRATOR_MAX_ATTEMPTS: %w", err)
		}
		cfg.OrchestratorMaxAttempts = attempts
	}
	if rawTimeout, ok := os.LookupEnv("BOAIRD_ORCHESTRATOR_TIMEOUT"); ok {
		timeout, err := time.ParseDuration(rawTimeout)
		if err != nil {
			return Config{}, fmt.Errorf("parse BOAIRD_ORCHESTRATOR_TIMEOUT: %w", err)
		}
		cfg.OrchestratorTimeout = timeout
	}
	if rawLimit, ok := os.LookupEnv("BOAIRD_CONTEXT_MAX_BYTES"); ok {
		limit, err := strconv.Atoi(rawLimit)
		if err != nil {
			return Config{}, fmt.Errorf("parse BOAIRD_CONTEXT_MAX_BYTES: %w", err)
		}
		cfg.ContextMaxBytes = limit
	}
	if rawLimit, ok := os.LookupEnv("BOAIRD_CONTEXT_MAX_TOKENS"); ok {
		limit, err := strconv.Atoi(rawLimit)
		if err != nil {
			return Config{}, fmt.Errorf("parse BOAIRD_CONTEXT_MAX_TOKENS: %w", err)
		}
		cfg.ContextMaxTokens = limit
	}

	if rawMode, ok := os.LookupEnv("BOAIRD_REMOTE_MODE"); ok {
		mode, err := strconv.ParseBool(rawMode)
		if err != nil {
			return Config{}, fmt.Errorf("parse BOAIRD_REMOTE_MODE: %w", err)
		}
		cfg.RemoteMode = mode
	}
	cfg.TLSCertFile = os.Getenv("BOAIRD_TLS_CERT_FILE")
	cfg.TLSKeyFile = os.Getenv("BOAIRD_TLS_KEY_FILE")
	cfg.TrustedProxies = splitList(os.Getenv("BOAIRD_TRUSTED_PROXIES"))
	if rawLimit, ok := os.LookupEnv("BOAIRD_RATE_LIMIT_REQUESTS"); ok {
		limit, err := strconv.Atoi(rawLimit)
		if err != nil {
			return Config{}, fmt.Errorf("parse BOAIRD_RATE_LIMIT_REQUESTS: %w", err)
		}
		cfg.RateLimitRequests = limit
	}
	if rawWindow, ok := os.LookupEnv("BOAIRD_RATE_LIMIT_WINDOW"); ok {
		window, err := time.ParseDuration(rawWindow)
		if err != nil {
			return Config{}, fmt.Errorf("parse BOAIRD_RATE_LIMIT_WINDOW: %w", err)
		}
		cfg.RateLimitWindow = window
	}
	if cfg.RemoteMode {
		if cfg.RateLimitRequests == 0 {
			cfg.RateLimitRequests = defaultRateLimitRequests
		}
		if cfg.RateLimitWindow == 0 {
			cfg.RateLimitWindow = defaultRateLimitWindow
		}
	}

	if err := cfg.Validate(); err != nil {
		return Config{}, err
	}
	return cfg, nil
}

// Validate checks settings that affect the listener and database path.
func (c Config) Validate() error {
	if c.MasterKey == (MasterKey{}) {
		return fmt.Errorf("BOAIRD_MASTER_KEY must be configured")
	}
	if c.LocalAuthToken == (LocalAuthToken{}) {
		return fmt.Errorf("BOAIRD_LOCAL_AUTH_TOKEN must be configured")
	}
	if c.MasterKey == MasterKey(c.LocalAuthToken) {
		return fmt.Errorf("BOAIRD_LOCAL_AUTH_TOKEN must be different from BOAIRD_MASTER_KEY")
	}
	if c.Host == "" {
		return fmt.Errorf("BOAIRD_HOST must not be empty")
	}
	address := net.ParseIP(c.Host)
	if address == nil {
		return fmt.Errorf("BOAIRD_HOST must be an IP address")
	}
	if !address.IsLoopback() && !c.RemoteMode {
		return fmt.Errorf("BOAIRD_HOST must be a loopback IP address unless BOAIRD_REMOTE_MODE is enabled")
	}
	if c.RemoteMode && (c.TLSCertFile == "" || c.TLSKeyFile == "") {
		return fmt.Errorf("BOAIRD_REMOTE_MODE requires BOAIRD_TLS_CERT_FILE and BOAIRD_TLS_KEY_FILE")
	}
	if (c.TLSCertFile == "") != (c.TLSKeyFile == "") {
		return fmt.Errorf("BOAIRD_TLS_CERT_FILE and BOAIRD_TLS_KEY_FILE must be configured together")
	}
	for _, value := range c.TrustedProxies {
		if !validTrustedProxy(value) {
			return fmt.Errorf("BOAIRD_TRUSTED_PROXIES must contain IP addresses or CIDR ranges")
		}
	}
	if c.RateLimitRequests < 0 || c.RateLimitRequests > maximumRateLimitRequests {
		return fmt.Errorf("BOAIRD_RATE_LIMIT_REQUESTS must be between 0 and %d", maximumRateLimitRequests)
	}
	if c.RateLimitRequests > 0 && c.RateLimitWindow < time.Second {
		return fmt.Errorf("BOAIRD_RATE_LIMIT_WINDOW must be at least 1s when rate limiting is enabled")
	}
	if c.Port < 1 || c.Port > 65535 {
		return fmt.Errorf("BOAIRD_PORT must be between 1 and 65535")
	}
	if c.DatabasePath == "" {
		return fmt.Errorf("BOAIRD_DATABASE_PATH must not be empty")
	}
	for _, host := range c.ProviderAllowedHosts {
		if !validProviderHost(host) {
			return fmt.Errorf("BOAIRD_PROVIDER_ALLOWED_HOSTS must contain exact hostnames or IP addresses")
		}
	}
	for _, value := range c.ProviderAllowedIPRanges {
		if _, err := netip.ParsePrefix(value); err != nil {
			return fmt.Errorf("BOAIRD_PROVIDER_ALLOWED_IP_RANGES must contain valid CIDR ranges")
		}
	}
	if c.AgentStaleAfter < time.Second || c.AgentStaleAfter >= c.AgentOfflineAfter {
		return fmt.Errorf("BOAIRD_AGENT_STALE_AFTER must be at least 1s and shorter than BOAIRD_AGENT_OFFLINE_AFTER")
	}
	if c.AgentOfflineAfter > 24*time.Hour {
		return fmt.Errorf("BOAIRD_AGENT_OFFLINE_AFTER must not exceed 24h")
	}
	if c.AgentStatusInterval < time.Second || c.AgentStatusInterval > time.Hour {
		return fmt.Errorf("BOAIRD_AGENT_STATUS_INTERVAL must be between 1s and 1h")
	}
	if c.OrchestratorMaxAttempts < 0 || c.OrchestratorMaxAttempts > maximumOrchestratorMaxAttempts {
		return fmt.Errorf("BOAIRD_ORCHESTRATOR_MAX_ATTEMPTS must be between 1 and %d", maximumOrchestratorMaxAttempts)
	}
	if c.OrchestratorTimeout < 0 || c.OrchestratorTimeout > maximumOrchestratorExecutionTimeout {
		return fmt.Errorf("BOAIRD_ORCHESTRATOR_TIMEOUT must be positive and at most %s", maximumOrchestratorExecutionTimeout)
	}
	if c.ContextMaxBytes < 512 || c.ContextMaxBytes > maximumContextMaxBytes {
		return fmt.Errorf("BOAIRD_CONTEXT_MAX_BYTES must be between 512 and %d", maximumContextMaxBytes)
	}
	if c.ContextMaxTokens < 128 || c.ContextMaxTokens > maximumContextMaxTokens {
		return fmt.Errorf("BOAIRD_CONTEXT_MAX_TOKENS must be between 128 and %d", maximumContextMaxTokens)
	}
	if c.ShellEnabled && len(c.ShellAllowedExecutables) == 0 {
		return fmt.Errorf("BOAIRD_SHELL_ALLOWED_EXECUTABLES must be set when BOAIRD_SHELL_ENABLED is true")
	}
	if len(c.ShellAllowedExecutables) > 64 {
		return fmt.Errorf("BOAIRD_SHELL_ALLOWED_EXECUTABLES must contain at most 64 executable names")
	}
	if c.GitAllowPush && !c.GitEnabled {
		return fmt.Errorf("BOAIRD_GIT_ALLOW_PUSH requires BOAIRD_GIT_ENABLED")
	}
	if c.GitAllowMerge && !c.GitEnabled {
		return fmt.Errorf("BOAIRD_GIT_ALLOW_MERGE requires BOAIRD_GIT_ENABLED")
	}
	seenAgents := make(map[string]struct{}, len(c.MCPAgentTokens))
	seenTokens := make(map[[32]byte]struct{}, len(c.MCPAgentTokens))
	for _, entry := range c.MCPAgentTokens {
		if strings.TrimSpace(entry.AgentID) == "" || strings.TrimSpace(entry.AgentID) != entry.AgentID || entry.Token == ([32]byte{}) {
			return fmt.Errorf("BOAIRD_MCP_AGENT_TOKENS entries require an agent_id and non-zero token")
		}
		if _, exists := seenAgents[entry.AgentID]; exists {
			return fmt.Errorf("BOAIRD_MCP_AGENT_TOKENS agent_id values must be unique")
		}
		if _, exists := seenTokens[entry.Token]; exists {
			return fmt.Errorf("BOAIRD_MCP_AGENT_TOKENS values must be unique")
		}
		if MasterKey(entry.Token) == c.MasterKey || LocalAuthToken(entry.Token) == c.LocalAuthToken {
			return fmt.Errorf("MCP agent tokens must be different from administrator and master-key secrets")
		}
		seenAgents[entry.AgentID] = struct{}{}
		seenTokens[entry.Token] = struct{}{}
	}
	return nil
}

func envDuration(name string, fallback time.Duration) (time.Duration, error) {
	raw, ok := os.LookupEnv(name)
	if !ok || strings.TrimSpace(raw) == "" {
		return fallback, nil
	}
	value, err := time.ParseDuration(strings.TrimSpace(raw))
	if err != nil {
		return 0, fmt.Errorf("parse %s: %w", name, err)
	}
	return value, nil
}

func boolEnv(name string) (bool, error) {
	raw, ok := os.LookupEnv(name)
	if !ok || strings.TrimSpace(raw) == "" {
		return false, nil
	}
	value, err := strconv.ParseBool(strings.TrimSpace(raw))
	if err != nil {
		return false, fmt.Errorf("parse %s: %w", name, err)
	}
	return value, nil
}

func splitList(value string) []string {
	if strings.TrimSpace(value) == "" {
		return nil
	}
	parts := strings.Split(value, ",")
	result := make([]string, 0, len(parts))
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part != "" {
			result = append(result, strings.ToLower(strings.TrimSuffix(part, ".")))
		}
	}
	return result
}

func splitExactList(value string) []string {
	if strings.TrimSpace(value) == "" {
		return nil
	}
	parts := strings.Split(value, ",")
	result := make([]string, 0, len(parts))
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part != "" {
			result = append(result, part)
		}
	}
	return result
}

func validProviderHost(host string) bool {
	if host == "" || strings.ContainsAny(host, "/@?#\t\r\n *") {
		return false
	}
	if address, err := netip.ParseAddr(host); err == nil {
		return address.Zone() == ""
	}
	if len(host) > 253 {
		return false
	}
	for _, label := range strings.Split(host, ".") {
		if label == "" || len(label) > 63 || label[0] == '-' || label[len(label)-1] == '-' {
			return false
		}
		for _, char := range label {
			if (char < 'a' || char > 'z') && (char < '0' || char > '9') && char != '-' {
				return false
			}
		}
	}
	return true
}

// Address returns the TCP address used by the HTTP server.
func (c Config) Address() string {
	return net.JoinHostPort(c.Host, strconv.Itoa(c.Port))
}

func validTrustedProxy(value string) bool {
	if _, err := netip.ParsePrefix(value); err == nil {
		return true
	}
	address, err := netip.ParseAddr(value)
	return err == nil && address.Zone() == ""
}

func envOrDefault(key, fallback string) string {
	value, ok := os.LookupEnv(key)
	if !ok {
		return fallback
	}
	return value
}
