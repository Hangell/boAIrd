package config

import (
	"os"
	"testing"
)

func TestLoadUsesLocalDefaults(t *testing.T) {
	unsetEnvironment(t, "BOAIRD_HOST", "BOAIRD_PORT", "BOAIRD_DATABASE_PATH")
	t.Setenv("BOAIRD_MASTER_KEY", testMasterKey())
	t.Setenv("BOAIRD_LOCAL_AUTH_TOKEN", testLocalAuthToken())

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
	t.Setenv("BOAIRD_MASTER_KEY", testMasterKey())
	t.Setenv("BOAIRD_LOCAL_AUTH_TOKEN", testLocalAuthToken())
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
	t.Setenv("BOAIRD_MASTER_KEY", testMasterKey())
	t.Setenv("BOAIRD_LOCAL_AUTH_TOKEN", testLocalAuthToken())
	t.Setenv("BOAIRD_PORT", "not-a-port")
	if _, err := Load(); err == nil {
		t.Fatal("Load() accepted an invalid port")
	}
}

func TestValidateRejectsOutOfRangePort(t *testing.T) {
	cfg := Config{Host: "127.0.0.1", Port: 65536, DatabasePath: "./data/boaird.db", MasterKey: MasterKey{1}, LocalAuthToken: LocalAuthToken{2}}
	if err := cfg.Validate(); err == nil {
		t.Fatal("Validate() accepted an out-of-range port")
	}
}

func TestValidateRejectsReusedMasterKeyForLocalAuthToken(t *testing.T) {
	cfg := Config{
		Host:           defaultHost,
		Port:           defaultPort,
		DatabasePath:   defaultDatabasePath,
		MasterKey:      MasterKey{1},
		LocalAuthToken: LocalAuthToken{1},
	}
	if err := cfg.Validate(); err == nil {
		t.Fatal("Validate() accepted the master key as the local auth token")
	}
}

func TestValidateRejectsNonLoopbackHosts(t *testing.T) {
	for _, host := range []string{"0.0.0.0", "192.0.2.1", "localhost"} {
		t.Run(host, func(t *testing.T) {
			cfg := Config{Host: host, Port: defaultPort, DatabasePath: defaultDatabasePath, MasterKey: MasterKey{1}, LocalAuthToken: LocalAuthToken{2}}
			if err := cfg.Validate(); err == nil {
				t.Fatalf("Validate() accepted non-loopback host %q", host)
			}
		})
	}
}

func TestValidateAcceptsLoopbackHosts(t *testing.T) {
	for _, host := range []string{"127.0.0.1", "::1"} {
		t.Run(host, func(t *testing.T) {
			cfg := Config{Host: host, Port: defaultPort, DatabasePath: defaultDatabasePath, MasterKey: MasterKey{1}, LocalAuthToken: LocalAuthToken{2}}
			if err := cfg.Validate(); err != nil {
				t.Fatalf("Validate() rejected loopback host %q: %v", host, err)
			}
		})
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
