package mqtt

import (
	"context"
	"errors"
	"strings"
	"time"

	mqtt "github.com/eclipse/paho.mqtt.golang"
)

type ClientOptions struct {
	Broker    string
	ClientID  string
	Username  string
	Password  string
	Clean     bool
	KeepAlive int
}

type Client struct {
	c mqtt.Client
	opts *mqtt.ClientOptions
}

func NewClient(o ClientOptions) (*Client, error) {
	if o.Broker == "" {
		return nil, errors.New("broker required")
	}
	broker := o.Broker
	if !strings.Contains(broker, "://") {
		broker = "tcp://" + broker
	}
	opts := mqtt.NewClientOptions().AddBroker(broker)
	opts.SetClientID(o.ClientID)
	if o.Username != "" {
		opts.SetUsername(o.Username)
	}
	if o.Password != "" {
		opts.SetPassword(o.Password)
	}
	opts.SetCleanSession(o.Clean)
	if o.KeepAlive <= 0 { o.KeepAlive = 30 }
	opts.SetKeepAlive(time.Duration(o.KeepAlive) * time.Second)
	return &Client{c: mqtt.NewClient(opts), opts: opts}, nil
}

func (c *Client) Connect(ctx context.Context) error {
	t := c.c.Connect()
	for {
		if t.WaitTimeout(100 * time.Millisecond) {
			return t.Error()
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}
	}
}

func (c *Client) Disconnect() {
	if c.c != nil && c.c.IsConnectionOpen() {
		c.c.Disconnect(250)
	}
}

type Handler func(topic string, payload []byte)

func (c *Client) Subscribe(ctx context.Context, topic string, qos byte, handler Handler) error {
	t := c.c.Subscribe(topic, qos, func(_ mqtt.Client, m mqtt.Message) {
		handler(m.Topic(), m.Payload())
	})
	for {
		if t.WaitTimeout(100 * time.Millisecond) {
			return t.Error()
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}
	}
}

func (c *Client) Publish(ctx context.Context, topic string, qos byte, retain bool, payload []byte) error {
	t := c.c.Publish(topic, qos, retain, payload)
	for {
		if t.WaitTimeout(100 * time.Millisecond) {
			return t.Error()
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}
	}
}
