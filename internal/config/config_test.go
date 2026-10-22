package config

import (
	"os"
	"testing"
)

func TestLoadUsesLocalDefaults(t *testing.T) {
	unsetEnvironment(t, "BOAIRD_HOST", "BOAIRD_PORT", "BOAIRD_DATABASE_PATH")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if cfg.Host != defaultHost || cfg.Port != defaultPort || cfg.DatabasePath != defaultDatabasePath {
		t.Fatalf("Load() = %+v, want local defaults", cfg)
	}
}

func TestLoadUsesConfiguredValues(t *testing.T) {
	t.Setenv("BOAIRD_HOST", defaultHost)
	t.Setenv("BOAIRD_PORT", "3110")
	t.Setenv("BOAIRD_DATABASE_PATH", defaultDatabasePath)
	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if cfg.Address() != "127.0.0.1:3110" {
		t.Fatalf("Address() = %q, want %q", cfg.Address(), "127.0.0.1:3110")
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
	t.Setenv("BOAIRD_PORT", "not-a-port")
	if _, err := Load(); err == nil {
		t.Fatal("Load() accepted an invalid port")
	}
}

func TestValidateRejectsOutOfRangePort(t *testing.T) {
	cfg := Config{Host: "127.0.0.1", Port: 65536, DatabasePath: "./data/boaird.db"}
	if err := cfg.Validate(); err == nil {
		t.Fatal("Validate() accepted an out-of-range port")
	}
}

func TestValidateRejectsNonLoopbackHosts(t *testing.T) {
	for _, host := range []string{"0.0.0.0", "192.0.2.1", "localhost"} {
		t.Run(host, func(t *testing.T) {
			cfg := Config{Host: host, Port: defaultPort, DatabasePath: defaultDatabasePath}
			if err := cfg.Validate(); err == nil {
				t.Fatalf("Validate() accepted non-loopback host %q", host)
			}
		})
	}
}

func TestValidateAcceptsLoopbackHosts(t *testing.T) {
	for _, host := range []string{"127.0.0.1", "::1"} {
		t.Run(host, func(t *testing.T) {
			cfg := Config{Host: host, Port: defaultPort, DatabasePath: defaultDatabasePath}
			if err := cfg.Validate(); err != nil {
				t.Fatalf("Validate() rejected loopback host %q: %v", host, err)
			}
		})
	}
}
