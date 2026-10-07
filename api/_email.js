// Envio de e-mail via Gmail SMTP (conta própria, sem domínio, sem custo).
// Usado pelo /api/email e pelo cron de alertas — mesmo canal em todo o sistema.
import nodemailer from 'nodemailer';
import { lerConfigServidor, emailConfigFrom } from './_config.js';

export const REMETENTE = (user) => `Inovar Refrigeração <${user}>`;

export async function enviarEmailAutomatico(para, assunto, html) {
  const cfg = emailConfigFrom(await lerConfigServidor());
  if (!cfg.gmailUser || !cfg.gmailPass) {
    return { enviado: false, motivo: 'gmail nao configurado' };
  }
  if (!para || !/.+@.+\..+/.test(para)) {
    return { enviado: false, motivo: 'e-mail invalido' };
  }
  try {
    const transporter = nodemailer.createTransport({
      service: 'gmail',
      auth: { user: cfg.gmailUser, pass: cfg.gmailPass }
    });
    const info = await transporter.sendMail({
      from: REMETENTE(cfg.gmailUser),
      to: para,
      subject: assunto,
      html
    });
    return { enviado: true, motivo: 'ok', id: info?.messageId || null };
  } catch (e) {
    return { enviado: false, motivo: String(e).slice(0, 140) };
  }
}

export async function gmailConfigurado() {
  const cfg = emailConfigFrom(await lerConfigServidor());
  return !!(cfg.gmailUser && cfg.gmailPass);
}
