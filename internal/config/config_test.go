package config

import (
	"os"
	"strings"
	"testing"
	"time"
)

func TestLoadUsesLocalDefaults(t *testing.T) {
	unsetEnvironment(t, "BOAIRD_HOST", "BOAIRD_PORT", "BOAIRD_DATABASE_PATH", "BOAIRD_DATABASE_DRIVER", "BOAIRD_DATABASE_URL", "BOAIRD_MCP_AGENT_TOKENS", "BOAIRD_PROVIDER_ALLOWED_HOSTS", "BOAIRD_PROVIDER_ALLOWED_IP_RANGES",
		"BOAIRD_AGENT_STALE_AFTER", "BOAIRD_AGENT_OFFLINE_AFTER", "BOAIRD_AGENT_STATUS_INTERVAL",
		"BOAIRD_ORCHESTRATOR_MAX_ATTEMPTS", "BOAIRD_ORCHESTRATOR_TIMEOUT")
	unsetEnvironment(t, "BOAIRD_CONTEXT_MAX_BYTES", "BOAIRD_CONTEXT_MAX_TOKENS", "BOAIRD_SHELL_ENABLED", "BOAIRD_SHELL_ALLOWED_EXECUTABLES")
	unsetEnvironment(t, "BOAIRD_GIT_ENABLED", "BOAIRD_GIT_ALLOW_PUSH", "BOAIRD_GIT_ALLOW_MERGE")
	t.Setenv("BOAIRD_MASTER_KEY", testMasterKey())
	t.Setenv("BOAIRD_LOCAL_AUTH_TOKEN", testLocalAuthToken())

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if cfg.Host != defaultHost || cfg.Port != defaultPort || cfg.DatabasePath != defaultDatabasePath || cfg.DatabaseDriver != defaultDatabaseDriver {
		t.Fatalf("Load() = %+v, want local defaults", cfg)
	}
	if cfg.AgentStaleAfter != defaultAgentStaleAfter || cfg.AgentOfflineAfter != defaultAgentOfflineAfter || cfg.AgentStatusInterval != defaultAgentStatusInterval {
		t.Fatalf("agent liveness defaults = %s/%s/%s", cfg.AgentStaleAfter, cfg.AgentOfflineAfter, cfg.AgentStatusInterval)
	}
	if cfg.OrchestratorMaxAttempts != defaultOrchestratorMaxAttempts || cfg.OrchestratorTimeout != defaultOrchestratorExecutionTimeout {
		t.Fatalf("orchestrator defaults = %d/%s", cfg.OrchestratorMaxAttempts, cfg.OrchestratorTimeout)
	}
	if cfg.ContextMaxBytes != defaultContextMaxBytes || cfg.ContextMaxTokens != defaultContextMaxTokens {
		t.Fatalf("context defaults = %d bytes/%d tokens", cfg.ContextMaxBytes, cfg.ContextMaxTokens)
	}
	if cfg.ShellEnabled || len(cfg.ShellAllowedExecutables) != 0 {
		t.Fatalf("shell defaults = enabled %t, allowed %v; want disabled without executables", cfg.ShellEnabled, cfg.ShellAllowedExecutables)
	}
	if cfg.GitEnabled || cfg.GitAllowPush || cfg.GitAllowMerge {
		t.Fatalf("git defaults = enabled %t, push %t, merge %t; want disabled and denied", cfg.GitEnabled, cfg.GitAllowPush, cfg.GitAllowMerge)
	}
}

func TestLoadParsesShellPolicyWithoutChangingExecutableNames(t *testing.T) {
	setValidSecrets(t)
	t.Setenv("BOAIRD_SHELL_ENABLED", "true")
	t.Setenv("BOAIRD_SHELL_ALLOWED_EXECUTABLES", "/usr/local/bin/Go, npm")
	cfg, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	if !cfg.ShellEnabled || len(cfg.ShellAllowedExecutables) != 2 || cfg.ShellAllowedExecutables[0] != "/usr/local/bin/Go" || cfg.ShellAllowedExecutables[1] != "npm" {
		t.Fatalf("shell policy = enabled %t, allowlist %q", cfg.ShellEnabled, cfg.ShellAllowedExecutables)
	}
}

