import { Client, Appliance, MaintenanceRecord, TechnicianProfile } from '../types';
import { differenceInMonths, parseISO, format } from 'date-fns';
import { ptBR } from 'date-fns/locale';

export const WhatsAppService = {
  // Cleans phone number to international 55DDXXXXXXXXX format
  cleanPhone(phone: string): string {
    const digits = phone.replace(/\D/g, '');
    if (digits.length === 10 || digits.length === 11) {
      return `55${digits}`;
    }
    return digits;
  },

  // Generates persuasive message based on status
  generateReturnMessage(
    client: Client,
    appliance: Appliance,
    lastMaintenance: MaintenanceRecord | undefined,
    profile: TechnicianProfile
  ): string {
    const clientFirstName = client.name.split(' ')[0];
    const techName = profile.name || 'seu técnico de ar-condicionado';
    const appDesc = `${appliance.type} ${appliance.capacityBtu} BTUs (${appliance.room})`;

    let monthsAgo = 6;
    let lastDateFormatted = '';
    if (lastMaintenance?.date) {
      try {
        const lastDate = parseISO(lastMaintenance.date);
        monthsAgo = Math.max(1, differenceInMonths(new Date(), lastDate));
        lastDateFormatted = format(lastDate, "dd 'de' MMMM 'de' yyyy", { locale: ptBR });
      } catch {
        monthsAgo = 6;
      }
    }

    return `Olá, *${clientFirstName}*! Tudo bem? 😊

Aqui é o *${techName}*. 

Estou entrando em contato pois já faz aproximadamente *${monthsAgo} meses* desde a última limpeza de ar preventiva do seu ar-condicionado (*${appDesc}*)${lastDateFormatted ? ` realizada em ${lastDateFormatted}` : ''}.

Com o uso contínuo, poeira, ácaros e fungos se acumulam na serpentina e turbina, o que pode:
⚠️ Aumentar o consumo de energia em até 30%
⚠️ Reduzir a potência de refrigeração
⚠️ Causar alergias e irritação respiratória

Para manter o ar do seu ambiente 100% puro e o aparelho funcionando como novo, vamos agendar a revisão preventiva para esta semana?

Qual dia fica melhor para você: *pela manhã* ou *à tarde*? ❄️🔧`;
  },

  // WhatsApp link generator
  getWhatsAppUrl(phone: string, message: string): string {
    const cleaned = this.cleanPhone(phone);
    const encoded = encodeURIComponent(message);
    return `https://api.whatsapp.com/send?phone=${cleaned}&text=${encoded}`;
  },

  // Open WhatsApp in new tab/app
  openWhatsApp(phone: string, message: string): void {
    const url = this.getWhatsAppUrl(phone, message);
    window.open(url, '_blank');
  },

  // Message for sending Service Order & Warranty receipt
  generateReceiptMessage(
    client: Client,
    appliance: Appliance,
    record: MaintenanceRecord,
    profile: TechnicianProfile
  ): string {
    const clientFirstName = client.name.split(' ')[0];
    const techName = profile.name;
    const warrantyExpiry = record.warrantyDays
      ? `Garantia de ${record.warrantyDays} dias inclusa.`
      : '';
    // datas blindadas: OS agendada/em andamento não tem retorno (string vazia)
    const fmtData = (v?: string | null) => {
      try {
        if (!v) return '—';
        const d = parseISO(String(v));
        return isNaN(d.getTime()) ? '—' : format(d, 'dd/MM/yyyy');
      } catch { return '—'; }
    };

    return `Olá *${clientFirstName}*! Segue o comprovante do serviço realizado:

❄️ *ORDEM DE SERVIÇO & COMPROVANTE*
👤 *Cliente:* ${client.name}
📍 *Local:* ${appliance.room} (${appliance.brand} ${appliance.capacityBtu} BTUs)
🔧 *Serviço:* ${record.serviceType}
📅 *Data:* ${fmtData(record.date)}
💰 *Valor:* R$ ${Number(record.price || 0).toFixed(2)} (${record.paymentMethod})
${profile.pixKey ? `🔑 *Chave PIX:* ${profile.pixKey} (${profile.pixType.toUpperCase()})\n` : ''}
🛡️ ${warrantyExpiry}
🔄 *Próximo retorno recomendado:* ${fmtData(record.returnDate)}

Muito obrigado pela confiança! Qualquer dúvida estou à disposição.
*${techName}* - ${profile.businessName}`;
  }
};
