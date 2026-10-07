import { supabase } from './supabase';

export async function googleCalendarRequest(action?: string, extra = {}) {
  const { data } = await supabase.auth.getSession();
  if (!data.session) throw new Error('Entre na sua conta para continuar.');
  const r = await fetch('/api/google-calendar', {
    method: action ? 'POST' : 'GET',
    headers: { Authorization: `Bearer ${data.session.access_token}`, 'Content-Type': 'application/json' },
    body: action ? JSON.stringify({ action, ...extra }) : undefined
  });
  const result = await r.json();
  if (!r.ok) throw new Error(result.error || 'Não foi possível conectar a agenda.');
  return result;
}

export type GoogleAuthorization = 'calendar' | 'contacts';

/**
 * Solicita somente a permissão necessária para a ação escolhida. Assim o
 * login comum continua simples e o administrador só vê a autorização de
 * Agenda ou Contatos quando optar por usar aquele recurso.
 */
export async function authorizeGoogle(resource: GoogleAuthorization = 'calendar') {
  const { data, error } = await supabase.auth.getUser();
  if (error || !data.user) throw new Error('Entre novamente para conectar o Google.');
  const linked = data.user.identities?.some(i => i.provider === 'google');
  const isCalendar = resource === 'calendar';
  const scopes = isCalendar
    ? 'email profile https://www.googleapis.com/auth/calendar.events'
    : 'email profile https://www.googleapis.com/auth/contacts.readonly';
  const returnFlag = isCalendar ? 'googleCalendar=1' : 'googleContacts=1';
  const options = {
    scopes,
    redirectTo: window.location.origin + '/?' + returnFlag,
    queryParams: { access_type: 'offline', prompt: 'consent', login_hint: data.user.email || '' }
  };
  const result = linked
    ? await supabase.auth.signInWithOAuth({ provider: 'google', options })
    : await supabase.auth.linkIdentity({ provider: 'google', options });
  if (result.error) throw result.error;
}

let syncing = false;
export async function automaticGoogleSync() {
  if (syncing || !navigator.onLine) return { skipped: true };
  syncing = true;
  try {
    const { data } = await supabase.auth.getSession();
    if (!data.session) return;
    if (new URLSearchParams(window.location.search).get('googleCalendar') === '1') {
      if (!data.session.provider_token) throw new Error('O Google não retornou uma autorização. Conecte a agenda novamente em Minha conta.');
      await googleCalendarRequest('connect', { providerToken: data.session.provider_token, providerRefreshToken: data.session.provider_refresh_token });
      window.history.replaceState({}, '', window.location.pathname);
    }
    const result = await googleCalendarRequest('sync');
    window.dispatchEvent(new CustomEvent('inovar-calendar-status', { detail: { ...result, error: null } }));
    return result;
  } catch (error: any) {
    window.dispatchEvent(new CustomEvent('inovar-calendar-status', { detail: { error: error.message } }));
    throw error;
  } finally { syncing = false; }
}

/**
 * Um único comando para o botão da agenda: na primeira vez abre somente a
 * autorização oficial do Google; nas demais, força uma atualização imediata.
 * A sincronização recorrente continua automática enquanto o app estiver ativo.
 */
export async function connectOrSyncGoogleCalendar() {
  const connection = await googleCalendarRequest();
  if (!connection.connected) {
    await authorizeGoogle('calendar');
    return { authorizing: true };
  }
  return automaticGoogleSync();
}