func TestLoadRejectsShellEnabledWithoutAllowlist(t *testing.T) {
	setValidSecrets(t)
	t.Setenv("BOAIRD_SHELL_ENABLED", "true")
	t.Setenv("BOAIRD_SHELL_ALLOWED_EXECUTABLES", "")
	if _, err := Load(); err == nil {
		t.Fatal("Load() accepted enabled shell without an executable allowlist")
	}
}

func TestLoadRejectsMalformedShellEnabled(t *testing.T) {
	setValidSecrets(t)
	t.Setenv("BOAIRD_SHELL_ENABLED", "sometimes")
	if _, err := Load(); err == nil {
		t.Fatal("Load() accepted a malformed shell enable flag")
	}
}

func TestLoadParsesGitPolicy(t *testing.T) {
	setValidSecrets(t)
	t.Setenv("BOAIRD_GIT_ENABLED", "true")
	t.Setenv("BOAIRD_GIT_ALLOW_PUSH", "true")
	cfg, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	if !cfg.GitEnabled || !cfg.GitAllowPush || cfg.GitAllowMerge {
		t.Fatalf("git policy = enabled %t, push %t, merge %t", cfg.GitEnabled, cfg.GitAllowPush, cfg.GitAllowMerge)
	}
}

func TestLoadRejectsGitPushWithoutEnablement(t *testing.T) {
	setValidSecrets(t)
	t.Setenv("BOAIRD_GIT_ALLOW_PUSH", "true")
	if _, err := Load(); err == nil {
		t.Fatal("Load() accepted git push without git enablement")
	}
}

func TestLoadRejectsMalformedGitEnabled(t *testing.T) {
	setValidSecrets(t)
	t.Setenv("BOAIRD_GIT_ENABLED", "sometimes")
	if _, err := Load(); err == nil {
		t.Fatal("Load() accepted a malformed git enable flag")
	}
}

func TestLoadParsesContextLimits(t *testing.T) {
	setValidSecrets(t)
	t.Setenv("BOAIRD_CONTEXT_MAX_BYTES", "32768")
	t.Setenv("BOAIRD_CONTEXT_MAX_TOKENS", "8192")
	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if cfg.ContextMaxBytes != 32768 || cfg.ContextMaxTokens != 8192 {
		t.Fatalf("context limits = %d bytes/%d tokens", cfg.ContextMaxBytes, cfg.ContextMaxTokens)
	}
}

func TestLoadRejectsInvalidContextLimits(t *testing.T) {
	for name, values := range map[string][2]string{
		"bytes below minimum":   {"BOAIRD_CONTEXT_MAX_BYTES", "256"},
		"bytes above maximum":   {"BOAIRD_CONTEXT_MAX_BYTES", "2097152"},
		"malformed byte limit":  {"BOAIRD_CONTEXT_MAX_BYTES", "large"},
		"tokens below minimum":  {"BOAIRD_CONTEXT_MAX_TOKENS", "64"},
		"tokens above maximum":  {"BOAIRD_CONTEXT_MAX_TOKENS", "100001"},
		"malformed token limit": {"BOAIRD_CONTEXT_MAX_TOKENS", "many"},
	} {
		t.Run(name, func(t *testing.T) {
			setValidSecrets(t)
			t.Setenv(values[0], values[1])
			if _, err := Load(); err == nil {
				t.Fatal("Load() accepted invalid context limits")
			}
		})
	}
}

