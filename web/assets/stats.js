// Estadísticas del repositorio en vivo desde la API pública de GitHub.
//
// El HTML trae cifras horneadas (por si falla la red, el límite de peticiones o
// el visitante no ejecuta JS). Aquí se refrescan y se guardan 10 minutos en
// localStorage: la API sin autenticar permite 60 peticiones por hora y por IP, y
// así un visitante no gasta más de una cada 10 minutos.
(function () {
  "use strict";

  var REPO = "xic17/akena-watch";
  var CLAVE = "akena_gh_stats";
  var TTL = 10 * 60 * 1000;

  function pintar(datos) {
    if (!datos) return;
    var valores = {
      stars: datos.stargazers_count,
      forks: datos.forks_count,
      watchers: datos.subscribers_count,
    };
    for (var clave in valores) {
      if (typeof valores[clave] !== "number") continue;
      var el = document.querySelector('[data-gh="' + clave + '"]');
      if (!el) continue;
      el.textContent = new Intl.NumberFormat("es").format(valores[clave]);
    }
  }

  function leerCache() {
    try {
      var guardado = JSON.parse(localStorage.getItem(CLAVE) || "null");
      if (guardado && guardado.datos && Date.now() - guardado.ts < TTL) {
        return guardado.datos;
      }
    } catch (e) {
      /* sin localStorage o datos corruptos: se ignora */
    }
    return null;
  }

  function guardarCache(datos) {
    try {
      localStorage.setItem(CLAVE, JSON.stringify({ ts: Date.now(), datos: datos }));
    } catch (e) {
      /* modo privado: se ignora */
    }
  }

  var cache = leerCache();
  if (cache) {
    pintar(cache); // reciente: no gasta una petición
    return;
  }

  fetch("https://api.github.com/repos/" + REPO, {
    headers: { Accept: "application/vnd.github+json" },
  })
    .then(function (res) {
      return res.ok ? res.json() : null;
    })
    .then(function (datos) {
      if (!datos) return;
      pintar(datos);
      guardarCache(datos);
    })
    .catch(function () {
      /* se quedan las cifras horneadas del HTML */
    });
})();
