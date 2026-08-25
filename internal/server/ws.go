package server

import (
	"net/http"
	"time"

	"github.com/gorilla/websocket"
)

var upgrader = websocket.Upgrader{
	ReadBufferSize:  1024,
	WriteBufferSize: 1024,
	CheckOrigin:     func(*http.Request) bool { return true },
}

// handleWS sirve el canal de tiempo real. La autenticación viaja en la
// cookie (misma origen) o, como respaldo, en el parámetro ?token=.
func (s *Server) handleWS(w http.ResponseWriter, r *http.Request) {
	u, err := s.currentUser(r)
	if err != nil {
		if tok := r.URL.Query().Get("token"); tok != "" {
			u, err = s.st.UserForSession(tok)
		}
		if err != nil {
			writeErr(w, http.StatusUnauthorized, "sesión inválida")
			return
		}
	}

	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		return
	}
	s.hub.Add(u.ID, conn)
	defer s.hub.Remove(u.ID, conn)

	// El cliente no envía mensajes: solo mantenemos la conexión viva
	// con pings del protocolo WebSocket.
	conn.SetReadLimit(1024)
	conn.SetReadDeadline(time.Now().Add(90 * time.Second))
	conn.SetPongHandler(func(string) error {
		conn.SetReadDeadline(time.Now().Add(90 * time.Second))
		return nil
	})
	for {
		if _, _, err := conn.ReadMessage(); err != nil {
			return
		}
	}
}
