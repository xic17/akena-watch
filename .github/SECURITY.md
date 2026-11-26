# Política de seguridad

## Versiones soportadas

Solo la **última versión publicada** recibe correcciones de seguridad. Akena
Watch es un binario autocontenido: actualizar es descargar el binario nuevo (o
volver a ejecutar el instalador) y reiniciar el proceso.

| Versión | Soporte |
| --- | --- |
| 1.2.x | ✅ |
| anteriores | ❌ |

## Cómo reportar una vulnerabilidad

**No abras un issue público.** Usa el **reporte privado de vulnerabilidades** de
GitHub:

1. Abre la pestaña [Security](https://github.com/xic17/akena-watch/security).
2. Pulsa **Report a vulnerability**.
3. Cuéntanos el problema con el mayor detalle posible.

Conviene incluir:

- Versión de Akena Watch (`/api/version` o el pie de página) y sistema operativo.
- Pasos para reproducirlo (peticiones, comandos, capturas).
- El impacto que crees que tiene y, si lo hay, una prueba de concepto mínima.

## Compromiso de respuesta

- Acuse de recibo en un plazo orientativo de **3 días laborables**.
- Confirmación y evaluación del impacto.
- Corrección en una versión nueva, con su entrada en el `CHANGELOG` y el crédito
  correspondiente si lo deseas, una vez publicada.

## Alcance

Dentro del alcance, por ejemplo:

- Saltarse la autenticación o los permisos (leer o modificar monitores, canales o
  datos de otra persona).
- Inyección (SQL, comandos, XSS) o ejecución remota de código.
- Exposición de credenciales, tokens de canales o datos personales.
- Cualquier forma de eludir el límite de intentos de acceso.

Fuera del alcance:

- Las peticiones salientes a direcciones internas desde monitores y herramientas:
  **es parte del diseño** (un monitor puede vigilar tu propia red). Si encuentras
  la forma de usarlo **sin** una sesión válida, eso sí es un hallazgo.
- Denegación de servicio contra tu propia instancia.
- Resultados de escáneres automáticos sin explicación de impacto ni prueba de
  concepto.
- Ingeniería social.

## Buenas prácticas para quien lo despliega

- Ejecuta el binario con un **usuario de sistema sin privilegios** y escucha solo
  en `127.0.0.1` cuando lo pongas detrás de un proxy (ver el README).
- No expongas la instancia sin TLS ni con una contraseña débil: en el primer
  arranque se crea el administrador y, desde ahí, todo manda.
- Protege el directorio de datos: ahí viven la base de datos SQLite y las
  credenciales de los canales de alerta.
