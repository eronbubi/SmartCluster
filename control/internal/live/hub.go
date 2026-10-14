// Package live pushes events to the control app over WebSocket
// (ARCHITECTURE.md 8.3). Frames use the same envelope as the agent protocol.
//
// Milestone a has no login yet; the API listener therefore binds to
// localhost by default. Authentication arrives with milestone b.
package live

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"regexp"
	"sync"
	"time"

	"github.com/coder/websocket"

	"github.com/eronbubi/smartcluster/pkg/proto"
)

// Topics a client can subscribe to.
const (
	TopicNodes = "nodes" // node.status of all nodes
	TopicStats = "stats" // node.stats of all nodes
)

// TopicNodeStats is the topic for the stats of one node.
func TopicNodeStats(nodeID string) string { return "node:" + nodeID + ":stats" }

// Event types.
const (
	TypeNodeStatus = "node.status"
	TypeNodeStats  = "node.stats"
)

var nodeStatsTopic = regexp.MustCompile(`^node:[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}:stats$`)

const (
	clientBuffer    = 256
	maxTopics       = 64
	maxClientFrame  = 16 << 10
	writeTimeout    = 10 * time.Second
	typeSubscribe   = "sub"
	typeUnsubscribe = "unsub"
	typeSubscribed  = "sub.ok"
)

func validTopic(t string) bool {
	return t == TopicNodes || t == TopicStats || nodeStatsTopic.MatchString(t)
}

// Hub fans out events to subscribed clients. Slow clients are disconnected
// instead of slowing down everyone else; they reconnect and refetch.
type Hub struct {
	OriginPatterns []string
	Log            *slog.Logger

	mu      sync.Mutex
	clients map[*client]struct{}
}

type client struct {
	send   chan []byte
	topics map[string]bool // guarded by Hub.mu
	gone   chan struct{}
	once   sync.Once
}

func (c *client) drop() { c.once.Do(func() { close(c.gone) }) }

// Publish sends an event to every client subscribed to topic.
func (h *Hub) Publish(topic, typ string, data any) {
	frame, err := proto.Encode(typ, "", time.Now(), data)
	if err != nil {
		h.Log.Error("live: encode event", "type", typ, "err", err)
		return
	}
	h.mu.Lock()
	defer h.mu.Unlock()
	for c := range h.clients {
		if !c.topics[topic] {
			continue
		}
		select {
		case c.send <- frame:
		default:
			c.drop()
		}
	}
}

// ServeHTTP accepts a client connection.
func (h *Hub) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	conn, err := websocket.Accept(w, r, &websocket.AcceptOptions{OriginPatterns: h.OriginPatterns})
	if err != nil {
		return // Accept already wrote the HTTP error
	}
	conn.SetReadLimit(maxClientFrame)
	c := &client{send: make(chan []byte, clientBuffer), topics: map[string]bool{}, gone: make(chan struct{})}
	h.mu.Lock()
	if h.clients == nil {
		h.clients = map[*client]struct{}{}
	}
	h.clients[c] = struct{}{}
	h.mu.Unlock()
	defer func() {
		h.mu.Lock()
		delete(h.clients, c)
		h.mu.Unlock()
	}()

	ctx, cancel := context.WithCancel(r.Context())
	defer cancel()
	go func() {
		defer cancel()
		h.readLoop(ctx, conn, c)
	}()

	for {
		select {
		case frame := <-c.send:
			wctx, wcancel := context.WithTimeout(ctx, writeTimeout)
			err := conn.Write(wctx, websocket.MessageText, frame)
			wcancel()
			if err != nil {
				return
			}
		case <-c.gone:
			_ = conn.Close(websocket.StatusPolicyViolation, "client too slow")
			return
		case <-ctx.Done():
			_ = conn.Close(websocket.StatusNormalClosure, "")
			return
		}
	}
}

type subscription struct {
	Topics []string `json:"topics"`
}

func (h *Hub) readLoop(ctx context.Context, conn *websocket.Conn, c *client) {
	for {
		_, data, err := conn.Read(ctx)
		if err != nil {
			return
		}
		env, err := proto.Decode(data)
		if err != nil {
			_ = conn.Close(websocket.StatusUnsupportedData, "invalid frame")
			return
		}
		var sub subscription
		if err := json.Unmarshal(env.Data, &sub); err != nil {
			_ = conn.Close(websocket.StatusUnsupportedData, "invalid subscription")
			return
		}
		for _, t := range sub.Topics {
			if !validTopic(t) {
				_ = conn.Close(websocket.StatusPolicyViolation, "unknown topic")
				return
			}
		}
		h.mu.Lock()
		switch env.Type {
		case typeSubscribe:
			for _, t := range sub.Topics {
				if len(c.topics) < maxTopics {
					c.topics[t] = true
				}
			}
		case typeUnsubscribe:
			for _, t := range sub.Topics {
				delete(c.topics, t)
			}
		}
		active := subscription{Topics: make([]string, 0, len(c.topics))}
		for t := range c.topics {
			active.Topics = append(active.Topics, t)
		}
		h.mu.Unlock()

		// Confirm, so the client knows from when on events will arrive.
		ack, err := proto.Encode(typeSubscribed, env.ID, time.Now(), active)
		if err != nil {
			return
		}
		select {
		case c.send <- ack:
		default:
			c.drop()
		}
	}
}
