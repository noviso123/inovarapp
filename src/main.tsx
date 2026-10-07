import React from 'react';
import ReactDOM from 'react-dom/client';
import App from './App';
import './index.css';
import { preloadPdfAssets } from './services/brandAssets';
import { emitirNotificacao } from './services/appNotifications';

// Incrementado a cada publicação funcional. Quem já tem o PWA recebe um
// aviso no sino com instruções para atualizar/reabrir a versão nova.
const APP_VERSION = '2026.10.07.4';

ReactDOM.createRoot(document.getElementById('root') as HTMLElement).render(
  <React.StrictMode>
    <App />
  </React.StrictMode>
);

try {
  const anterior = localStorage.getItem('inovarapp-version');
  localStorage.setItem('inovarapp-version', APP_VERSION);
  if (anterior && anterior !== APP_VERSION) {
    window.setTimeout(() => emitirNotificacao('Nova versão disponível', 'O InovarApp foi atualizado. Reabra o aplicativo se algum recurso ainda estiver em tela antiga.'), 900);
  }
} catch { /* armazenamento indisponível não impede o aplicativo */ }

// PDFs são usados após a navegação inicial. Carregar os recursos em segundo
// plano evita espera perceptível quando o usuário toca em Gerar/Enviar.
const warmDocuments = () => {
  void Promise.all([preloadPdfAssets(), import('jspdf')]);
};
if ('requestIdleCallback' in window) {
  (window as Window & { requestIdleCallback: (cb: () => void, options?: { timeout: number }) => number })
    .requestIdleCallback(warmDocuments, { timeout: 2500 });
} else {
  globalThis.setTimeout(warmDocuments, 900);
}

if (import.meta.env.PROD && 'serviceWorker' in navigator) {
  window.addEventListener('load', () => {
    navigator.serviceWorker.register('/sw.js')
      .then((registration) => registration.update())
      .catch(() => {});
  });
}