func TestLoadParsesOrchestratorSettings(t *testing.T) {
	setValidSecrets(t)
	t.Setenv("BOAIRD_ORCHESTRATOR_MAX_ATTEMPTS", "5")
	t.Setenv("BOAIRD_ORCHESTRATOR_TIMEOUT", "90s")
	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if cfg.OrchestratorMaxAttempts != 5 || cfg.OrchestratorTimeout != 90*time.Second {
		t.Fatalf("orchestrator settings = %d/%s", cfg.OrchestratorMaxAttempts, cfg.OrchestratorTimeout)
	}
}

func TestLoadRejectsInvalidOrchestratorSettings(t *testing.T) {
	for name, environment := range map[string]map[string]string{
		"attempts above maximum": {"BOAIRD_ORCHESTRATOR_MAX_ATTEMPTS": "11"},
		"negative attempts":      {"BOAIRD_ORCHESTRATOR_MAX_ATTEMPTS": "-1"},
		"malformed attempts":     {"BOAIRD_ORCHESTRATOR_MAX_ATTEMPTS": "three"},
		"negative timeout":       {"BOAIRD_ORCHESTRATOR_TIMEOUT": "-1s"},
		"timeout above maximum":  {"BOAIRD_ORCHESTRATOR_TIMEOUT": "2h"},
		"malformed timeout":      {"BOAIRD_ORCHESTRATOR_TIMEOUT": "soon"},
	} {
		t.Run(name, func(t *testing.T) {
			setValidSecrets(t)
			for key, value := range environment {
				t.Setenv(key, value)
			}
			if _, err := Load(); err == nil {
				t.Fatal("expected invalid orchestrator settings to be rejected")
			}
		})
	}
}

func TestLoadRegistersMCPAgentTokenWithoutExposingIt(t *testing.T) {
	setValidSecrets(t)
	t.Setenv("BOAIRD_MCP_AGENT_TOKENS", `[{"agent_id":"agent-1","token":"0303030303030303030303030303030303030303030303030303030303030303"}]`)
	cfg, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	if len(cfg.MCPAgentTokens) != 1 || cfg.MCPAgentTokens[0].AgentID != "agent-1" || cfg.MCPAgentTokens[0].Token[0] != 3 {
		t.Fatalf("MCP agent token was not decoded: %+v", cfg.MCPAgentTokens)
	}
	if strings.Contains(cfg.MCPAgentTokens[0].String(), "030303") || strings.Contains(cfg.MCPAgentTokens[0].GoString(), "030303") {
		t.Fatal("MCP agent token formatting exposed the secret")
	}
}

func TestLoadAcceptsConfiguredAgentPolicy(t *testing.T) {
	setValidSecrets(t)
	t.Setenv("BOAIRD_AGENT_STALE_AFTER", "2m")
	t.Setenv("BOAIRD_AGENT_OFFLINE_AFTER", "10m")
	t.Setenv("BOAIRD_AGENT_STATUS_INTERVAL", "15s")
	cfg, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	if cfg.AgentStaleAfter != 2*time.Minute || cfg.AgentOfflineAfter != 10*time.Minute || cfg.AgentStatusInterval != 15*time.Second {
		t.Fatalf("configured agent policy = %s/%s/%s", cfg.AgentStaleAfter, cfg.AgentOfflineAfter, cfg.AgentStatusInterval)
	}
}

func TestLoadRejectsInvalidAgentPolicy(t *testing.T) {
	for name, values := range map[string][3]string{
		"stale equals offline":   {"5m", "5m", "30s"},
		"stale below minimum":    {"500ms", "5m", "30s"},
		"offline above maximum":  {"90s", "25h", "30s"},
		"interval below minimum": {"90s", "5m", "0s"},
		"malformed duration":     {"ninety", "5m", "30s"},
	} {
		t.Run(name, func(t *testing.T) {
			setValidSecrets(t)
			t.Setenv("BOAIRD_AGENT_STALE_AFTER", values[0])
			t.Setenv("BOAIRD_AGENT_OFFLINE_AFTER", values[1])
			t.Setenv("BOAIRD_AGENT_STATUS_INTERVAL", values[2])
			if _, err := Load(); err == nil {
				t.Fatal("Load() accepted an invalid agent liveness policy")
			}
		})
	}
}

