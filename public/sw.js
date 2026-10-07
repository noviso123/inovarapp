// Cache somente o aplicativo e ativos estáticos. APIs e dados de clientes
// nunca entram no cache, mas o PWA continua abrindo sem sinal para consultar
// os dados já salvos localmente e reconectar depois.
const CACHE = 'inovar-static-v10';
const APP_SHELL = ['/', '/index.html', '/manifest.webmanifest', '/favicon.svg'];
self.addEventListener('install', event => {
  event.waitUntil(caches.open(CACHE).then(cache => cache.addAll(APP_SHELL)).catch(() => {}));
  self.skipWaiting();
});
self.addEventListener('activate', event => event.waitUntil((async () => {
  for (const key of await caches.keys()) if (key.startsWith('inovar-static-') && key !== CACHE) await caches.delete(key);
  await self.clients.claim();
})()));
self.addEventListener('fetch', event => {
  const url = new URL(event.request.url);
  if (event.request.method !== 'GET' || url.origin !== self.location.origin || url.pathname.startsWith('/api/')) return;
  if (event.request.mode === 'navigate') {
    event.respondWith((async () => {
      try {
        const response = await fetch(event.request);
        if (response.ok) {
          const cache = await caches.open(CACHE);
          await cache.put('/index.html', response.clone());
        }
        return response;
      } catch {
        return (await caches.match('/index.html')) || new Response('<!doctype html><html lang="pt-BR"><meta name="viewport" content="width=device-width,initial-scale=1"><meta charset="utf-8"><title>InovarApp — Sem conexão</title><body style="font:18px system-ui;background:#f8fafc;color:#0f172a;padding:32px"><h1>Você está sem conexão</h1><p>Reconecte para abrir seus atendimentos e salvar alterações com segurança.</p><a style="color:#0369a1" href="/">Tentar novamente</a></body></html>', { headers: { 'Content-Type': 'text/html; charset=utf-8' } });
      }
    })());
  } else if (url.pathname.startsWith('/assets/')) {
    event.respondWith((async () => {
      const cache = await caches.open(CACHE);
      const cached = await cache.match(event.request);
      if (cached) return cached;
      const response = await fetch(event.request);
      if (response.ok) await cache.put(event.request, response.clone());
      return response;
    })());
  }
});

self.addEventListener('push', event => {
  const data = event.data?.json?.() || {};
  const title = data.title || 'InovarApp';
  event.waitUntil(self.registration.showNotification(title, {
    body: data.body || 'Há uma atualização no seu atendimento.',
    icon: '/icon-192.png',
    badge: '/icon-192.png',
    data: { url: data.url || '/' },
    tag: data.tag || 'inovar-notification',
    renotify: true
  }));
});
self.addEventListener('notificationclick', event => {
  event.notification.close();
  event.waitUntil(clients.openWindow(event.notification.data?.url || '/'));
});
