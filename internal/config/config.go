package config

import (
	"encoding/json"
	"errors"
	"os"
	"strings"

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

type LoraWriteOutputConfig struct {
	LoraWriteTopic string `ini:"lora_write_topic"`
	QoS            int    `ini:"qos"`
	Retain         bool   `ini:"retain"`
}

type Config struct {
	MQTTInput  MQTTConfig
	MQTTOutput MQTTConfig
	LoraWriteOutput LoraWriteOutputConfig
	Aloxy      AloxyConfig
	Logging    LoggingConfig
	Sensors    SensorsConfig
	SensorPropertyMap  map[string]string
	SensorPropertyMeta map[string]string
	SensorPropertyTimestamp map[string]string
	SensorPropertyStatus    map[string]string
}

func Load(path string) (Config, error) {
	cfg := Config{}
	// Use LoadSources with IgnoreInlineComment so '#' in values (e.g., MQTT topics like "application/#")
	// is not treated as an inline comment and truncated by the parser.
	f, err := ini.LoadSources(ini.LoadOptions{IgnoreInlineComment: true}, path)
	if err != nil {
		return cfg, err
	}
	if err := f.Section("mqtt_input").MapTo(&cfg.MQTTInput); err != nil {
		return cfg, err
	}
	if err := f.Section("mqtt_output").MapTo(&cfg.MQTTOutput); err != nil {
		return cfg, err
	}
	// Optional second output topic (uses same broker as MQTTOutput)
	_ = f.Section("lora_write_output").MapTo(&cfg.LoraWriteOutput)
	if err := f.Section("aloxy").MapTo(&cfg.Aloxy); err != nil {
		return cfg, err
	}
	if err := f.Section("logging").MapTo(&cfg.Logging); err != nil {
		return cfg, err
	}
	if err := f.Section("sensors").MapTo(&cfg.Sensors); err != nil {
		return cfg, err
	}
	// Optional dynamic property map and meta sections
	cfg.SensorPropertyMap = f.Section("sensor_property_map").KeysHash()
	cfg.SensorPropertyMeta = f.Section("sensor_property_meta").KeysHash()
	cfg.SensorPropertyTimestamp = f.Section("sensor_property_timestamp").KeysHash()
	cfg.SensorPropertyStatus = f.Section("sensor_property_status").KeysHash()
	// Normalize topics: remove surrounding single/double quotes if present
	cfg.MQTTInput.LoraInputTopic = strings.Trim(cfg.MQTTInput.LoraInputTopic, "\"'")
	cfg.MQTTOutput.LoraOutputTopic = strings.Trim(cfg.MQTTOutput.LoraOutputTopic, "\"'")
	cfg.LoraWriteOutput.LoraWriteTopic = strings.Trim(cfg.LoraWriteOutput.LoraWriteTopic, "\"'")
	if cfg.MQTTInput.Broker == "" || cfg.MQTTOutput.Broker == "" {
		return cfg, errors.New("mqtt brokers must be set")
	}
	return cfg, nil
}

// Sensors file supports two schemas:
// 1) Back-compat flat map: { "<devEUI>": "<SensorName>", ... }
// 2) Extended object: { "sensor_names": {devEUI: name}, "tag_map": {sosid: tagid} }
func LoadSensors(path string) (nameByDevEUI map[string]string, tagIdBySosId map[string]string, err error) {
	nameByDevEUI = map[string]string{}
	tagIdBySosId = map[string]string{}
	b, err := os.ReadFile(path)
	if err != nil { return }
	// Try extended schema first
	var ext struct {
		SensorNames map[string]string `json:"sensor_names"`
		TagMap      map[string]string `json:"tag_map"`
	}
	if e := json.Unmarshal(b, &ext); e == nil && (len(ext.SensorNames) > 0 || len(ext.TagMap) > 0) {
		if ext.SensorNames != nil { nameByDevEUI = ext.SensorNames }
		if ext.TagMap != nil { tagIdBySosId = ext.TagMap }
		return
	}
	// Fallback to flat map
	if e := json.Unmarshal(b, &nameByDevEUI); e != nil {
		err = e
		return
	}
	return
}