func TestLoadRejectsInvalidMCPAgentTokens(t *testing.T) {
	setValidSecrets(t)
	for name, value := range map[string]string{
		"malformed JSON":  `not-json`,
		"invalid hex":     `[{"agent_id":"agent-1","token":"not-hex"}]`,
		"duplicate agent": `[{"agent_id":"agent-1","token":"0303030303030303030303030303030303030303030303030303030303030303"},{"agent_id":"agent-1","token":"0404040404040404040404040404040404040404040404040404040404040404"}]`,
		"duplicate token": `[{"agent_id":"agent-1","token":"0303030303030303030303030303030303030303030303030303030303030303"},{"agent_id":"agent-2","token":"0303030303030303030303030303030303030303030303030303030303030303"}]`,
	} {
		t.Run(name, func(t *testing.T) {
			t.Setenv("BOAIRD_MCP_AGENT_TOKENS", value)
			if _, err := Load(); err == nil {
				t.Fatal("Load() accepted invalid MCP token configuration")
			}
		})
	}
}

func setValidSecrets(t *testing.T) {
	t.Helper()
	t.Setenv("BOAIRD_MASTER_KEY", testMasterKey())
	t.Setenv("BOAIRD_LOCAL_AUTH_TOKEN", testLocalAuthToken())
	t.Setenv("BOAIRD_MCP_AGENT_TOKENS", "")
	t.Setenv("BOAIRD_PROVIDER_ALLOWED_HOSTS", "")
	t.Setenv("BOAIRD_PROVIDER_ALLOWED_IP_RANGES", "")
}

func TestLoadUsesConfiguredValues(t *testing.T) {
	t.Setenv("BOAIRD_HOST", defaultHost)
	t.Setenv("BOAIRD_PORT", "3110")
	t.Setenv("BOAIRD_DATABASE_PATH", defaultDatabasePath)
	t.Setenv("BOAIRD_MASTER_KEY", testMasterKey())
	t.Setenv("BOAIRD_LOCAL_AUTH_TOKEN", testLocalAuthToken())
	t.Setenv("BOAIRD_MCP_AGENT_TOKENS", "")
	t.Setenv("BOAIRD_PROVIDER_ALLOWED_HOSTS", "models.example, 10.4.0.7")
	t.Setenv("BOAIRD_PROVIDER_ALLOWED_IP_RANGES", "10.4.0.0/24, fd00::/8")
	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if cfg.Address() != "127.0.0.1:3110" {
		t.Fatalf("Address() = %q, want %q", cfg.Address(), "127.0.0.1:3110")
	}
	if strings.Join(cfg.ProviderAllowedHosts, ",") != "models.example,10.4.0.7" || strings.Join(cfg.ProviderAllowedIPRanges, ",") != "10.4.0.0/24,fd00::/8" {
		t.Fatalf("provider endpoint policy not parsed: hosts=%v ranges=%v", cfg.ProviderAllowedHosts, cfg.ProviderAllowedIPRanges)
	}
}

func TestLoadRejectsInvalidProviderEndpointPolicy(t *testing.T) {
	setValidSecrets(t)
	for name, env := range map[string]string{
		"wildcard host":       "*.example.com",
		"URL instead of host": "https://provider.example/v1",
		"invalid IP range":    "10.1.0.1/99",
	} {
		t.Run(name, func(t *testing.T) {
			t.Setenv("BOAIRD_PROVIDER_ALLOWED_HOSTS", "")
			t.Setenv("BOAIRD_PROVIDER_ALLOWED_IP_RANGES", "")
			if name == "invalid IP range" {
				t.Setenv("BOAIRD_PROVIDER_ALLOWED_IP_RANGES", env)
			} else {
				t.Setenv("BOAIRD_PROVIDER_ALLOWED_HOSTS", env)
			}
			if _, err := Load(); err == nil {
				t.Fatal("Load() accepted invalid provider endpoint policy")
			}
		})
	}
}

