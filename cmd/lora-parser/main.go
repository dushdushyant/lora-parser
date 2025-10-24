package main

import (
	"context"
	"encoding/json"
	"os"
	"os/signal"
	"syscall"
	"time"

	"lora-parser/internal/aloxy"
	"lora-parser/internal/config"
	"lora-parser/internal/logging"
	mqttcli "lora-parser/internal/mqtt"
	"lora-parser/internal/processor"
)

func main() {
	cfg, err := config.Load("configs/config.ini")
	if err != nil {
		panic(err)
	}
	logger, closeLogger, err := logging.NewLogger(cfg.Logging.File, cfg.Logging.Level, cfg.Logging.MaxSizeMB, cfg.Logging.MaxBackups, cfg.Logging.MaxAgeDays)
	if err != nil {
		panic(err)
	}
	defer closeLogger()

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	sensorMap, tagMap, err := config.LoadSensors(cfg.Sensors.MapFile)
	if err != nil {
		logger.Fatal().Err(err).Msg("failed to load sensor map")
	}

	inClient, err := mqttcli.NewClient(mqttcli.ClientOptions{
		Broker:   cfg.MQTTInput.Broker,
		ClientID: cfg.MQTTInput.ClientID,
		Username: cfg.MQTTInput.Username,
		Password: cfg.MQTTInput.Password,
		Clean:    true,
		KeepAlive: 30,
	})
	if err != nil {
		logger.Fatal().Err(err).Msg("failed to create input mqtt client")
	}
	outClient, err := mqttcli.NewClient(mqttcli.ClientOptions{
		Broker:   cfg.MQTTOutput.Broker,
		ClientID: cfg.MQTTOutput.ClientID,
		Username: cfg.MQTTOutput.Username,
		Password: cfg.MQTTOutput.Password,
		Clean:    true,
		KeepAlive: 30,
	})
	if err != nil {
		logger.Fatal().Err(err).Msg("failed to create output mqtt client")
	}

	if err := inClient.Connect(ctx); err != nil {
		logger.Fatal().Err(err).Msg("failed to connect input mqtt")
	}
	defer inClient.Disconnect()
	if err := outClient.Connect(ctx); err != nil {
		logger.Fatal().Err(err).Msg("failed to connect output mqtt")
	}
	defer outClient.Disconnect()

	al := aloxy.Client{
		BaseURL:       cfg.Aloxy.BaseURL,
		Method:        cfg.Aloxy.Method,
		PayloadField:  cfg.Aloxy.PayloadField,
		Timeout:       time.Duration(cfg.Aloxy.TimeoutSeconds) * time.Second,
		AuthType:      cfg.Aloxy.AuthType,
		AuthToken:     cfg.Aloxy.AuthToken,
		AuthHeaderKey: cfg.Aloxy.AuthHeaderKey,
		Logger:        logger,
		MockEnabled:   cfg.Aloxy.MockEnabled,
		MockFile:      cfg.Aloxy.MockFile,
		MockJSON:      cfg.Aloxy.MockJSON,
	}

	proc := processor.Processor{
		Logger:            logger,
		Aloxy:             al,
		SensorNameByDevEUI: sensorMap,
	}

	h := func(topic string, payload []byte) {
		outs, _, devEUI, err := proc.HandleMessage(ctx, payload)
		if err != nil {
			logger.Error().Err(err).Msg("processing failed")
			return
		}
		for _, o := range outs {
			b, _ := json.Marshal(o)
			if err := outClient.Publish(ctx, cfg.MQTTOutput.LoraOutputTopic, byte(cfg.MQTTOutput.QoS), cfg.MQTTOutput.Retain, b); err != nil {
				logger.Error().Err(err).Msg("publish failed")
			} else {
				logger.Info().Str("topic", cfg.MQTTOutput.LoraOutputTopic).RawJSON("payload", b).Msg("published")
			}
		}

		// Aggregate second output (Option B): single message with array of tags
		if cfg.LoraWriteOutput.LoraWriteTopic != "" {
			wp := processor.WritePayload{Status: "W"}
			for _, o := range outs {
				sosid := o.Sensor // exactly same as first output's sensor field
				tagid := tagMap[sosid]
				if tagid == "" {
					logger.Warn().Str("sosid", sosid).Msg("missing tagid mapping; skipping tag")
					continue
				}
				wp.Tags = append(wp.Tags, processor.WriteTag{
					TagID:       tagid,
					SosID:       sosid,
					HistorianID: devEUI,
					Value:       o.Value,
				})
			}
			if len(wp.Tags) > 0 {
				wb, _ := json.Marshal(wp)
				if err := outClient.Publish(ctx, cfg.LoraWriteOutput.LoraWriteTopic, byte(cfg.LoraWriteOutput.QoS), cfg.LoraWriteOutput.Retain, wb); err != nil {
					logger.Error().Err(err).Msg("publish lora_write_output failed")
				} else {
					logger.Info().Str("topic", cfg.LoraWriteOutput.LoraWriteTopic).RawJSON("payload", wb).Msg("published lora_write_output")
				}
			}
		}
	}

	if err := inClient.Subscribe(ctx, cfg.MQTTInput.LoraInputTopic, byte(cfg.MQTTInput.QoS), h); err != nil {
		logger.Fatal().Err(err).Msg("subscribe failed")
	}
	logger.Info().Str("topic", cfg.MQTTInput.LoraInputTopic).Msg("listening")
	<-ctx.Done()
}
