package websocket

import (
	"sync"

	"github.com/gofiber/websocket/v2"
	"github.com/rs/zerolog/log"
)

// Client represents a connected WebSocket client.
type Client struct {
	Conn   *websocket.Conn
	UserID string
	Send   chan []byte
}

// Hub maintains all active WebSocket connections.
type Hub struct {
	clients    map[string]*Client
	broadcast  chan []byte
	register   chan *Client
	unregister chan *Client
	mu         sync.RWMutex
}

// NewHub creates a new WebSocket hub.
func NewHub() *Hub {
	return &Hub{
		clients:    make(map[string]*Client),
		broadcast:  make(chan []byte, 256),
		register:   make(chan *Client),
		unregister: make(chan *Client),
	}
}

// Run starts the hub's event loop.
func (h *Hub) Run() {
	for {
		select {
		case client := <-h.register:
			h.mu.Lock()
			h.clients[client.UserID] = client
			h.mu.Unlock()
			log.Info().Str("user_id", client.UserID).Msg("WebSocket client connected")

		case client := <-h.unregister:
			h.mu.Lock()
			if _, ok := h.clients[client.UserID]; ok {
				delete(h.clients, client.UserID)
				close(client.Send)
			}
			h.mu.Unlock()
			log.Info().Str("user_id", client.UserID).Msg("WebSocket client disconnected")

		case message := <-h.broadcast:
			h.mu.RLock()
			for _, client := range h.clients {
				select {
				case client.Send <- message:
				default:
					close(client.Send)
					delete(h.clients, client.UserID)
				}
			}
			h.mu.RUnlock()
		}
	}
}

// SendToUser sends a message to a specific user's WebSocket connection.
func (h *Hub) SendToUser(userID string, message []byte) {
	h.mu.RLock()
	client, ok := h.clients[userID]
	h.mu.RUnlock()

	if ok {
		select {
		case client.Send <- message:
		default:
			h.mu.Lock()
			close(client.Send)
			delete(h.clients, userID)
			h.mu.Unlock()
		}
	}
}

// Broadcast sends a message to all connected clients.
func (h *Hub) Broadcast(message []byte) {
	h.broadcast <- message
}

// HandleConnection handles a new WebSocket connection.
func (h *Hub) HandleConnection(conn *websocket.Conn, userID string) {
	client := &Client{
		Conn:   conn,
		UserID: userID,
		Send:   make(chan []byte, 256),
	}

	h.register <- client

	// Write pump
	go func() {
		defer func() {
			h.unregister <- client
			conn.Close()
		}()

		for message := range client.Send {
			if err := conn.WriteMessage(websocket.TextMessage, message); err != nil {
				log.Error().Err(err).Str("user_id", userID).Msg("WebSocket write error")
				return
			}
		}
	}()

	// Read pump
	for {
		_, _, err := conn.ReadMessage()
		if err != nil {
			if websocket.IsUnexpectedCloseError(err, websocket.CloseGoingAway, websocket.CloseAbnormalClosure) {
				log.Error().Err(err).Str("user_id", userID).Msg("WebSocket read error")
			}
			break
		}
	}
}