func unsetEnvironment(t *testing.T, keys ...string) {
	t.Helper()
	for _, key := range keys {
		value, ok := os.LookupEnv(key)
		if !ok {
			continue
		}
		if err := os.Unsetenv(key); err != nil {
			t.Fatalf("unset %s: %v", key, err)
		}
		t.Cleanup(func() {
			if err := os.Setenv(key, value); err != nil {
				t.Errorf("restore %s: %v", key, err)
			}
		})
	}
}

func TestLoadRejectsInvalidPort(t *testing.T) {
	t.Setenv("BOAIRD_MASTER_KEY", testMasterKey())
	t.Setenv("BOAIRD_LOCAL_AUTH_TOKEN", testLocalAuthToken())
	t.Setenv("BOAIRD_PORT", "not-a-port")
	if _, err := Load(); err == nil {
		t.Fatal("Load() accepted an invalid port")
	}
}

func testConfig(host string) Config {
	return Config{
		Host: host, Port: defaultPort, DatabasePath: defaultDatabasePath, DatabaseDriver: "sqlite", MasterKey: MasterKey{1}, LocalAuthToken: LocalAuthToken{2},
		AgentStaleAfter: defaultAgentStaleAfter, AgentOfflineAfter: defaultAgentOfflineAfter, AgentStatusInterval: defaultAgentStatusInterval,
		ContextMaxBytes: defaultContextMaxBytes, ContextMaxTokens: defaultContextMaxTokens,
	}
}

func TestValidateDatabaseDriverConfiguration(t *testing.T) {
	t.Setenv("PGSSLMODE", "")
	t.Setenv("PGSERVICE", "")
	tests := []struct {
		name string
		edit func(*Config)
		want bool
	}{
		{name: "default SQLite", want: true},
		{name: "unsupported driver", edit: func(cfg *Config) { cfg.DatabaseDriver = "mysql" }},
		{name: "PostgreSQL missing URL", edit: func(cfg *Config) { cfg.DatabaseDriver = "postgres" }},
		{name: "PostgreSQL valid URL", edit: func(cfg *Config) {
			cfg.DatabaseDriver = "postgres"
			cfg.DatabaseURL = "postgres://user:secret@localhost/boaird?sslmode=disable"
		}, want: true},
		{name: "remote PostgreSQL requires certificate verification", edit: func(cfg *Config) {
			cfg.DatabaseDriver = "postgres"
			cfg.DatabaseURL = "postgres://user:secret@db.example/boaird?sslmode=require"
		}},
		{name: "remote PostgreSQL requires hostname verification", edit: func(cfg *Config) {
			cfg.DatabaseDriver = "postgres"
			cfg.DatabaseURL = "postgres://user:secret@db.example/boaird?sslmode=verify-ca"
		}},
		{name: "remote PostgreSQL verified TLS", edit: func(cfg *Config) {
			cfg.DatabaseDriver = "postgres"
			cfg.DatabaseURL = "postgres://user:secret@db.example/boaird?sslmode=verify-full"
		}, want: true},
		{name: "local URL host cannot hide remote query host", edit: func(cfg *Config) {
			cfg.DatabaseDriver = "postgres"
			cfg.DatabaseURL = "postgres://user:secret@localhost/boaird?host=db.example.com&sslmode=disable"
		}},
		{name: "remote query host with hostname verification", edit: func(cfg *Config) {
			cfg.DatabaseDriver = "postgres"
			cfg.DatabaseURL = "postgres://user:secret@localhost/boaird?host=db.example.com&sslmode=verify-full"
		}, want: true},
		{name: "remote host fallback cannot use plaintext", edit: func(cfg *Config) {
			cfg.DatabaseDriver = "postgres"
			cfg.DatabaseURL = "postgres://user:secret@localhost/boaird?host=localhost,db.example.com&sslmode=disable"
		}},
		{name: "remote URL is not downgraded by PGSSLMODE", edit: func(cfg *Config) {
			cfg.DatabaseDriver = "postgres"
			cfg.DatabaseURL = "postgres://user:secret@db.example/boaird"
		}},
		{name: "URL with SQLite", edit: func(cfg *Config) { cfg.DatabaseURL = "postgres://localhost/boaird" }},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			cfg := testConfig("127.0.0.1")
			if test.edit != nil {
				test.edit(&cfg)
			}
			err := cfg.Validate()
			if (err == nil) != test.want {
				t.Fatalf("Validate() error = %v, want success %v", err, test.want)
			}
		})
	}
}

