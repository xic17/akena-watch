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

// vars son los valores disponibles en las plantillas de cuerpo JSON de
// los webhooks (estilo Uptime Kuma).
type vars struct {
	MonitorName string
	MonitorURL  string
	MonitorType string
	Status      string
	Msg         string
	Latency     string
	Time        string
	Localtime   string
}

// Send notifica un cambio de estado. recovery=true significa que el
// monitor volvió a estar en línea.
func (m *Manager) Send(mon store.Monitor, detail string, latencyMS int, at time.Time, recovery bool) {
	channels, err := m.st.ListNotificationsForMonitor(mon.ID)
	if err != nil || len(channels) == 0 {
		return
	}

	text := formatMessage(mon, detail, latencyMS, at, recovery)
	v := vars{
		MonitorName: mon.Name,
		MonitorURL:  mon.URL,
		MonitorType: mon.Type,
		Status:      "up",
		Msg:         detail,
		Time:        at.Format(time.RFC3339),
		Localtime:   at.Local().Format("02/01/2006 15:04:05"),
	}
	if !recovery {
		v.Status = "down"
	}
	if v.Msg == "" {
		v.Msg = "sin error reportado"
	}
	if latencyMS > 0 {
		v.Latency = fmt.Sprintf("%d ms", latencyMS)
	}

	for _, ch := range channels {
		go func(ch store.Notification) {
			if err := sendChannel(ch, text, v); err != nil {
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

// SendSlow notifica que el monitor lleva varios checks por encima del umbral
// de lentitud. recovery=false no aplica: es un estado intermedio, no una caída.
func (m *Manager) SendSlow(mon store.Monitor, detail string, latencyMS int, at time.Time) {
	channels, err := m.st.ListNotificationsForMonitor(mon.ID)
	if err != nil || len(channels) == 0 {
		return
	}
	text := formatSlowMessage(mon, detail, latencyMS, at)
	v := vars{
		MonitorName: mon.Name,
		MonitorURL:  mon.URL,
		MonitorType: mon.Type,
		Status:      "slow",
		Msg:         detail,
		Time:        at.Format(time.RFC3339),
		Localtime:   at.Local().Format("02/01/2006 15:04:05"),
	}
	if latencyMS > 0 {
		v.Latency = fmt.Sprintf("%d ms", latencyMS)
	}
	for _, ch := range channels {
		go func(ch store.Notification) {
			if err := sendChannel(ch, text, v); err != nil {
				log.Printf("alerta lenta %q (canal %s): %v", ch.Name, ch.Type, err)
			}
		}(ch)
	}
}

func formatSlowMessage(mon store.Monitor, detail string, latencyMS int, at time.Time) string {
	if detail == "" {
		detail = "latencia por encima del umbral"
	}
	var b strings.Builder
	fmt.Fprintf(&b, "🟡 *Akena Watch — Monitor lento*\n")
	fmt.Fprintf(&b, "Monitor: *%s* (%s)\n", mon.Name, mon.Type)
	fmt.Fprintf(&b, "Destino: %s\n", mon.URL)
	fmt.Fprintf(&b, "Detalle: %s\n", detail)
	if latencyMS > 0 {
		fmt.Fprintf(&b, "Latencia: %d ms\n", latencyMS)
	}
	fmt.Fprintf(&b, "Hora: %s", at.Local().Format("02/01/2006 15:04:05"))
	return b.String()
}

// SendCert notifica que el certificado TLS de un monitor HTTPS está a punto
// de expirar (o ya expiró). {{status}} = "cert" en las plantillas.
func (m *Manager) SendCert(mon store.Monitor, detail string, at time.Time) {
	channels, err := m.st.ListNotificationsForMonitor(mon.ID)
	if err != nil || len(channels) == 0 {
		return
	}
	text := formatCertMessage(mon, detail, at)
	v := vars{
		MonitorName: mon.Name,
		MonitorURL:  mon.URL,
		MonitorType: mon.Type,
		Status:      "cert",
		Msg:         detail,
		Time:        at.Format(time.RFC3339),
		Localtime:   at.Local().Format("02/01/2006 15:04:05"),
	}
	for _, ch := range channels {
		go func(ch store.Notification) {
			if err := sendChannel(ch, text, v); err != nil {
				log.Printf("alerta de certificado %q (canal %s): %v", ch.Name, ch.Type, err)
			}
		}(ch)
	}
}

func formatCertMessage(mon store.Monitor, detail string, at time.Time) string {
	if detail == "" {
		detail = "certificado a punto de expirar"
	}
	var b strings.Builder
	fmt.Fprintf(&b, "🟠 *Akena Watch — Certificado por expirar*\n")
	fmt.Fprintf(&b, "Monitor: *%s* (%s)\n", mon.Name, mon.Type)
	fmt.Fprintf(&b, "Destino: %s\n", mon.URL)
	fmt.Fprintf(&b, "Detalle: %s\n", detail)
	fmt.Fprintf(&b, "Hora: %s", at.Local().Format("02/01/2006 15:04:05"))
	return b.String()
}

// NotifyOwnerCert envía el aviso de certificado al Telegram del propietario.
func (m *Manager) NotifyOwnerCert(mon store.Monitor, detail string, at time.Time) {
	m.notifyOwnerText(mon, formatCertMessage(mon, detail, at), at)
}

// Test envía un mensaje de prueba por el canal indicado, sin tocar
// ningún monitor. Se usa desde el botón "Probar" de la interfaz.
// Las variables de la plantilla se rellenan con valores de ejemplo
// para que se vea el renderizado del cuerpo personalizado.
func (m *Manager) Test(ch store.Notification) error {
	now := time.Now()
	return sendChannel(ch, testMessage, vars{
		MonitorName: "Monitor de prueba",
		MonitorURL:  "https://ejemplo.com",
		MonitorType: "http",
		Status:      "up",
		Msg:         "Mensaje de prueba — si recibes esto, todo funciona.",
		Latency:     "45 ms",
		Time:        now.Format(time.RFC3339),
		Localtime:   now.Local().Format("02/01/2006 15:04:05"),
	})
}

const testMessage = "🧪 Prueba de canal de Akena Watch — si recibes esto, todo funciona.\nSiempre en Guardia."

// NotifyOwner envía la alerta directamente al Telegram del propietario
// del monitor (su ID de perfil), usando el bot de su primer canal de
// Telegram. Se ignora en silencio si no hay ID o canal configurado.
func (m *Manager) NotifyOwner(mon store.Monitor, detail string, latencyMS int, at time.Time, recovery bool) {
	m.notifyOwnerText(mon, formatMessage(mon, detail, latencyMS, at, recovery), at)
}

// NotifyOwnerSlow envía el aviso de lentitud al Telegram del propietario.
func (m *Manager) NotifyOwnerSlow(mon store.Monitor, detail string, latencyMS int, at time.Time) {
	m.notifyOwnerText(mon, formatSlowMessage(mon, detail, latencyMS, at), at)
}

func (m *Manager) notifyOwnerText(mon store.Monitor, text string, at time.Time) {
	owner, err := m.st.GetUserByID(mon.OwnerID)
	if err != nil || strings.TrimSpace(owner.TelegramID) == "" {
		return
	}
	ch, err := m.st.FirstTelegramChannel(mon.OwnerID)
	if err != nil {
		return
	}
	var cfg telegramConfig
	if err := json.Unmarshal([]byte(ch.Config), &cfg); err != nil {
		return
	}
	if cfg.BotToken == "" {
		return
	}
	go func() {
		client := &http.Client{Timeout: 15 * time.Second}
		if err := sendTelegramMsg(client, cfg.BotToken, owner.TelegramID, text); err != nil {
			log.Printf("aviso al propietario %q (monitor %d): %v", owner.Username, mon.ID, err)
		}
	}()
}

// --- Canales ---

func sendChannel(ch store.Notification, text string, v vars) error {
	client := &http.Client{Timeout: 15 * time.Second}
	switch ch.Type {
	case store.NotifWebhook:
		return sendWebhook(client, ch.Config, text, v)
	case store.NotifTelegram:
		return sendTelegram(client, ch.Config, text)
	case store.NotifSMTP:
		return sendSMTP(ch.Config, text)
	}
	return fmt.Errorf("tipo de canal desconocido: %s", ch.Type)
}

type webhookConfig struct {
	URL  string `json:"url"`
	Body string `json:"body"` // plantilla JSON con {{variables}}
}

func sendWebhook(client *http.Client, cfgJSON, text string, v vars) error {
	var cfg webhookConfig
	if err := json.Unmarshal([]byte(cfgJSON), &cfg); err != nil {
		return err
	}
	if cfg.URL == "" {
		return fmt.Errorf("URL de webhook vacía")
	}

	var payload []byte
	if cfg.Body != "" {
		payload = []byte(expandTemplate(cfg.Body, v))
		if !json.Valid(payload) {
			return fmt.Errorf("el cuerpo personalizado no produce JSON válido (revisa comillas y llaves)")
		}
	} else {
		payload, _ = json.Marshal(map[string]string{"text": text})
	}
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

// expandTemplate reemplaza las variables {{...}} del cuerpo JSON
// personalizado. Los valores se escapan como JSON para poder usarse
// dentro de comillas: "{{msg}}" -> "detalle con \"comillas\"".
func expandTemplate(tpl string, v vars) string {
	r := strings.NewReplacer(
		"{{monitorName}}", jsonEsc(v.MonitorName),
		"{{monitorUrl}}", jsonEsc(v.MonitorURL),
		"{{monitorType}}", jsonEsc(v.MonitorType),
		"{{status}}", jsonEsc(v.Status),
		"{{msg}}", jsonEsc(v.Msg),
		"{{latency}}", jsonEsc(v.Latency),
		"{{time}}", jsonEsc(v.Time),
		"{{localtime}}", jsonEsc(v.Localtime),
	)
	return r.Replace(tpl)
}

// jsonEsc escapa un valor para incrustarlo dentro de una cadena JSON.
func jsonEsc(s string) string {
	b, err := json.Marshal(s)
	if err != nil || len(b) < 2 {
		return s
	}
	return string(b[1 : len(b)-1])
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
	return sendTelegramMsg(client, cfg.BotToken, cfg.ChatID, text)
}

// sendTelegramMsg envía un mensaje con un bot concreto a un chat concreto.
func sendTelegramMsg(client *http.Client, botToken, chatID, text string) error {
	if botToken == "" || chatID == "" {
		return fmt.Errorf("configuración de Telegram incompleta")
	}
	payload, _ := json.Marshal(map[string]any{
		"chat_id":                  chatID,
		"text":                     text,
		"parse_mode":               "Markdown",
		"disable_web_page_preview": true,
	})
	url := "https://api.telegram.org/bot" + botToken + "/sendMessage"
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
