package notifier

import (
	"strings"
	"testing"
	"time"

	"akena-watch/internal/store"
)

// TestFormatDomainMessage comprueba el texto del aviso de vencimiento de
// dominio: lo que verá quien lo reciba por Telegram o por un webhook.
func TestFormatDomainMessage(t *testing.T) {
	mon := store.Monitor{Name: "Web principal", URL: "https://ejemplo.com", Type: "http"}
	at := time.Date(2026, 10, 15, 12, 0, 0, 0, time.UTC)

	texto := formatDomainMessage(mon, "el dominio vence en 21 días (2026-11-05)", at)
	for _, esperado := range []string{
		"Dominio por vencer",
		"Web principal",
		"https://ejemplo.com",
		"el dominio vence en 21 días (2026-11-05)",
	} {
		if !strings.Contains(texto, esperado) {
			t.Fatalf("el mensaje no contiene %q:\n%s", esperado, texto)
		}
	}

	if !strings.Contains(formatDomainMessage(mon, "", at), "a punto de vencer") {
		t.Fatal("sin detalle debe usarse el texto por defecto")
	}
}
