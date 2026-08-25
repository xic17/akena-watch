package monitor

import (
	"context"
	"log"
	"sync"
	"time"

	"akena-watch/internal/notifier"
	"akena-watch/internal/store"
)

// Broadcaster publica eventos en tiempo real hacia un conjunto de usuarios.
// Lo implementa el hub de WebSockets del servidor.
type Broadcaster interface {
	Publish(userIDs []int64, evt map[string]any)
}

// Scheduler ejecuta los checks de los monitores activos según su intervalo.
type Scheduler struct {
	st     *store.Store
	hub    Broadcaster
	notify *notifier.Manager

	mu    sync.Mutex
	state map[int64]*monState

	stop chan struct{}
	done chan struct{}
}

type monState struct {
	lastRun             time.Time
	status              string // "up" | "down" | "" (sin historial)
	consecutiveFailures int
	downAlerted         bool
}

// NewScheduler crea un scheduler. Hub puede ser nil (sin tiempo real).
func NewScheduler(st *store.Store, hub Broadcaster, notify *notifier.Manager) *Scheduler {
	return &Scheduler{
		st:     st,
		hub:    hub,
		notify: notify,
		state:  make(map[int64]*monState),
		stop:   make(chan struct{}),
		done:   make(chan struct{}),
	}
}

// Start arranca el bucle de checks en segundo plano.
func (s *Scheduler) Start() {
	go s.loop()
}

// Stop detiene el bucle y espera a que termine.
func (s *Scheduler) Stop() {
	close(s.stop)
	<-s.done
}

func (s *Scheduler) loop() {
	defer close(s.done)

	s.loadInitialState()
	tick := time.NewTicker(time.Second)
	defer tick.Stop()

	lastPrune := time.Now()
	for {
		select {
		case <-s.stop:
			return
		case now := <-tick.C:
			s.tick(now)
			if time.Since(lastPrune) >= time.Hour {
				if err := s.st.PruneHeartbeats(5000); err != nil {
					log.Printf("prune de heartbeats: %v", err)
				}
				lastPrune = time.Now()
			}
		}
	}
}

// loadInitialState recupera el estado previo desde el último heartbeat,
// para no alertar en falso al reiniciar un monitor que ya estaba caído.
func (s *Scheduler) loadInitialState() {
	monitors, err := s.st.ListActiveMonitors()
	if err != nil {
		log.Printf("cargando estado inicial: %v", err)
		return
	}
	for _, m := range monitors {
		hb, err := s.st.LatestHeartbeat(m.ID)
		if err != nil {
			continue
		}
		st := &monState{lastRun: hb.CheckedAt, status: hb.Status}
		if hb.Status == store.StatusDown {
			st.consecutiveFailures = 1
		}
		s.mu.Lock()
		s.state[m.ID] = st
		s.mu.Unlock()
	}
}

func (s *Scheduler) tick(now time.Time) {
	monitors, err := s.st.ListActiveMonitors()
	if err != nil {
		log.Printf("listando monitores activos: %v", err)
		return
	}

	for _, m := range monitors {
		s.mu.Lock()
		st, ok := s.state[m.ID]
		if !ok {
			st = &monState{}
			s.state[m.ID] = st
		}
		due := now.Sub(st.lastRun) >= time.Duration(m.IntervalS)*time.Second
		if due {
			st.lastRun = now // reserva el slot: evita ejecuciones duplicadas
		}
		s.mu.Unlock()

		if due {
			go s.run(m, now)
		}
	}
}

func (s *Scheduler) run(m store.Monitor, now time.Time) {
	ctx, cancel := context.WithTimeout(context.Background(), time.Duration(m.TimeoutS+5)*time.Second)
	defer cancel()

	res := Check(ctx, m)
	hb := store.Heartbeat{
		MonitorID: m.ID,
		Status:    res.Status,
		Code:      res.Code,
		LatencyMS: res.LatencyMS,
		Error:     res.Error,
		CheckedAt: now,
	}
	if err := s.st.InsertHeartbeat(hb); err != nil {
		log.Printf("guardando heartbeat (monitor %d): %v", m.ID, err)
	}

	s.mu.Lock()
	st := s.state[m.ID]
	if res.Status == store.StatusUp {
		st.consecutiveFailures = 0
	} else {
		st.consecutiveFailures++
	}
	transition := st.status != "" && st.status != res.Status
	st.status = res.Status

	if m.Notify && s.notify != nil {
		if res.Status == store.StatusDown && st.consecutiveFailures >= m.MaxRetries && !st.downAlerted {
			st.downAlerted = true
			go s.notify.Send(m, res.Error, res.LatencyMS, hb.CheckedAt, false)
		} else if res.Status == store.StatusUp && st.downAlerted {
			st.downAlerted = false
			go s.notify.Send(m, res.Error, res.LatencyMS, hb.CheckedAt, true)
		}
	}
	s.mu.Unlock()

	_ = transition // por ahora la transición se refleja en el heartbeat y el WS

	if s.hub != nil {
		viewers, err := s.st.ListMonitorViewerIDs(m.ID)
		if err == nil {
			s.hub.Publish(viewers, map[string]any{
				"type": "heartbeat",
				"monitor": map[string]any{
					"id":         m.ID,
					"name":       m.Name,
					"status":     res.Status,
					"latency_ms": res.LatencyMS,
					"error":      res.Error,
					"checked_at": hb.CheckedAt.Format(time.RFC3339),
				},
			})
		}
	}
}
