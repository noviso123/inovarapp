import { supabase } from './supabase';

function toBytes(value: string) {
  const padding = '='.repeat((4 - value.length % 4) % 4);
  const base64 = (value + padding).replace(/-/g, '+').replace(/_/g, '/');
  const raw = atob(base64);
  return Uint8Array.from(raw, c => c.charCodeAt(0));
}

export function pushDisponivel() {
  return 'serviceWorker' in navigator && 'PushManager' in window && 'Notification' in window && !!import.meta.env.VITE_VAPID_PUBLIC_KEY;
}

export async function ativarNotificacoesPush() {
  if (!pushDisponivel()) throw new Error('Este navegador ainda não oferece notificações para o aplicativo.');
  const permission = await Notification.requestPermission();
  if (permission !== 'granted') throw new Error('Permissão de notificações não concedida. Você pode ativá-la nas configurações do navegador.');
  const registration = await navigator.serviceWorker.ready;
  const subscription = await registration.pushManager.subscribe({ userVisibleOnly: true, applicationServerKey: toBytes(import.meta.env.VITE_VAPID_PUBLIC_KEY) });
  const { data } = await supabase.auth.getSession();
  if (!data.session) throw new Error('Entre na sua conta antes de ativar notificações neste dispositivo.');
  const r = await fetch('/api/notificacoes', {
    method: 'POST',
    headers: { Authorization: `Bearer ${data.session.access_token}`, 'Content-Type': 'application/json' },
    body: JSON.stringify({ acao: 'inscrever', subscription: subscription.toJSON() })
  });
  const result = await r.json().catch(() => ({}));
  if (!r.ok) throw new Error(result.error || 'Não foi possível registrar este dispositivo.');
  return subscription;
}

export async function enviarNotificacaoNosDispositivos(titulo: string, texto: string, url = '/') {
  const { data } = await supabase.auth.getSession();
  if (!data.session) return false;
  const r = await fetch('/api/notificacoes', {
    method: 'POST',
    headers: { Authorization: `Bearer ${data.session.access_token}`, 'Content-Type': 'application/json' },
    body: JSON.stringify({ acao: 'enviar', titulo, texto, url })
  });
  return r.ok;
}
