package live

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http/httptest"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/coder/websocket"

	"github.com/eronbubi/smartcluster/pkg/proto"
	"github.com/eronbubi/smartcluster/pkg/testenv"
)

const nodeID = "0192f1c4-7e2a-7b3c-9d4e-5f6a7b8c9d0e"

func newHub(t *testing.T) (*Hub, string) {
	t.Helper()
	testenv.RequireServers(t)
	h := &Hub{Log: slog.New(slog.NewTextHandler(io.Discard, nil))}
	srv := httptest.NewServer(h)
	t.Cleanup(srv.Close)
	return h, "ws" + strings.TrimPrefix(srv.URL, "http")
}

func dial(t *testing.T, url string) *websocket.Conn {
	t.Helper()
	c, _, err := websocket.Dial(context.Background(), url, nil) //nolint:bodyclose // websocket.Dial owns the body: "You never need to close resp.Body yourself."
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = c.CloseNow() })
	return c
}

func next(t *testing.T, c *websocket.Conn) proto.Envelope {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	_, data, err := c.Read(ctx)
	if err != nil {
		t.Fatal(err)
	}
	env, err := proto.Decode(data)
	if err != nil {
		t.Fatal(err)
	}
	return env
}

// change sends sub or unsub and waits for the confirmation.
func change(t *testing.T, c *websocket.Conn, typ string, topics ...string) []string {
	t.Helper()
	frame, _ := proto.Encode(typ, "req-1", time.Now(), subscription{Topics: topics})
	if err := c.Write(context.Background(), websocket.MessageText, frame); err != nil {
		t.Fatal(err)
	}
	env := next(t, c)
	if env.Type != typeSubscribed || env.ID != "req-1" {
		t.Fatalf("got %s (%s), want sub.ok", env.Type, env.ID)
	}
	var active subscription
	_ = json.Unmarshal(env.Data, &active)
	slices.Sort(active.Topics)
	return active.Topics
}

func TestSubscribeAndReceive(t *testing.T) {
	h, url := newHub(t)
	c := dial(t, url)
	if active := change(t, c, typeSubscribe, TopicNodeStats(nodeID), TopicNodes); len(active) != 2 {
		t.Fatalf("active topics %v", active)
	}

	h.Publish(TopicNodeStats(nodeID), TypeNodeStats, map[string]string{"node_id": nodeID})
	env := next(t, c)
	var data map[string]string
	_ = json.Unmarshal(env.Data, &data)
	if env.Type != TypeNodeStats || data["node_id"] != nodeID {
		t.Errorf("got %s %v", env.Type, data)
	}
}

func TestOnlySubscribedTopicsArrive(t *testing.T) {
	h, url := newHub(t)
	c := dial(t, url)
	change(t, c, typeSubscribe, TopicNodes)

	h.Publish(TopicStats, TypeNodeStats, "not subscribed")
	h.Publish(TopicNodes, TypeNodeStatus, "offline")
	if env := next(t, c); env.Type != TypeNodeStatus || string(env.Data) != `"offline"` {
		t.Errorf("got %s %s, want only the node.status event", env.Type, env.Data)
	}

	if active := change(t, c, typeUnsubscribe, TopicNodes); len(active) != 0 {
		t.Fatalf("still subscribed to %v", active)
	}
	h.Publish(TopicNodes, TypeNodeStatus, "after unsubscribe")
	ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
	defer cancel()
	if _, _, err := c.Read(ctx); err == nil {
		t.Error("received an event after unsubscribing")
	}
}

func TestUnknownTopicClosesConnection(t *testing.T) {
	_, url := newHub(t)
	c := dial(t, url)
	frame, _ := proto.Encode(typeSubscribe, "", time.Now(), subscription{Topics: []string{"node:../../etc:stats"}})
	_ = c.Write(context.Background(), websocket.MessageText, frame)
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	_, _, err := c.Read(ctx)
	if websocket.CloseStatus(err) != websocket.StatusPolicyViolation {
		t.Errorf("got %v, want policy violation close", err)
	}
}

func TestSlowClientIsDropped(t *testing.T) {
	h := &Hub{Log: slog.New(slog.NewTextHandler(io.Discard, nil))}
	c := &client{send: make(chan []byte, 1), topics: map[string]bool{TopicStats: true}, gone: make(chan struct{})}
	h.clients = map[*client]struct{}{c: {}}
	h.Publish(TopicStats, TypeNodeStats, 1)
	h.Publish(TopicStats, TypeNodeStats, 2) // buffer full
	select {
	case <-c.gone:
	default:
		t.Error("slow client was not dropped")
	}
}
