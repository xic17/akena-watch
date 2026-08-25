package server

import (
	"encoding/json"
	"sync"

	"github.com/gorilla/websocket"
)

// Hub mantiene las conexiones WebSocket por usuario y publica eventos
// en tiempo real. Lo usa el scheduler para reflejar cada heartbeat.
type Hub struct {
	mu      sync.RWMutex
	clients map[int64]map[*websocket.Conn]*sync.Mutex
}

func NewHub() *Hub {
	return &Hub{clients: make(map[int64]map[*websocket.Conn]*sync.Mutex)}
}

func (h *Hub) Add(userID int64, c *websocket.Conn) {
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.clients[userID] == nil {
		h.clients[userID] = make(map[*websocket.Conn]*sync.Mutex)
	}
	h.clients[userID][c] = &sync.Mutex{}
}

func (h *Hub) Remove(userID int64, c *websocket.Conn) {
	h.mu.Lock()
	defer h.mu.Unlock()
	if m := h.clients[userID]; m != nil {
		delete(m, c)
		if len(m) == 0 {
			delete(h.clients, userID)
		}
	}
}

// Publish envía un evento a todos los usuarios indicados.
// El mutex por conexión evita escrituras concurrentes sobre el socket.
func (h *Hub) Publish(userIDs []int64, evt map[string]any) {
	if len(userIDs) == 0 {
		return
	}
	data, err := json.Marshal(evt)
	if err != nil {
		return
	}

	h.mu.RLock()
	defer h.mu.RUnlock()

	seen := make(map[*websocket.Conn]bool)
	for _, uid := range userIDs {
		for c, wmu := range h.clients[uid] {
			if seen[c] {
				continue
			}
			seen[c] = true
			wmu.Lock()
			_ = c.WriteMessage(websocket.TextMessage, data)
			wmu.Unlock()
			// si falla, la conexión se descarta cuando el cliente se desconecte
		}
	}
}
