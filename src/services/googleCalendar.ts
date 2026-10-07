export interface GoogleEventPayload {
  summary: string;
  description?: string;
  location?: string;
  start: {
    dateTime: string;
    timeZone: string;
  };
  end: {
    dateTime: string;
    timeZone: string;
  };
}

/**
 * Envia um evento para o Google Calendar usando o provider_token retornado pelo Supabase (Google OAuth).
 */
export async function syncEventToGoogleCalendar(providerToken: string, payload: GoogleEventPayload): Promise<boolean> {
  try {
    const response = await fetch('https://www.googleapis.com/calendar/v3/calendars/primary/events', {
      method: 'POST',
      headers: {
        'Authorization': `Bearer ${providerToken}`,
        'Content-Type': 'application/json'
      },
      body: JSON.stringify(payload)
    });

    if (!response.ok) {
      console.error('Falha ao sincronizar com Google Calendar:', await response.text());
      return false;
    }

    return true;
  } catch (error) {
    console.error('Erro na requisição para Google Calendar:', error);
    return false;
  }
}
