// Package ping ofrece pings de red ejecutados desde el servidor:
// TCP (latencia de conexión, sin privilegios) e ICMP echo (requiere
// permisos de socket o net.ipv4.ping_group_range habilitado).
package ping

import (
	"context"
	"net"
	"os"
	"strconv"
	"time"

	"golang.org/x/net/icmp"
	"golang.org/x/net/ipv4"
)

// Protocolos soportados.
const (
	ProtoTCP  = "tcp"
	ProtoICMP = "icmp"
)

// Config describe una sesión de ping.
type Config struct {
	Host     string        // nombre o IP
	Proto    string        // ProtoTCP (default) o ProtoICMP
	Port     int           // solo TCP
	Interval time.Duration // entre paquetes
	Count    int           // 0 = hasta que se cancele el contexto
}

// Result es el resultado de un paquete.
type Result struct {
	Seq       int
	OK        bool
	LatencyMS int
	Error     string
	Fatal     bool // el error es permanente (p. ej. sin permisos): detiene la sesión
}

// Run ejecuta la sesión hasta agotar Count o hasta que ctx se cancele.
// Cada resultado se entrega a send desde la misma goroutine (ordenado).
func (c Config) Run(ctx context.Context, send func(Result)) {
	if c.Interval <= 0 {
		c.Interval = time.Second
	}
	if c.Port == 0 {
		c.Port = 443
	}
	for seq := 1; ; seq++ {
		if c.Count > 0 && seq > c.Count {
			return
		}
		res := c.ping(ctx, seq)
		send(res)
		if res.Fatal {
			return // el error es permanente: no tiene sentido seguir
		}
		select {
		case <-ctx.Done():
			return
		case <-time.After(c.Interval):
		}
	}
}

func (c Config) ping(ctx context.Context, seq int) Result {
	if c.Proto == ProtoICMP {
		return icmpPing(ctx, c.Host, seq)
	}
	return tcpPing(ctx, c.Host, c.Port, seq)
}

func tcpPing(ctx context.Context, host string, port, seq int) Result {
	addr := net.JoinHostPort(host, strconv.Itoa(port))
	d := net.Dialer{}
	start := time.Now()
	conn, err := d.DialContext(ctx, "tcp", addr)
	latency := int(time.Since(start).Milliseconds())
	if err != nil {
		return Result{Seq: seq, OK: false, LatencyMS: latency, Error: err.Error()}
	}
	_ = conn.Close()
	return Result{Seq: seq, OK: true, LatencyMS: latency}
}

// icmpPing envía un echo request usando el socket "udp4" (ping sockets de
// Linux), que permite ICMP sin socket crudo cuando el kernel lo permite.
func icmpPing(ctx context.Context, host string, seq int) Result {
	conn, err := icmp.ListenPacket("udp4", "0.0.0.0")
	if err != nil {
		return Result{Seq: seq, OK: false, Fatal: true,
			Error: "ICMP no disponible (permisos): " + err.Error() +
				". Usa el tipo TCP o habilita los permisos de ping en el servidor."}
	}
	defer conn.Close()

	ip, err := net.ResolveIPAddr("ip4", host)
	if err != nil {
		return Result{Seq: seq, OK: false, Error: "resolución DNS: " + err.Error()}
	}

	id := os.Getpid() & 0xffff
	msg := icmp.Message{
		Type: ipv4.ICMPTypeEcho, Code: 0,
		Body: &icmp.Echo{ID: id, Seq: seq},
	}
	data, err := msg.Marshal(nil) // IPv4: nil calcula el checksum automáticamente
	if err != nil {
		return Result{Seq: seq, OK: false, Error: err.Error()}
	}

	start := time.Now()
	if _, err := conn.WriteTo(data, ip); err != nil {
		return Result{Seq: seq, OK: false, Error: err.Error()}
	}

	deadline := 2 * time.Second
	if dl, ok := ctx.Deadline(); ok {
		if left := time.Until(dl); left < deadline {
			deadline = left
		}
	}
	if deadline <= 0 {
		return Result{Seq: seq, OK: false, Error: "contexto cancelado"}
	}
	_ = conn.SetReadDeadline(time.Now().Add(deadline))

	buf := make([]byte, 1500)
	for {
		n, peer, err := conn.ReadFrom(buf)
		if err != nil {
			return Result{Seq: seq, OK: false, Error: "sin respuesta (timeout)"}
		}
		if peer.String() != ip.String() {
			continue // respuesta de otro destino
		}
		rm, err := icmp.ParseMessage(1, buf[:n])
		if err != nil {
			continue
		}
		echo, ok := rm.Body.(*icmp.Echo)
		if rm.Type != ipv4.ICMPTypeEchoReply || !ok || echo.ID != id || echo.Seq != seq {
			continue
		}
		return Result{Seq: seq, OK: true, LatencyMS: int(time.Since(start).Milliseconds())}
	}
}
