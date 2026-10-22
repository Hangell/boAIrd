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
)

const (
	defaultHost         = "127.0.0.1"
	defaultPort         = 3110
	defaultDatabasePath = "./data/boaird.db"
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
		Host:         envOrDefault("BOAIRD_HOST", defaultHost),
		Port:         defaultPort,
		DatabasePath: envOrDefault("BOAIRD_DATABASE_PATH", defaultDatabasePath),
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
	if address == nil || !address.IsLoopback() {
		return fmt.Errorf("BOAIRD_HOST must be a loopback IP address until authenticated remote mode is implemented")
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

func validProviderHost(host string) bool {
	if host == "" || strings.ContainsAny(host, "/@?#\t\r\n *") {
		return false
	}
	if address, err := netip.ParseAddr(host); err == nil {
		if address.Zone() != "" {
			return false
		}
		return true
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

func envOrDefault(key, fallback string) string {
	value, ok := os.LookupEnv(key)
	if !ok {
		return fallback
	}
	return value
}
