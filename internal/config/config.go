package config

import (
	"encoding/json"
	"errors"
	"os"

	ini "gopkg.in/ini.v1"
)

type MQTTConfig struct {
	Broker         string `ini:"broker"`
	Username       string `ini:"username"`
	Password       string `ini:"password"`
	QoS            int    `ini:"qos"`
	ClientID       string `ini:"client_id"`
	LoraInputTopic string `ini:"lora_input_topic"`
	LoraOutputTopic string `ini:"lora_output_topic"`
	Retain         bool   `ini:"retain"`
}

type AloxyConfig struct {
	BaseURL       string `ini:"base_url"`
	Method        string `ini:"method"`
	PayloadField  string `ini:"payload_field"`
	TimeoutSeconds int   `ini:"timeout_seconds"`
	AuthType      string `ini:"auth_type"`
	AuthToken     string `ini:"auth_token"`
	AuthHeaderKey string `ini:"auth_header_key"`
	MockEnabled   bool   `ini:"mock_enabled"`
	MockFile      string `ini:"mock_file"`
	MockJSON      string `ini:"mock_json"`
}

type LoggingConfig struct {
	File        string `ini:"file"`
	Level       string `ini:"level"`
	MaxSizeMB   int    `ini:"max_size_mb"`
	MaxBackups  int    `ini:"max_backups"`
	MaxAgeDays  int    `ini:"max_age_days"`
}

type SensorsConfig struct {
	MapFile string `ini:"map_file"`
}

type Config struct {
	MQTTInput  MQTTConfig
	MQTTOutput MQTTConfig
	Aloxy      AloxyConfig
	Logging    LoggingConfig
	Sensors    SensorsConfig
}

func Load(path string) (Config, error) {
	cfg := Config{}
	f, err := ini.Load(path)
	if err != nil {
		return cfg, err
	}
	if err := f.Section("mqtt_input").MapTo(&cfg.MQTTInput); err != nil {
		return cfg, err
	}
	if err := f.Section("mqtt_output").MapTo(&cfg.MQTTOutput); err != nil {
		return cfg, err
	}
	if err := f.Section("aloxy").MapTo(&cfg.Aloxy); err != nil {
		return cfg, err
	}
	if err := f.Section("logging").MapTo(&cfg.Logging); err != nil {
		return cfg, err
	}
	if err := f.Section("sensors").MapTo(&cfg.Sensors); err != nil {
		return cfg, err
	}
	if cfg.MQTTInput.Broker == "" || cfg.MQTTOutput.Broker == "" {
		return cfg, errors.New("mqtt brokers must be set")
	}
	return cfg, nil
}

func LoadSensorMap(path string) (map[string]string, error) {
	m := map[string]string{}
	b, err := os.ReadFile(path)
	if err != nil {
		return m, err
	}
	if err := json.Unmarshal(b, &m); err != nil {
		return m, err
	}
	return m, nil
}
