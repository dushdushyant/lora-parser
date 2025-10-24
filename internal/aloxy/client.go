package aloxy

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/rs/zerolog"
)

type Client struct {
	BaseURL       string
	Method        string
	PayloadField  string
	Timeout       time.Duration
	AuthType      string
	AuthToken     string
	AuthHeaderKey string
	Logger        zerolog.Logger
	// Mocking
	MockEnabled   bool
	MockFile      string
	MockJSON      string
}

type Response struct {
	Features struct {
		DeviceEvents struct {
			Properties struct {
				Status struct {
					RightButtonPressed any    `json:"rightButtonPressed"`
					Timestamp         string `json:"timestamp"`
				} `json:"status"`
			} `json:"properties"`
		} `json:"DeviceEvents"`
		ValvePosition struct {
			Properties struct {
				Status struct {
					LogicalPosition struct {
						Open           any `json:"open"`
						Closed         any `json:"closed"`
						OpenPercentage any `json:"openPercentage"`
					} `json:"logicalPosition"`
				} `json:"status"`
				Valid    bool   `json:"valid"`
				Timestamp string `json:"timestamp"`
			} `json:"properties"`
		} `json:"ValvePosition"`
	} `json:"features"`
}

type RequestBody map[string]any

func (c Client) Do(ctx context.Context, deviceName string, payloadHex string) (Response, error) {
	var out Response
	// Mock override
	if c.MockEnabled {
		var data []byte
		if strings.TrimSpace(c.MockJSON) != "" {
			data = []byte(c.MockJSON)
		} else if strings.TrimSpace(c.MockFile) != "" {
			b, err := os.ReadFile(c.MockFile)
			if err != nil { return out, fmt.Errorf("read mock file: %w", err) }
			data = b
		} else {
			return out, fmt.Errorf("mock_enabled is true but no mock_json or mock_file provided")
		}
		if err := json.Unmarshal(data, &out); err != nil {
			return out, fmt.Errorf("invalid mock json: %w", err)
		}
		return out, nil
	}
	method := strings.ToUpper(strings.TrimSpace(c.Method))
	if method == "" { method = http.MethodPost }

	url := strings.TrimRight(c.BaseURL, "/") + "/" + deviceName
	body := RequestBody{}
	key := c.PayloadField
	if key == "" { key = "payload" }
	body[key] = payloadHex
	b, _ := json.Marshal(body)

	req, err := http.NewRequestWithContext(ctx, method, url, bytes.NewReader(b))
	if err != nil { return out, err }
	req.Header.Set("Content-Type", "application/json")
	// Auth
	switch strings.ToLower(c.AuthType) {
	case "bearer":
		if c.AuthToken != "" { req.Header.Set("Authorization", "Bearer "+c.AuthToken) }
	case "header":
		h := c.AuthHeaderKey
		if h == "" { h = "X-API-Key" }
		if c.AuthToken != "" { req.Header.Set(h, c.AuthToken) }
	}

	client := &http.Client{ Timeout: c.Timeout }
	resp, err := client.Do(req)
	if err != nil { return out, err }
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		bb, _ := io.ReadAll(resp.Body)
		return out, fmt.Errorf("aloxy http %d: %s", resp.StatusCode, string(bb))
	}
	dec := json.NewDecoder(resp.Body)
	if err := dec.Decode(&out); err != nil { return out, err }
	return out, nil
}
