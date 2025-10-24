package processor

import (
	"context"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/rs/zerolog"

	"lora-parser/internal/aloxy"
	"lora-parser/internal/lns"
)

type Processor struct {
	Logger             zerolog.Logger
	Aloxy              aloxy.Client
	SensorNameByDevEUI map[string]string
}

type FlatOut struct {
	Value     any    `json:"value"`
	Status    bool   `json:"status"`
	Sensor    string `json:"sensor"`
	StartTime string `json:"starttime"`
}

func (p Processor) HandleMessage(ctx context.Context, payload []byte) ([]FlatOut, error) {
	var msg lns.Message
	if err := json.Unmarshal(payload, &msg); err != nil {
		return nil, fmt.Errorf("invalid lns json: %w", err)
	}
	// Prefer deviceInfo.* if provided, else fallback to top-level
	deviceName := strings.TrimSpace(msg.DeviceInfo.DeviceName)
	if deviceName == "" {
		deviceName = strings.TrimSpace(msg.DeviceName)
	}
	devEUI := strings.TrimSpace(msg.DeviceInfo.DevEUI)
	if devEUI == "" {
		devEUI = strings.TrimSpace(msg.DevEUI)
	}
	if msg.Data == "" || deviceName == "" || devEUI == "" {
		return nil, fmt.Errorf("missing fields in lns json")
	}
	// base64 -> hex
	raw, err := base64.StdEncoding.DecodeString(msg.Data)
	if err != nil {
		return nil, fmt.Errorf("invalid base64 data: %w", err)
	}
	hexPayload := strings.ToLower(hex.EncodeToString(raw))

	resp, err := p.Aloxy.Do(ctx, deviceName, hexPayload)
	if err != nil {
		return nil, err
	}
	valid := resp.Features.ValvePosition.Properties.Valid
	vpTs := resp.Features.ValvePosition.Properties.Timestamp
	start := formatToGMT(vpTs)

	name := p.SensorNameByDevEUI[devEUI]
	if name == "" {
		name = devEUI
	}

	outs := make([]FlatOut, 0, 4)
	outs = append(outs, FlatOut{
		Value:     resp.Features.ValvePosition.Properties.Status.LogicalPosition.Open,
		Status:    valid,
		Sensor:    name + "-STATUS_OPEN",
		StartTime: start,
	})
	outs = append(outs, FlatOut{
		Value:     resp.Features.ValvePosition.Properties.Status.LogicalPosition.Closed,
		Status:    valid,
		Sensor:    name + "-STATUS_CLOSE",
		StartTime: start,
	})
	outs = append(outs, FlatOut{
		Value:     resp.Features.ValvePosition.Properties.Status.LogicalPosition.OpenPercentage,
		Status:    valid,
		Sensor:    name + "-PERCENTAGE_OPEN",
		StartTime: start,
	})
	outs = append(outs, FlatOut{
		Value:     resp.Features.DeviceEvents.Properties.Status.RightButtonPressed,
		Status:    valid,
		Sensor:    name + "-RBTNP",
		StartTime: start,
	})
	return outs, nil
}

func formatToGMT(ts string) string {
	if ts == "" { return "" }
	layouts := []string{time.RFC3339Nano, time.RFC3339, "2006-01-02 15:04:05", "2006-01-02T15:04:05"}
	var t time.Time
	var err error
	for _, l := range layouts {
		t, err = time.Parse(l, ts)
		if err == nil { break }
	}
	if err != nil {
		return ts // fallback
	}
	return t.UTC().Format("02-Jan-2006 15:04:05 GMT")
}
