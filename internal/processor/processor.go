package processor

import (
	"context"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"reflect"
	"strings"
	"time"

	"github.com/rs/zerolog"

	"lora-parser/internal/aloxy"
	"lora-parser/internal/lns"
)

type Processor struct {
	Logger             zerolog.Logger
	Aloxy              aloxy.Client
	// Configuration-driven property paths
	// e.g., PropertyMap["STATUS_OPEN"] = "Features.ValvePosition.Properties.Status.LogicalPosition.Open"
	PropertyMap        map[string]string
	// e.g., PropertyMeta["StartTime"] = "Features.ValvePosition.Properties.Status.Timestamp"
	//       PropertyMeta["Status"]    = "Features.ValvePosition.Properties.Status.Valid"
	PropertyMeta       map[string]string
	// Optional per-sensor overrides
	// e.g., PropertyTimestamp["RBTNP_TIME"] = "Features.DeviceEvents.Properties.status.rightButtonPressedTimestamp"
	PropertyTimestamp  map[string]string
	// e.g., PropertyStatus["RBTNP_STATUS"] = "true" or a path like "Features...valid"
	PropertyStatus     map[string]string
}

type FlatOut struct {
	Value     any    `json:"value"`
	Status    bool   `json:"status"`
	Sensor    string `json:"sensor"`
	StartTime string `json:"starttime"`
}

type WriteTag struct {
	TagID       string `json:"tagid"`
	SosID       string `json:"sosid"`
	HistorianID string `json:"historianid"`
	Value       any    `json:"value"`
}

type WritePayload struct {
	Status string     `json:"status"`
	Tags   []WriteTag `json:"tags"`
}

func (p Processor) HandleMessage(ctx context.Context, payload []byte) ([]FlatOut, string, string, error) {
	var msg lns.Message
	if err := json.Unmarshal(payload, &msg); err != nil {
		return nil, "", "", fmt.Errorf("invalid lns json: %w", err)
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
		return nil, "", "", fmt.Errorf("missing fields in lns json")
	}
	// base64 -> hex
	raw, err := base64.StdEncoding.DecodeString(msg.Data)
	if err != nil {
		return nil, "", "", fmt.Errorf("invalid base64 data: %w", err)
	}
	hexPayload := strings.ToLower(hex.EncodeToString(raw))

	resp, err := p.Aloxy.Do(ctx, deviceName, hexPayload)
	if err != nil {
		return nil, "", "", err
	}
	// Resolve meta defaults from configuration
	// Default fallbacks maintain prior behavior if paths are missing
	globalStatus := true
	if path, ok := p.PropertyMeta["Status"]; ok {
		if v, ok := getByPath(resp, path); ok {
			switch b := v.(type) {
			case bool:
				globalStatus = b
			case *bool:
				if b != nil {
					globalStatus = *b
				}
			case int:
				globalStatus = b != 0
			case float64:
				globalStatus = b != 0
			}
		} else {
			p.Logger.Warn().Str("path", path).Msg("Failed to resolve status path")
		}
	}
	globalStart := ""
	if path, ok := p.PropertyMeta["StartTime"]; ok {
		if v, ok := getByPath(resp, path); ok {
			if s, ok := v.(string); ok {
				p.Logger.Info().Str("vp_timestamp_raw", s).Msg("aloxy vp timestamp")
				globalStart = formatToGMT(strings.TrimSpace(s))
			}
		} else {
			p.Logger.Warn().Str("path", path).Msg("Failed to resolve start time path")
		}
	}

	// Use deviceName directly as the base sensor name
	name := deviceName

	// Build outputs dynamically from PropertyMap
	outs := make([]FlatOut, 0, len(p.PropertyMap))
	for suffix, path := range p.PropertyMap {
		if v, ok := getByPath(resp, path); ok {
			// Per-sensor overrides for status and start time
			st := globalStart
			if tsPath, ok := p.PropertyTimestamp[suffix+"_TIME"]; ok {
				if tv, ok := getByPath(resp, tsPath); ok {
					if s, ok := tv.(string); ok {
						st = formatToGMT(strings.TrimSpace(s))
					}
				} else {
					p.Logger.Warn().Str("path", tsPath).Str("sensor", suffix).Msg("Failed to resolve per-sensor timestamp path")
				}
			}
			sv := globalStatus
			if sConf, ok := p.PropertyStatus[suffix+"_STATUS"]; ok {
				// Allow direct literal booleans "true"/"false"
				low := strings.ToLower(strings.TrimSpace(strings.Trim(sConf, "\"'")))
				if low == "true" || low == "false" {
					sv = (low == "true")
				} else {
					if pv, ok := getByPath(resp, sConf); ok {
						switch b := pv.(type) {
						case bool:
							sv = b
						case *bool:
							if b != nil {
								sv = *b
							}
						case int:
							sv = b != 0
						case float64:
							sv = b != 0
						}
					} else {
						p.Logger.Warn().Str("path", sConf).Str("sensor", suffix).Msg("Failed to resolve per-sensor status path")
					}
				}
			}
			outs = append(outs, FlatOut{
				Value:     v,
				Status:    sv,
				Sensor:    name + "-" + suffix,
				StartTime: st,
			})
		} else {
			p.Logger.Warn().Str("path", path).Msg("Failed to resolve property path")
		}
	}
	return outs, name, devEUI, nil
}

func getByPath(data any, path string) (any, bool) {
	v := reflect.ValueOf(data)
	for _, seg := range strings.Split(path, ".") {
		if v.Kind() == reflect.Ptr {
			if v.IsNil() {
				return nil, false
			}
			v = v.Elem()
		}
		if v.Kind() == reflect.Interface {
			if v.IsNil() {
				return nil, false
			}
			v = v.Elem()
		}
		if v.Kind() != reflect.Struct {
			return nil, false
		}
		t := v.Type()
		// Try direct field name (case sensitive)
		f := v.FieldByName(seg)
		if !f.IsValid() {
			// Try case-insensitive match and JSON tag match
			matched := false
			lowerSeg := strings.ToLower(seg)
			for i := 0; i < t.NumField(); i++ {
				sf := t.Field(i)
				// Exported fields only
				if sf.PkgPath != "" { // unexported
					continue
				}
				// Compare by case-insensitive field name
				if strings.ToLower(sf.Name) == lowerSeg {
					f = v.Field(i)
					matched = true
					break
				}
				// Compare by json tag
				if tag := sf.Tag.Get("json"); tag != "" {
					tagName := strings.Split(tag, ",")[0]
					if tagName == seg || strings.ToLower(tagName) == lowerSeg {
						f = v.Field(i)
						matched = true
						break
					}
				}
			}
			if !matched {
				return nil, false
			}
		}
		v = f
	}
	if v.Kind() == reflect.Ptr {
		if v.IsNil() {
			return nil, false
		}
		v = v.Elem()
	}
	if v.Kind() == reflect.Interface {
		if v.IsNil() {
			return nil, false
		}
		v = v.Elem()
	}
	return v.Interface(), true
}

func formatToGMT(ts string) string {
	if ts == "" {
		return ""
	}
	layouts := []string{time.RFC3339Nano, time.RFC3339, "2006-01-02 15:04:05", "2006-01-02T15:04:05"}
	var t time.Time
	var err error
	for _, l := range layouts {
		t, err = time.Parse(l, ts)
		if err == nil {
			break
		}
	}
	if err != nil {
		return ts // fallback
	}
	return t.UTC().Format("02-Jan-2006 15:04:05 GMT")
}