func TestValidatePostgresURLTLSOverridesPGSSLMODE(t *testing.T) {
	t.Setenv("PGSSLMODE", "disable")
	t.Setenv("PGSERVICE", "")
	cfg := testConfig("127.0.0.1")
	cfg.DatabaseDriver = "postgres"
	cfg.DatabaseURL = "postgres://user:secret@db.example/boaird?sslmode=verify-full"
	if err := cfg.Validate(); err != nil {
		t.Fatalf("Validate() rejected explicit verify-full URL with PGSSLMODE=disable: %v", err)
	}
}

func TestValidateRejectsOutOfRangePort(t *testing.T) {
	cfg := testConfig("127.0.0.1")
	cfg.Port = 65536
	if err := cfg.Validate(); err == nil {
		t.Fatal("Validate() accepted an out-of-range port")
	}
}

func TestValidateRejectsReusedMasterKeyForLocalAuthToken(t *testing.T) {
	cfg := testConfig(defaultHost)
	cfg.LocalAuthToken = LocalAuthToken{1}
	if err := cfg.Validate(); err == nil {
		t.Fatal("Validate() accepted the master key as the local auth token")
	}
}

func TestValidateRejectsNonLoopbackHosts(t *testing.T) {
	for _, host := range []string{"0.0.0.0", "192.0.2.1", "localhost"} {
		t.Run(host, func(t *testing.T) {
			cfg := testConfig(host)
			if err := cfg.Validate(); err == nil {
				t.Fatalf("Validate() accepted non-loopback host %q", host)
			}
		})
	}
}

func TestValidateAcceptsLoopbackHosts(t *testing.T) {
	for _, host := range []string{"127.0.0.1", "::1"} {
		t.Run(host, func(t *testing.T) {
			cfg := testConfig(host)
			if err := cfg.Validate(); err != nil {
				t.Fatalf("Validate() rejected loopback host %q: %v", host, err)
			}
		})
	}
}

func TestValidateRejectsRemoteModeWithoutTLS(t *testing.T) {
	cfg := testConfig("192.0.2.1")
	cfg.RemoteMode = true
	if err := cfg.Validate(); err == nil {
		t.Fatal("Validate() accepted remote mode without TLS")
	}
}

func TestValidateAcceptsRemoteModeWithTLS(t *testing.T) {
	cfg := testConfig("0.0.0.0")
	cfg.RemoteMode = true
	cfg.TLSCertFile = "/etc/boaird/cert.pem"
	cfg.TLSKeyFile = "/etc/boaird/key.pem"
	if err := cfg.Validate(); err != nil {
		t.Fatalf("Validate() rejected valid remote mode: %v", err)
	}
}

func TestValidateRejectsPartialTLS(t *testing.T) {
	cfg := testConfig("192.0.2.1")
	cfg.RemoteMode = true
	cfg.TLSCertFile = "/etc/boaird/cert.pem"
	if err := cfg.Validate(); err == nil {
		t.Fatal("Validate() accepted a certificate without a key")
	}
}

