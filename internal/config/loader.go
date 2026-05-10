package config

import (
	"bytes"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/spf13/viper"
)

// Load initializes Viper, reads the configuration from the specified path,
// environment variables, or standard input (if provided as JSON fallback),
// and returns the populated BotConfig.
//
// The resolution order is:
//  1. Config file at `path` (if provided)
//  2. Config file path from BOT_CONFIG_FILE env var
//  3. Raw JSON from BOT_CONFIG_JSON env var
//  4. Environment variables directly (e.g. BOT_UUID -> BotUUID)
//  5. Standard input (if not a terminal)
func Load(path string) (*BotConfig, error) {
	v := viper.New()

	// 1. Setup environment variables mapping.
	// We prefix all environment variables with "BOT_" for safety.
	v.SetEnvPrefix("bot")
	v.SetEnvKeyReplacer(strings.NewReplacer(".", "_"))
	v.AutomaticEnv()

	// 2. Determine configuration source.
	if path != "" {
		v.SetConfigFile(path)
		if err := v.ReadInConfig(); err != nil {
			return nil, fmt.Errorf("config: read %s: %w", path, err)
		}
	} else if p := os.Getenv("BOT_CONFIG_FILE"); p != "" {
		v.SetConfigFile(p)
		if err := v.ReadInConfig(); err != nil {
			return nil, fmt.Errorf("config: read from env BOT_CONFIG_FILE %s: %w", p, err)
		}
	} else if raw := os.Getenv("BOT_CONFIG_JSON"); raw != "" {
		v.SetConfigType("json")
		if err := v.ReadConfig(bytes.NewBufferString(raw)); err != nil {
			return nil, fmt.Errorf("config: parse BOT_CONFIG_JSON: %w", err)
		}
	} else {
		// Fallback to stdin if data is piped.
		stat, _ := os.Stdin.Stat()
		if (stat.Mode() & os.ModeCharDevice) == 0 {
			v.SetConfigType("json")
			if err := v.ReadConfig(os.Stdin); err != nil {
				if err == io.EOF {
					return nil, fmt.Errorf("config: stdin: empty input")
				}
				return nil, fmt.Errorf("config: stdin: %w", err)
			}
		} else {
			// No config file, no env JSON, no stdin. Rely purely on ENV vars.
			// (Viper will still unmarshal AutomaticEnv values).
		}
	}

	// 3. Unmarshal into our struct using mapstructure.
	var cfg BotConfig
	if err := v.Unmarshal(&cfg); err != nil {
		return nil, fmt.Errorf("config: unmarshal: %w", err)
	}

	return &cfg, nil
}

// LoadFromFile reads path and unmarshals it into *BotConfig.
func LoadFromFile(path string) (*BotConfig, error) {
	return Load(path)
}

// LoadFromBytes is convenient for tests and embeds.
func LoadFromBytes(b []byte) (*BotConfig, error) {
	v := viper.New()
	v.SetConfigType("json")
	if err := v.ReadConfig(bytes.NewReader(b)); err != nil {
		return nil, fmt.Errorf("config: parse bytes: %w", err)
	}
	var cfg BotConfig
	if err := v.Unmarshal(&cfg); err != nil {
		return nil, fmt.Errorf("config: unmarshal: %w", err)
	}
	return &cfg, nil
}
