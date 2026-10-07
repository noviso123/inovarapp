/** Eventos de notificação compartilhados por modais e telas.
 * O App registra no sino, mostra a notificação local e replica para os outros
 * dispositivos inscritos da mesma conta. */
export function emitirNotificacao(titulo: string, texto: string, url = '/') {
  window.dispatchEvent(new CustomEvent('inovar:notificacao', { detail: { titulo, texto, url } }));
}
