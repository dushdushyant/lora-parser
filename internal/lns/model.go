package lns

type RxInfo struct {
	GatewayID string  `json:"gatewayID"`
	RSSI      int     `json:"rssi"`
	LoraSnr   float64 `json:"loraSnr"`
}

type TxInfo struct {
	Frequency uint64 `json:"frequency"`
	DR        int    `json:"dr"`
}

type DeviceInfo struct {
	DevEUI     string `json:"devEUI"`
	DeviceName string `json:"deviceName"`
}

type Message struct {
	ApplicationID   string     `json:"applicationID"`
	ApplicationName string     `json:"applicationName"`
	DeviceName      string     `json:"deviceName"`
	DevEUI          string     `json:"devEUI"`
	DeviceInfo      DeviceInfo `json:"deviceInfo"`
	RxInfo          []RxInfo   `json:"rxInfo"`
	TxInfo          TxInfo     `json:"txInfo"`
	FPort           int        `json:"fPort"`
	Data            string     `json:"data"`
	Object          any        `json:"object"`
}
