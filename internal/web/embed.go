// Package web embebe las plantillas y los archivos estáticos dentro
// del binario: no hay que copiar nada, el ejecutable es autónomo.
package web

import "embed"

//go:embed templates static
var FS embed.FS
