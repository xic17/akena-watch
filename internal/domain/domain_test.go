package domain

import (
	"context"
	"errors"
	"testing"
	"time"
)

// Los textos son extractos reales de distintos registros, para cubrir los
// formatos que se encuentran de verdad (incluidos los avisos que mencionan la
// fecha de expiración sin darla).
func TestFechaEnTexto(t *testing.T) {
	casos := []struct {
		nombre   string
		texto    string
		esperado string // "" = ErrSinFecha
	}{
		{
			nombre: "gTLD con avisos del registrador",
			texto: `   Domain Name: AKENAWATCH.COM
   Registry Domain ID: 1234567_DOMAIN_COM-VRSN
   Registrar WHOIS Server: whois.namecheap.com
   Updated Date: 2026-08-26T00:09:44Z
   Creation Date: 2025-08-26T00:09:44Z
   Registry Expiry Date: 2027-08-26T00:09:44Z
   Registrar: NameCheap, Inc.
   NOTICE: The expiration date displayed in this record is the date the
   registrar's sponsorship of the domain name registration in the registry is
   currently set to expire. This date does not necessarily reflect the expiration
   Registrar Registration Expiration Date: 2027-08-26T00:09:44Z`,
			esperado: "2027-08-26",
		},
		{
			nombre:   "org",
			texto:    "Registry Expiry Date: 2027-08-30T04:00:00Z\n",
			esperado: "2027-08-30",
		},
		{
			nombre:   "cl con hora y zona",
			texto:    "Expiration date: 2030-02-08 21:00:00 CLST\n",
			esperado: "2030-02-08",
		},
		{
			nombre:   "mx con espacios de más y nota en español",
			texto:    "Expiration Date:   2027-02-23\n% NOTA: La fecha de expiracion mostrada en esta consulta es la fecha\n",
			esperado: "2027-02-23",
		},
		{
			nombre:   "uk con mes abreviado",
			texto:    "    Expiry date:  15-Sep-2026\n",
			esperado: "2026-09-15",
		},
		{
			nombre:   "uk con mes en mayúsculas",
			texto:    "Expiry date: 15-SEP-2026\n",
			esperado: "2026-09-15",
		},
		{
			nombre:   "br con formato compacto",
			texto:    "domain:      ejemplo.com.br\nexpires:     20260915\n",
			esperado: "2026-09-15",
		},
		{
			nombre:   "mes con nombre largo",
			texto:    "Expiration Date: September 15, 2026\n",
			esperado: "2026-09-15",
		},
		{
			nombre:   "año.mes.día",
			texto:    "Expiration Date: 2026.09.15\n",
			esperado: "2026-09-15",
		},
		{
			nombre:   "día.mes.año",
			texto:    "Expiry Date: 15.09.2026\n",
			esperado: "2026-09-15",
		},
		{
			nombre:   "etiqueta en español",
			texto:    "Fecha de vencimiento: 15-Sep-2026\n",
			esperado: "2026-09-15",
		},
		{
			nombre:   "paid-till",
			texto:    "paid-till: 20260915\n",
			esperado: "2026-09-15",
		},
		{
			// DENIC no publica la fecha de vencimiento en su WHOIS.
			nombre:   "de sin fecha publicada",
			texto:    "% Copyright (c) 2010 by DENIC\nDomain: ejemplo.de\nStatus: connect\n",
			esperado: "",
		},
		{
			nombre:   "solo avisos sin fecha propia",
			texto:    "NOTICE: The expiration date displayed in this record is the date\nregistrar's sponsorship is set to expire.\n",
			esperado: "",
		},
		{
			nombre:   "valor vacío",
			texto:    "Registry Expiry Date: -\n",
			esperado: "",
		},
		{
			nombre:   "fecha imposible",
			texto:    "Registry Expiry Date: 0000-00-00\n",
			esperado: "",
		},
		{
			nombre:   "texto vacío",
			texto:    "",
			esperado: "",
		},
	}

	for _, c := range casos {
		t.Run(c.nombre, func(t *testing.T) {
			got, err := FechaEnTexto(c.texto)
			if c.esperado == "" {
				if !errors.Is(err, ErrSinFecha) {
					t.Fatalf("esperaba ErrSinFecha, got %v (%v)", err, got)
				}
				return
			}
			if err != nil {
				t.Fatalf("error inesperado: %v", err)
			}
			want, _ := time.Parse("2006-01-02", c.esperado)
			if !got.Equal(want) {
				t.Fatalf("fecha = %s, esperado %s", got.Format("2006-01-02"), c.esperado)
			}
		})
	}
}

func TestNormalizar(t *testing.T) {
	casos := map[string]string{
		"https://ejemplo.com/ruta?x=1":   "ejemplo.com",
		"http://sub.ejemplo.com:8080/":   "sub.ejemplo.com",
		"EJEMPLO.COM.":                   "ejemplo.com",
		"ejemplo.com:443":                "ejemplo.com",
		"  ejemplo.com  ":                "ejemplo.com",
		"ejemplo.com/path":               "ejemplo.com",
		"www.ejemplo.co.uk:8443":         "www.ejemplo.co.uk",
		"https://panel.api.ejemplo.com/": "panel.api.ejemplo.com",
		"":                               "",
		"   ":                            "",
	}
	for entrada, esperado := range casos {
		if got := Normalizar(entrada); got != esperado {
			t.Fatalf("Normalizar(%q) = %q, esperado %q", entrada, got, esperado)
		}
	}
}

func TestRegistrable(t *testing.T) {
	casos := map[string]string{
		"ejemplo.com":       "ejemplo.com",
		"www.ejemplo.com":   "ejemplo.com",
		"a.b.ejemplo.co.uk": "ejemplo.co.uk",
		"ejemplo.com.ar":    "ejemplo.com.ar",
		"sub.ejemplo.org":   "ejemplo.org",
		"localhost":         "",
		"192.168.1.1":       "",
		"nas.local":         "nas.local", // .local no está en la lista, pero es un TLD de un solo nivel
		"servidor-interno":  "",
		"":                  "",
	}
	for host, esperado := range casos {
		if got := Registrable(host); got != esperado {
			t.Fatalf("Registrable(%q) = %q, esperado %q", host, got, esperado)
		}
	}
}

func TestLookupEntradaInvalida(t *testing.T) {
	casos := []string{"", "   ", "192.168.1.1", "localhost"}
	for _, entrada := range casos {
		if _, err := Lookup(context.Background(), entrada); err == nil {
			t.Fatalf("Lookup(%q) debería fallar", entrada)
		}
	}
}
