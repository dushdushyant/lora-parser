package mqtt

import (
	"context"
	"errors"
	"sync"
	"strings"
	"time"

	mqtt "github.com/eclipse/paho.mqtt.golang"
	"github.com/rs/zerolog/log"
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
	mu   sync.RWMutex
	subs []subscription
}

type subscription struct {
	topic   string
	qos     byte
	handler Handler
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

	cl := &Client{opts: opts}

	// Enable auto-reconnect and resubscribe on reconnect
	opts.SetAutoReconnect(true)
	opts.SetOnConnectHandler(func(mc mqtt.Client) {
		log.Info().Str("broker", broker).Str("client_id", o.ClientID).Msg("mqtt connected")
		cl.resubscribeAll(mc)
	})
	opts.SetConnectionLostHandler(func(mc mqtt.Client, err error) {
		log.Warn().Err(err).Str("broker", broker).Str("client_id", o.ClientID).Msg("mqtt connection lost; will auto-reconnect")
		// Let paho handle reconnect with its internal backoff
	})

	cl.c = mqtt.NewClient(opts)
	return cl, nil
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
	// Register subscription for resubscribe on reconnect
	c.mu.Lock()
	c.subs = append(c.subs, subscription{topic: topic, qos: qos, handler: handler})
	c.mu.Unlock()
	log.Info().Str("topic", topic).Uint8("qos", qos).Msg("mqtt subscribe request")

	t := c.c.Subscribe(topic, qos, func(_ mqtt.Client, m mqtt.Message) {
		handler(m.Topic(), m.Payload())
	})
	for {
		if t.WaitTimeout(100 * time.Millisecond) {
			if err := t.Error(); err != nil {
				log.Error().Err(err).Str("topic", topic).Uint8("qos", qos).Msg("mqtt subscribe failed")
				return err
			}
			log.Info().Str("topic", topic).Uint8("qos", qos).Msg("mqtt subscribe acknowledged")
			return nil
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

func (c *Client) resubscribeAll(mc mqtt.Client) {
	c.mu.RLock()
	subs := make([]subscription, len(c.subs))
	copy(subs, c.subs)
	c.mu.RUnlock()
	log.Info().Int("count", len(subs)).Msg("mqtt resubscribe start")
	for _, s := range subs {
		token := mc.Subscribe(s.topic, s.qos, func(_ mqtt.Client, m mqtt.Message) {
			s.handler(m.Topic(), m.Payload())
		})
		if !token.WaitTimeout(5 * time.Second) {
			log.Warn().Str("topic", s.topic).Uint8("qos", s.qos).Msg("mqtt resubscribe timeout; will retry on next reconnect")
			continue
		}
		if err := token.Error(); err != nil {
			log.Error().Err(err).Str("topic", s.topic).Uint8("qos", s.qos).Msg("mqtt resubscribe failed")
			continue
		}
		log.Info().Str("topic", s.topic).Uint8("qos", s.qos).Msg("mqtt resubscribed")
	}
}
