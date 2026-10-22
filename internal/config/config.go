package config

import (
	"encoding/hex"
	"fmt"
	"net"
	"os"
	"strconv"
)

const (
	defaultHost         = "127.0.0.1"
	defaultPort         = 3110
	defaultDatabasePath = "./data/boaird.db"
)

// Config contains the settings required to start the local service.
type Config struct {
	Host         string
	Port         int
	DatabasePath string
	MasterKey    MasterKey
}

// MasterKey is the deployment-supplied key used to encrypt provider credentials.
type MasterKey [32]byte

func (MasterKey) String() string   { return "[REDACTED]" }
func (MasterKey) GoString() string { return "[REDACTED]" }

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
	return nil
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
