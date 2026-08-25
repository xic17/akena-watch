/**
 * Akena Watch — Worker de Cloudflare Containers.
 *
 * Este es el ÚNICO archivo específico de Cloudflare. El binario Go
 * es agnóstico de plataforma; este Worker solo enruta tráfico hacia
 * la instancia y la mantiene despierta con un cron de keep-alive.
 *
 * Deploy:
 *   cd deploy/cloudflare
 *   npx wrangler deploy
 */

import { Container, getContainer } from "@cloudflare/containers";

export class AkenaWatch extends Container {
	/** Puerto en el que escucha el binario Go dentro del contenedor. */
	defaultPort = 8080;
	/** Red de seguridad: si el cron fallara, la instancia duerme igual. */
	sleepAfter = "30m";
}

interface Env {
	AKENA_WATCH: DurableObjectNamespace<AkenaWatch>;
}

export default {
	async fetch(request: Request, env: Env): Promise<Response> {
		// ID fijo "main": todas las peticiones van a la MISMA instancia,
		// replicando el comportamiento de un servidor dedicado.
		return getContainer(env.AKENA_WATCH, "main").fetch(request);
	},

	// Keep-alive: con un ping por minuto el contenedor nunca duerme y su
	// scheduler interno de checks corre 24/7. Sin esto, los checks se
	// detendrían entre peticiones de usuarios.
	async scheduled(_controller: unknown, env: Env): Promise<void> {
		const instance = getContainer(env.AKENA_WATCH, "main");
		await instance.fetch(new Request("http://akena.local/ping", { method: "GET" }));
	},
};
