// Convite de calendário (.ics) — ao agendar, o arquivo é gerado e o celular
// oferece salvar na agenda (Android e iPhone salvam no app de calendário nativo).

export interface EventoCalendario {
  id?: string;
  titulo: string;
  descricao?: string;
  local?: string;
  data: string; // YYYY-MM-DD
  hora?: string; // HH:MM
  duracaoMin?: number;
}

function fimEvento(data: string, hora: string | undefined, duracaoMin: number): string {
  const d = data.replace(/-/g, '');
  const h = hora || '09:00';
  const dt = new Date(`${data}T${h}:00`);
  dt.setMinutes(dt.getMinutes() + duracaoMin);
  const hh = String(dt.getHours()).padStart(2, '0');
  const mm = String(dt.getMinutes()).padStart(2, '0');
  return `${d}T${hh}${mm}00`;
}

export function baixarConviteICS(ev: EventoCalendario): boolean {
  try {
    const inicio = `${ev.data.replace(/-/g, '')}T${(ev.hora || '09:00').replace(':', '')}00`;
    const fim = fimEvento(ev.data, ev.hora, ev.duracaoMin ?? 120);
    const stamp = new Date().toISOString().replace(/[-:]/g, '').split('.')[0] + 'Z';
    const esc = (s: string) => (s || '').replace(/\\/g, '\\\\').replace(/;/g, '\\;').replace(/,/g, '\\,').replace(/\n/g, '\\n');

    const ics = [
      'BEGIN:VCALENDAR',
      'VERSION:2.0',
      'PRODID:-//InovarApp//PT-BR',
      'CALSCALE:GREGORIAN',
      'METHOD:PUBLISH',
      'BEGIN:VEVENT',
      `UID:${ev.id || `${Date.now()}-${Math.random().toString(36).slice(2)}`}@inovarapp`,
      `DTSTAMP:${stamp}`,
      `DTSTART:${inicio}`,
      `DTEND:${fim}`,
      `SUMMARY:${esc(ev.titulo)}`,
      `DESCRIPTION:${esc(ev.descricao || '')}`,
      `LOCATION:${esc(ev.local || '')}`,
      'BEGIN:VALARM',
      'TRIGGER:-PT1H',
      'ACTION:DISPLAY',
      `DESCRIPTION:${esc(ev.titulo)}`,
      'END:VALARM',
      'END:VEVENT',
      'END:VCALENDAR'
    ].join('\r\n');

    const blob = new Blob([ics], { type: 'text/calendar;charset=utf-8' });
    const url = URL.createObjectURL(blob);
    const a = document.createElement('a');
    a.href = url;
    a.download = 'agendamento-inovar.ics';
    document.body.appendChild(a);
    a.click();
    document.body.removeChild(a);
    setTimeout(() => URL.revokeObjectURL(url), 3000);
    return true;
  } catch {
    return false;
  }
}

// Alternativa universal: abre o Google Agenda com o evento preenchido
export function linkGoogleAgenda(ev: EventoCalendario): string {
  const fmt = (d: string, h?: string) => `${d.replace(/-/g, '')}T${(h || '0900').replace(':', '')}00`;
  const dt = new Date(`${ev.data}T${ev.hora || '09:00'}:00`);
  dt.setMinutes(dt.getMinutes() + (ev.duracaoMin ?? 120));
  const fh = `${String(dt.getFullYear())}${String(dt.getMonth() + 1).padStart(2, '0')}${String(dt.getDate()).padStart(2, '0')}T${String(dt.getHours()).padStart(2, '0')}${String(dt.getMinutes()).padStart(2, '0')}00`;
  const p = new URLSearchParams({
    action: 'TEMPLATE',
    text: ev.titulo,
    dates: `${fmt(ev.data, ev.hora)}/${fh}`,
    details: ev.descricao || '',
    location: ev.local || ''
  });
  return 'https://calendar.google.com/calendar/render?' + p.toString();
}

// Alternativa para Outlook/Office 365; o mesmo evento também pode ser importado pelo arquivo .ics.
export function linkOutlookAgenda(ev: EventoCalendario): string {
  const inicio = new Date(`${ev.data}T${ev.hora || '09:00'}:00`);
  const fim = new Date(inicio.getTime() + (ev.duracaoMin ?? 120) * 60000);
  const p = new URLSearchParams({
    path: '/calendar/action/compose',
    rru: 'addevent',
    subject: ev.titulo,
    startdt: inicio.toISOString(),
    enddt: fim.toISOString(),
    body: ev.descricao || '',
    location: ev.local || ''
  });
  return 'https://outlook.live.com/calendar/0/deeplink/compose?' + p.toString();
}
