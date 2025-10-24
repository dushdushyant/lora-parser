# LoRa LNS -> Aloxy Parser (Go)

A Go application that:
- Subscribes to an MQTT input topic for ChirpStack (LoRa LNS) JSON uplinks.
- Extracts `data` (base64), converts to hex.
- Calls Aloxy Core endpoint appended with `deviceName` and posts the hex payload.
- Parses Aloxy response to derive four flat JSON outputs.
- Publishes all four outputs to a single MQTT output topic.
- Logs to file with rotation.

## Build

- Go 1.21+

```
go mod tidy
go build -o bin/lora-parser ./cmd/lora-parser
```

## Configure

- Core config (INI): `configs/config.ini`
- Sensor mapping (JSON): `configs/sensors.json`

### INI (`configs/config.ini`)

Key points:
- `mqtt_input` and `mqtt_output` support different brokers.
- Brokers can be specified as `host:port` (scheme auto-adds tcp://) or `tcp://host:port`.
- Set QoS (0/1/2), client IDs, topics (`lora_input_topic`, `lora_output_topic`).
- Aloxy:
  - `base_url` like `https://aloxy.example.com/api`
  - Device name is appended automatically.
  - `payload_field` defaults to `payload` if blank.
  - `auth_type`: `none`, `bearer`, or `header`. When `header`, provide `auth_header_key` and `auth_token`.
- Logging path will be created if missing. Rotation defaults are set in the file.
- `sensors.map_file` points to sensor name mapping JSON.

### Sensor map (`configs/sensors.json`)

Map devEUI (hex string) to sensor base name (site/los/building style). Example:

```
{
  "0102030405060708": "SITE1-LOS1-BLDG1",
  "0011223344556677": "SITE2-LOS1-BLDG3"
}
```

## Run

```
./bin/lora-parser
```

The app listens on the input topic and publishes outputs to the configured output topic.

## Message Formats

### Input (ChirpStack uplink, example)
```
{
  "applicationID": "...",
  "applicationName": "...",
  "deviceName": "device1",
  "devEUI": "0102030405060708",
  "rxInfo": [ ... ],
  "txInfo": { ... },
  "fPort": 1,
  "data": "AQIDBAU=",
  "object": { }
}
```

### Output (4 messages published)
Each message has the shape:
```
{
  "value": <bool|number|string>,
  "status": <bool>,
  "sensor": "<SensorName>-<SUFFIX>",
  "starttime": "DD-Mon-YYYY HH:MM:SS GMT"
}
```
SUFFIX mapping:
- STATUS_OPEN
- STATUS_CLOSE
- PERCENTAGE_OPEN
- RBTNP

SensorName is derived from `devEUI` via `configs/sensors.json`. If missing, `devEUI` itself is used.

## Notes

- MQTT over TCP (port 1883). Username/password supported. TLS is not implemented in this version.
- Graceful shutdown on SIGTERM/CTRL+C.
- Logs go to `logs/app.log` by default.
