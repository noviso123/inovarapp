// Foto de perfil (cliente e técnico) — imagem comprimida no dispositivo,
// guardada no storage privado em fotos-perfil/{userId}.jpg via /api/documentos.
import { supabase } from './supabase';

const MAX_LADO = 320;

export function comprimirFotoPerfil(file: File): Promise<string> {
  return new Promise((resolve, reject) => {
    const fr = new FileReader();
    fr.onerror = () => reject(new Error('Falha ao ler a imagem'));
    fr.onload = () => {
      const bruto = fr.result as string;
      const img = new Image();
      img.onerror = () => reject(new Error('Formato de imagem não suportado'));
      img.onload = () => {
        try {
          const escala = Math.min(1, MAX_LADO / Math.max(img.width, img.height));
          const canvas = document.createElement('canvas');
          canvas.width = Math.round(img.width * escala);
          canvas.height = Math.round(img.height * escala);
          const ctx = canvas.getContext('2d');
          if (!ctx) return resolve(bruto);
          ctx.drawImage(img, 0, 0, canvas.width, canvas.height);
          resolve(canvas.toDataURL('image/jpeg', 0.85));
        } catch {
          resolve(bruto);
        }
      };
      img.src = bruto;
    };
    fr.readAsDataURL(file);
  });
}

async function authHeader(): Promise<Record<string, string>> {
  const { data: sessionData } = await supabase.auth.getSession();
  return {
    'Content-Type': 'application/json',
    Authorization: `Bearer ${sessionData?.session?.access_token || ''}`
  };
}

export async function subirFotoPerfil(base64: string): Promise<{ ok: boolean; erro?: string }> {
  try {
    const r = await fetch('/api/documentos', {
      method: 'POST',
      headers: await authHeader(),
      body: JSON.stringify({ acao: 'fotoperfil', base64 })
    });
    const j = await r.json().catch(() => ({}));
    if (!r.ok) return { ok: false, erro: j.error || 'Falha ao enviar a foto.' };
  return { ok: true };
  } catch {
    return { ok: false, erro: 'Sem conexão para enviar a foto.' };
  }
}

export async function urlFotoPerfil(): Promise<string | null> {
  try {
    const r = await fetch('/api/documentos', {
      method: 'POST',
      headers: await authHeader(),
      body: JSON.stringify({ acao: 'fotoperfil-url' })
    });
    const j = await r.json().catch(() => ({}));
    // A mesma chave é sobrescrita ao trocar a foto. O sufixo evita que o
    // navegador/CDN mostre a miniatura anterior após uma atualização.
    const url = j?.url ? String(j.url) : '';
    return url ? `${url}${url.includes('?') ? '&' : '?'}v=${Date.now()}` : null;
  } catch {
    return null;
  }
}

export async function removerFotoPerfil(): Promise<void> {
  try {
    await fetch('/api/documentos', {
      method: 'POST',
      headers: await authHeader(),
      body: JSON.stringify({ acao: 'fotoperfil-remover' })
    });
  } catch { /* best-effort */ }
}
