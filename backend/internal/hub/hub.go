// Package hub implements a WebSocket fan-out hub for broadcasting realtime
// monitor status updates to connected clients.
package hub

import (
	"encoding/json"
	"log"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/gorilla/websocket"
)

const (
	// writeWait is the maximum time allowed to write a message to the peer.
	writeWait = 10 * time.Second
	// pongWait is the maximum time to wait for a pong from the peer.
	pongWait = 60 * time.Second
	// pingPeriod must be less than pongWait.
	pingPeriod = (pongWait * 9) / 10
	// maxMessageSize is the maximum inbound message size (clients don't send much).
	maxMessageSize = 512
	// sendBufferSize is the per-client outbound message buffer.
	sendBufferSize = 256
)

// Message is the envelope for all WebSocket messages sent to clients.
type Message struct {
	Type    string      `json:"type"`
	Payload interface{} `json:"payload"`
}

// Client represents a single WebSocket connection managed by the Hub.
type Client struct {
	ID   string
	hub  *Hub
	conn *websocket.Conn
	send chan []byte
}

// Hub maintains the set of active clients and broadcasts messages to them.
type Hub struct {
	clients    map[*Client]struct{}
	broadcast  chan []byte
	register   chan *Client
	unregister chan *Client
	mu         sync.RWMutex // protects ClientCount reads from outside the run loop
	done       chan struct{}
}

// New creates a new Hub. Call Run() to start the event loop.
func New() *Hub {
	return &Hub{
		clients:    make(map[*Client]struct{}),
		broadcast:  make(chan []byte, 256),
		register:   make(chan *Client),
		unregister: make(chan *Client),
		done:       make(chan struct{}),
	}
}

// Run starts the hub event loop. It blocks until the done channel is closed.
// Typically called as: go hub.Run()
func (h *Hub) Run() {
	log.Printf("hub: started")
	for {
		select {
		case client := <-h.register:
			h.clients[client] = struct{}{}
			log.Printf("hub: client %s connected (total=%d)", client.ID, len(h.clients))

		case client := <-h.unregister:
			if _, ok := h.clients[client]; ok {
				delete(h.clients, client)
				close(client.send)
				log.Printf("hub: client %s disconnected (total=%d)", client.ID, len(h.clients))
			}

		case message := <-h.broadcast:
			for client := range h.clients {
				select {
				case client.send <- message:
				default:
					// Client send buffer full — disconnect slow consumer.
					delete(h.clients, client)
					close(client.send)
					log.Printf("hub: client %s dropped (slow consumer)", client.ID)
				}
			}

		case <-h.done:
			// Shutdown: close all client connections.
			for client := range h.clients {
				close(client.send)
				delete(h.clients, client)
			}
			log.Printf("hub: stopped")
			return
		}
	}
}

// Stop signals the hub to shut down and close all client connections.
func (h *Hub) Stop() {
	close(h.done)
}

// Broadcast encodes the message as JSON and sends it to all connected clients.
// Non-blocking: if the broadcast channel is full, the message is dropped with a log warning.
func (h *Hub) Broadcast(msg Message) {
	data, err := json.Marshal(msg)
	if err != nil {
		log.Printf("hub: marshal broadcast: %v", err)
		return
	}

	select {
	case h.broadcast <- data:
	default:
		log.Printf("hub: broadcast channel full, message dropped")
	}
}

// ClientCount returns the current number of connected clients.
func (h *Hub) ClientCount() int {
	h.mu.RLock()
	defer h.mu.RUnlock()
	return len(h.clients)
}

// RegisterClient upgrades an HTTP connection to WebSocket and registers it with the hub.
// Returns the new client or an error. The caller is responsible for calling
// this after authentication has been verified.
func (h *Hub) RegisterClient(conn *websocket.Conn) *Client {
	client := &Client{
		ID:   uuid.NewString(),
		hub:  h,
		conn: conn,
		send: make(chan []byte, sendBufferSize),
	}

	h.register <- client

	// Start write and read pumps for this client.
	go client.writePump()
	go client.readPump()

	return client
}

// writePump pumps messages from the hub to the WebSocket connection.
func (c *Client) writePump() {
	ticker := time.NewTicker(pingPeriod)
	defer func() {
		ticker.Stop()
		c.conn.Close()
	}()

	for {
		select {
		case message, ok := <-c.send:
			c.conn.SetWriteDeadline(time.Now().Add(writeWait))
			if !ok {
				// Hub closed the channel.
				c.conn.WriteMessage(websocket.CloseMessage, []byte{})
				return
			}

			w, err := c.conn.NextWriter(websocket.TextMessage)
			if err != nil {
				return
			}
			w.Write(message)

			// Drain queued messages into the same write to reduce syscalls.
			n := len(c.send)
			for i := 0; i < n; i++ {
				w.Write([]byte("\n"))
				w.Write(<-c.send)
			}

			if err := w.Close(); err != nil {
				return
			}

		case <-ticker.C:
			c.conn.SetWriteDeadline(time.Now().Add(writeWait))
			if err := c.conn.WriteMessage(websocket.PingMessage, nil); err != nil {
				return
			}
		}
	}
}

// readPump pumps messages from the WebSocket connection to the hub.
// For Pulse, clients don't send meaningful messages — this just handles
// pong responses and detects disconnection.
func (c *Client) readPump() {
	defer func() {
		c.hub.unregister <- c
		c.conn.Close()
	}()

	c.conn.SetReadLimit(maxMessageSize)
	c.conn.SetReadDeadline(time.Now().Add(pongWait))
	c.conn.SetPongHandler(func(string) error {
		c.conn.SetReadDeadline(time.Now().Add(pongWait))
		return nil
	})

	for {
		_, _, err := c.conn.ReadMessage()
		if err != nil {
			if websocket.IsUnexpectedCloseError(err, websocket.CloseGoingAway, websocket.CloseNormalClosure) {
				log.Printf("hub: client %s read error: %v", c.ID, err)
			}
			break
		}
	}
}
