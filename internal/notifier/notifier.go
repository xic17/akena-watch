// Package notifier envía alertas por los canales configurados
// (webhook, Telegram, email SMTP) cuando un monitor cambia de estado.
package notifier

import (
	"bytes"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"net/smtp"
	"strings"
	"time"

	"akena-watch/internal/store"
)

// Manager despacha alertas usando los canales asociados a cada monitor.
type Manager struct {
	st *store.Store
}

func NewManager(st *store.Store) *Manager { return &Manager{st: st} }

// Send notifica un cambio de estado. recovery=true significa que el
// monitor volvió a estar en línea.
func (m *Manager) Send(mon store.Monitor, detail string, latencyMS int, at time.Time, recovery bool) {
	channels, err := m.st.ListNotificationsForMonitor(mon.ID)
	if err != nil || len(channels) == 0 {
		return
	}

	text := formatMessage(mon, detail, latencyMS, at, recovery)
	for _, ch := range channels {
		go func(ch store.Notification) {
			if err := sendChannel(ch, text); err != nil {
				log.Printf("alerta %q (canal %s): %v", ch.Name, ch.Type, err)
			}
		}(ch)
	}
}

func formatMessage(mon store.Monitor, detail string, latencyMS int, at time.Time, recovery bool) string {
	icon := "🟢"
	title := "Recuperado"
	if !recovery {
		icon = "🔴"
		title = "Monitor caído"
	}
	if detail == "" {
		detail = "sin error reportado"
	}
	var b strings.Builder
	fmt.Fprintf(&b, "%s *Akena Watch — %s*\n", icon, title)
	fmt.Fprintf(&b, "Monitor: *%s* (%s)\n", mon.Name, mon.Type)
	fmt.Fprintf(&b, "Destino: %s\n", mon.URL)
	fmt.Fprintf(&b, "Detalle: %s\n", detail)
	if latencyMS > 0 {
		fmt.Fprintf(&b, "Latencia: %d ms\n", latencyMS)
	}
	fmt.Fprintf(&b, "Hora: %s", at.Local().Format("02/01/2006 15:04:05"))
	return b.String()
}

// --- Canales ---

func sendChannel(ch store.Notification, text string) error {
	client := &http.Client{Timeout: 15 * time.Second}
	switch ch.Type {
	case store.NotifWebhook:
		return sendWebhook(client, ch.Config, text)
	case store.NotifTelegram:
		return sendTelegram(client, ch.Config, text)
	case store.NotifSMTP:
		return sendSMTP(ch.Config, text)
	}
	return fmt.Errorf("tipo de canal desconocido: %s", ch.Type)
}

type webhookConfig struct {
	URL string `json:"url"`
}

func sendWebhook(client *http.Client, cfgJSON, text string) error {
	var cfg webhookConfig
	if err := json.Unmarshal([]byte(cfgJSON), &cfg); err != nil {
		return err
	}
	if cfg.URL == "" {
		return fmt.Errorf("URL de webhook vacía")
	}
	payload, _ := json.Marshal(map[string]string{"text": text})
	req, err := http.NewRequest(http.MethodPost, cfg.URL, bytes.NewReader(payload))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 400 {
		return fmt.Errorf("webhook respondió %d", resp.StatusCode)
	}
	return nil
}

type telegramConfig struct {
	BotToken string `json:"bot_token"`
	ChatID   string `json:"chat_id"`
}

func sendTelegram(client *http.Client, cfgJSON, text string) error {
	var cfg telegramConfig
	if err := json.Unmarshal([]byte(cfgJSON), &cfg); err != nil {
		return err
	}
	if cfg.BotToken == "" || cfg.ChatID == "" {
		return fmt.Errorf("configuración de Telegram incompleta")
	}
	payload, _ := json.Marshal(map[string]any{
		"chat_id":                  cfg.ChatID,
		"text":                     text,
		"parse_mode":               "Markdown",
		"disable_web_page_preview": true,
	})
	url := "https://api.telegram.org/bot" + cfg.BotToken + "/sendMessage"
	resp, err := client.Post(url, "application/json", bytes.NewReader(payload))
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 400 {
		var body struct {
			Description string `json:"description"`
		}
		_ = json.NewDecoder(resp.Body).Decode(&body)
		return fmt.Errorf("Telegram respondió %d: %s", resp.StatusCode, body.Description)
	}
	return nil
}

type smtpConfig struct {
	Host string `json:"host"`
	Port int    `json:"port"`
	User string `json:"user"`
	Pass string `json:"pass"`
	From string `json:"from"`
	To   string `json:"to"`
}

func sendSMTP(cfgJSON, text string) error {
	var cfg smtpConfig
	if err := json.Unmarshal([]byte(cfgJSON), &cfg); err != nil {
		return err
	}
	if cfg.Host == "" || cfg.Port == 0 || cfg.From == "" || cfg.To == "" {
		return fmt.Errorf("configuración SMTP incompleta")
	}

	subject := "Akena Watch — alerta"
	msg := "From: " + cfg.From + "\r\n" +
		"To: " + cfg.To + "\r\n" +
		"Subject: " + subject + "\r\n" +
		"MIME-Version: 1.0\r\n" +
		"Content-Type: text/plain; charset=UTF-8\r\n\r\n" +
		text

	addr := fmt.Sprintf("%s:%d", cfg.Host, cfg.Port)
	var auth smtp.Auth
	if cfg.User != "" {
		auth = smtp.PlainAuth("", cfg.User, cfg.Pass, cfg.Host)
	}
	if err := smtp.SendMail(addr, auth, cfg.From, []string{cfg.To}, []byte(msg)); err != nil {
		return err
	}
	return nil
}
