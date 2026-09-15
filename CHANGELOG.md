# Changelog

Todos los cambios notables de **Akena Watch — Siempre en Guardia**.

El formato sigue [Keep a Changelog](https://keepachangelog.com/es/1.1.0/) y el
proyecto usa versionado [SemVer](https://semver.org/lang/es/).

## [Sin publicar]

### Seguridad

Auditoría completa del código y del historial antes de dar por publicada la
versión. Los arreglos no cambian la forma de usar Akena Watch.

- **Cookie de sesión `Secure` en HTTPS**: la cookie se marca `Secure` cuando la
  petición llega por HTTPS (conexión directa o cabecera `X-Forwarded-Proto` de un
  proxy inverso), para que no viaje por canales sin cifrar. En HTTP local (por
  ejemplo `http://127.0.0.1:8080`) se mantiene sin `Secure`, de modo que el
  acceso directo sigue funcionando.
- **La lista de usuarios ya no expone datos personales**: `GET /api/users`
  devolvía el correo y el ID de Telegram de todas las personas a cualquier
  usuario autenticado. Ahora la ficha completa (correo, Telegram, grupos y
  accesos) la recibe solo un administrador; el resto obtiene únicamente id,
  nombre de usuario y rol —lo necesario para compartir monitores—.
- **Se descartan los canales de alerta ajenos**: al crear o editar un monitor
  solo se asocian canales que pertenecen al usuario que edita o al propietario
  del monitor. Antes, un colaborador con permiso de edición podía asociar el bot
  de Telegram o el webhook de otra persona y hacer que sus propias alertas
  salieran por la cuenta ajena.
- **`GET /api/groups` requiere rol de administrador**: antes, cualquier usuario
  autenticado podía enumerar los nombres de todos los grupos del sistema.
- **El instalador verifica el checksum de forma obligatoria**: `scripts/install.sh`
  usa `curl -fsSL` (una respuesta 404 ya no se instala como si fuera el binario)
  y aborta si no puede descargar o validar `SHA256SUMS`.

### Corregido

- **CI de nuevo en verde**: tres archivos (`internal/store/monitors.go`,
  `internal/server/handlers_monitors.go` y
  `internal/server/handlers_httpcheck.go`) incumplían `gofmt`, así que el flujo
  de CI estaba en rojo. Solo cambia el formato: no hay cambios de
  comportamiento.
- **El instalador se adjunta a cada release**: el comando de una línea del
  README descargaba `install.sh` desde los assets de la release, pero el flujo
  de publicación no lo incluía (devolvía 404).

## [1.1.0] — 2026-09-15

### Añadido

#### Herramientas

- **Inspección HTTP** (nueva pestaña): lanza una petición desde el servidor, como
  `curl`, y muestra tiempos desglosados (DNS, conexión, TLS, TTFB y total), la
  cadena de redirecciones, las cabeceras de respuesta, el certificado TLS y una
  vista previa del cuerpo. Permite elegir método, cabeceras adicionales y cuerpo
  JSON para probar APIs.
- **Certificado TLS** (nueva pestaña): inspecciona el certificado de un servidor
  —validez, emisor, SANs, número de serie, algoritmos de firma y clave, cadena y
  días restantes— con puerto configurable.
- **Escaneo de puertos** (nueva pestaña): comprueba qué puertos TCP aceptan
  conexión, con perfiles predefinidos (comunes, web, bases de datos) o rango
  personalizado, mostrando el servicio asociado y la latencia de cada puerto.

#### Monitores

- **Umbral de lentitud**: además de «arriba/caído», los monitores detectan
  degradación. Se configura un umbral de latencia y cuántos checks consecutivos
  deben superarlo para avisar. El monitor muestra el estado «lento» (indicador
  ámbar en el dashboard y en la página de estado) y envía una alerta única que se
  rearma al normalizarse.
- **Aviso de expiración de certificado**: en monitores HTTP(S) se puede avisar
  cuando al certificado le quedan menos días que el umbral configurado. El
  certificado se comprueba una vez al día (señal de evolución lenta) y la alerta
  🟠 se envía una sola vez por ciclo, con mensajes específicos si expira hoy o si
  ya ha expirado.
- **Ventana de mantenimiento semanal**: define día y franja horaria en la que no
  se envían alertas, con soporte de franjas que cruzan la medianoche. Los checks
  y el historial se mantienen intactos —el uptime sigue siendo honesto— y si el
  monitor sigue caído al terminar la ventana, la alerta se dispara en el
  siguiente check.
- **Duplicar monitor**: clona la configuración completa (incluidos canales) con
  el sufijo «(copia)». Las comparticiones no se copian: el clon pertenece al
  mismo propietario.

#### Desarrollo

- `scripts/dev.sh` (y `make dev`): arranque en modo desarrollo con datos de
  prueba. Recompila, reinicia la instancia anterior y escucha solo en
  `127.0.0.1`. El directorio de datos se toma de `AKENA_TEST_DATA_DIR`, de
  `TEST_DATA_DIR` en `Makefile.local` (config personal gitignoreada) o de
  `./.test-data`.

#### Interfaz

- Enlace al sitio web oficial (`akenawatch.com`) en la página «Acerca de».

### Cambiado

- El modal de crear/editar monitor es más ancho (720 px) para que el formulario
  de configuración no quede comprimido.
- El formulario de Inspección HTTP se muestra apilado a ancho completo en lugar
  de repartido en columnas.
- En «Certificado TLS» los textos (emisor, protocolo, cifrado) pasan a la tabla
  de detalles; las tarjetas grandes quedan reservadas a valores numéricos.
- Los formularios de herramientas que contienen listas y cuerpos de texto ya no
  heredan la rejilla compacta de los formularios cortos.

### Corregido

- **Ventana de mantenimiento**: el cálculo del cruce de medianoche usaba el día
  siguiente en vez del anterior, por lo que la franja posterior a medianoche no
  se reconocía.
- **Inspección HTTP**: las URLs y los valores de cabeceras largos desbordaban el
  margen derecho del panel; ahora se parten en varias líneas.
- **Inspección HTTP**: márgenes inconsistentes entre las secciones del resultado
  (cabeceras y cuerpo).

### Notas de actualización

Las columnas nuevas (`latency_threshold_ms`, `slow_retries`, `cert_alert_days`,
`maint_enabled`, `maint_weekday`, `maint_start`, `maint_end`) se añaden
automáticamente a las bases de datos existentes al arrancar esta versión. No hay
migración manual ni pérdida de datos: monitores, historial, usuarios y canales
se conservan.

## [1.0.7] — 2026-08-27

### Corregido

- **Ping ICMP (herramienta)**: en Linux el kernel reescribe el identificador del
  `echo request` con el puerto local del socket (`inet_num`) en los ping sockets,
  de modo que la respuesta llegaba con otro identificador y se descartaba,
  mostrando «sin respuesta». Ahora se espera el identificador correcto según el
  tipo de socket (`udp4` o crudo).

## [1.0.6] — 2026-08-27

### Añadido

- **ICMP sin configuración manual**: `scripts/install.sh` concede `cap_net_raw`
  al binario cuando se ejecuta con permisos de root y el servicio systemd
  incluido declara `AmbientCapabilities=CAP_NET_RAW`.

### Cambiado

- El motor de ping intenta el socket crudo (`ip4:icmp`) cuando el kernel deniega
  el ping socket sin privilegios (`udp4`), lo que habilita ICMP también en
  entornos que corren como root (Docker, Cloudflare Containers).

## [1.0.5] — 2026-08-27

### Corregido

- **Ping ICMP (herramienta)**: se enviaba a un `*net.IPAddr` cuando el socket
  `udp4` exige `*net.UDPAddr`, lo que producía `write udp …: invalid argument`.
  Además, la comparación del origen de la respuesta se hace únicamente por IP.

## [1.0.4] — 2026-08-26

### Añadido

- **Pausar y reanudar monitores**: nuevo endpoint
  `PUT /api/monitors/{id}/active`, icono de pausa/reproducción en el listado,
  fila atenuada con la marca «pausado» y el mismo estado en la página pública.
  Un monitor pausado no se comprueba ni genera alertas.

## [1.0.3] — 2026-08-26

### Corregido

- Los botones implementados como enlace (`<a class="btn">`) ya no se subrayan al
  pasar el ratón por encima.

## [1.0.2] — 2026-08-26

### Corregido

- **Botón «Perfil»**: fallaba en las páginas de Herramientas y «Acerca de»
  porque el contenedor de modales (`#modal-root`) no existía en esas plantillas.
  Ahora `openModal()` crea el contenedor si falta, de modo que funciona en
  cualquier página.

### Cambiado

- Los estáticos (`app.js`, `app.css`) se sirven con la versión en la URL para
  evitar que el navegador use copias en caché tras una actualización.

## [1.0.1] — 2026-08-26

### Corregido

- **Inicio de sesión**: el formulario enviaba `email` y `telegram_id` en
  cualquier formulario de autenticación y el servidor rechazaba campos
  desconocidos («solicitud inválida»). Ahora solo se envían los campos presentes
  en cada formulario.

## [1.0.0] — 2026-08-25

### Añadido

- Primera versión pública: monitor de disponibilidad en **un solo binario
  estático** (Go, SQLite embebida y frontend embebido, sin dependencias de
  runtime).
- Asistente de primer arranque para crear la cuenta administradora.
- Multi-usuario con roles (administrador y colaborador), grupos de monitores y
  accesos por grupo o mediante asignación manual.
- Monitores **HTTP** (método, cabeceras, cuerpo JSON, estado esperado y palabra
  clave, con opción de invertirla), **TCP** y **DNS**, con intervalo, timeout,
  reintentos y agrupación.
- Canales de alerta: **webhook** (cuerpo JSON personalizable con variables),
  **Telegram** y **SMTP**, con notificación de caída y de recuperación, más
  aviso directo al propietario por su Telegram.
- Página de estado pública por usuario, con título, descripción y refresco
  automático.
- Herramientas: **ping en tiempo real** por WebSocket (TCP e ICMP), **Whois** y
  **DNS Lookup**.
- Dashboard con resumen estadístico, sparklines de latencia, uptime de 24 h/7
  días/30 días y percentil 95 por monitor.
- Despliegue documentado en Linux (systemd), **CloudPanel 2** (reverse proxy),
  **Cloudflare Containers** y Docker.

[Sin publicar]: https://github.com/xic17/akena-watch/compare/v1.1.0...HEAD
[1.1.0]: https://github.com/xic17/akena-watch/compare/v1.0.7...v1.1.0
[1.0.7]: https://github.com/xic17/akena-watch/compare/v1.0.6...v1.0.7
[1.0.6]: https://github.com/xic17/akena-watch/compare/v1.0.5...v1.0.6
[1.0.5]: https://github.com/xic17/akena-watch/compare/v1.0.4...v1.0.5
[1.0.4]: https://github.com/xic17/akena-watch/compare/v1.0.3...v1.0.4
[1.0.3]: https://github.com/xic17/akena-watch/compare/v1.0.2...v1.0.3
[1.0.2]: https://github.com/xic17/akena-watch/compare/v1.0.1...v1.0.2
[1.0.1]: https://github.com/xic17/akena-watch/compare/v1.0.0...v1.0.1
[1.0.0]: https://github.com/xic17/akena-watch/releases/tag/v1.0.0