func TestValidateRejectsInvalidTrustedProxy(t *testing.T) {
	cfg := testConfig("127.0.0.1")
	cfg.TrustedProxies = []string{"not-an-ip"}
	if err := cfg.Validate(); err == nil {
		t.Fatal("Validate() accepted an invalid trusted proxy")
	}
}

func TestValidateRejectsInvalidRateLimit(t *testing.T) {
	cfg := testConfig("127.0.0.1")
	cfg.RateLimitRequests = 10
	cfg.RateLimitWindow = 500 * time.Millisecond
	if err := cfg.Validate(); err == nil {
		t.Fatal("Validate() accepted a sub-second rate limit window")
	}
}

func TestLoadAppliesRemoteModeDefaults(t *testing.T) {
	setValidSecrets(t)
	t.Setenv("BOAIRD_HOST", "0.0.0.0")
	t.Setenv("BOAIRD_REMOTE_MODE", "true")
	t.Setenv("BOAIRD_TLS_CERT_FILE", "/etc/boaird/cert.pem")
	t.Setenv("BOAIRD_TLS_KEY_FILE", "/etc/boaird/key.pem")
	t.Setenv("BOAIRD_TRUSTED_PROXIES", "10.0.0.0/8, 192.168.1.1")
	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if !cfg.RemoteMode || cfg.TLSCertFile == "" || cfg.TLSKeyFile == "" {
		t.Fatalf("remote mode not parsed: %+v", cfg)
	}
	if cfg.RateLimitRequests != defaultRateLimitRequests || cfg.RateLimitWindow != defaultRateLimitWindow {
		t.Fatalf("remote rate limit defaults = %d/%s", cfg.RateLimitRequests, cfg.RateLimitWindow)
	}
	if len(cfg.TrustedProxies) != 2 {
		t.Fatalf("trusted proxies = %v", cfg.TrustedProxies)
	}
}

func TestLoadRejectsMissingOrMalformedMasterKey(t *testing.T) {
	t.Setenv("BOAIRD_LOCAL_AUTH_TOKEN", testLocalAuthToken())
	for _, value := range []string{"", "not-hex", "abcd"} {
		t.Run(value, func(t *testing.T) {
			t.Setenv("BOAIRD_MASTER_KEY", value)
			if _, err := Load(); err == nil {
				t.Fatal("Load() accepted a missing or malformed master key")
			}
		})
	}
}

func TestLoadDecodesMasterKey(t *testing.T) {
	t.Setenv("BOAIRD_MASTER_KEY", testMasterKey())
	t.Setenv("BOAIRD_LOCAL_AUTH_TOKEN", testLocalAuthToken())
	cfg, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	if cfg.MasterKey[0] != 1 || cfg.MasterKey[31] != 1 {
		t.Fatal("Load() did not decode the master key")
	}
}

func TestLoadRejectsMissingOrMalformedLocalAuthToken(t *testing.T) {
	t.Setenv("BOAIRD_MASTER_KEY", testMasterKey())
	for _, value := range []string{"", "not-hex", "abcd"} {
		t.Run(value, func(t *testing.T) {
			t.Setenv("BOAIRD_LOCAL_AUTH_TOKEN", value)
			if _, err := Load(); err == nil {
				t.Fatal("Load() accepted a missing or malformed local auth token")
			}
		})
	}
}

func TestLoadDecodesLocalAuthToken(t *testing.T) {
	t.Setenv("BOAIRD_MASTER_KEY", testMasterKey())
	t.Setenv("BOAIRD_LOCAL_AUTH_TOKEN", testLocalAuthToken())
	cfg, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	if cfg.LocalAuthToken[0] != 2 || cfg.LocalAuthToken[31] != 2 {
		t.Fatal("Load() did not decode the local auth token")
	}
}

func testMasterKey() string {
	return "0101010101010101010101010101010101010101010101010101010101010101"
}

func testLocalAuthToken() string {
	return "0202020202020202020202020202020202020202020202020202020202020202"
}
