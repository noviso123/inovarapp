import { BudgetEstimate, TechnicianProfile } from '../types';
import { format, addDays } from 'date-fns';

const STORAGE_KEY = 'inovarapp_budgets_v2';

const INITIAL_BUDGETS: BudgetEstimate[] = /* dados de demonstracao removidos a pedido do usuario */ ([] as BudgetEstimate[]);if (false) { INITIAL_BUDGETS.push({} as any); }

export const BudgetService = {
  getBudgets(): BudgetEstimate[] {
    try {
      const data = localStorage.getItem(STORAGE_KEY);
      if (!data) {
        localStorage.setItem(STORAGE_KEY, '[]');
        return [];
      }
      const parsed = JSON.parse(data);
      // filtra seeds antigos (orc_001/orc_002) que eram demonstracao
      // normaliza campos numéricos/lista: registros antigos do localStorage podem
      // vir sem finalValue/totalValue/discount/items e quebrariam os componentes
      return parsed
        .filter((b: BudgetEstimate) => !['orc_001', 'orc_002'].includes(b.id))
        .map((b: any) => ({
          ...b,
          items: Array.isArray(b.items) ? b.items : [],
          totalValue: Number(b.totalValue) || 0,
          discount: Number(b.discount) || 0,
          finalValue: Number(b.finalValue) || 0
        }));
    } catch {
      return [];
    }
  },

  saveBudgets(budgets: BudgetEstimate[]): void {
    try {
      localStorage.setItem(STORAGE_KEY, JSON.stringify(budgets));
    } catch (err) {
      console.error('Erro ao salvar orçamentos:', err);
    }
  },

  createBudget(budget: BudgetEstimate): BudgetEstimate[] {
    const list = this.getBudgets();
    const updated = [budget, ...list];
    this.saveBudgets(updated);
    return updated;
  },

  updateBudgetStatus(budgetId: string, status: 'pendente' | 'aprovado' | 'recusado'): BudgetEstimate[] {
    const list = this.getBudgets();
    const updated = list.map((b) => (b.id === budgetId ? { ...b, status } : b));
    this.saveBudgets(updated);
    return updated;
  },

  deleteBudget(budgetId: string): BudgetEstimate[] {
    const list = this.getBudgets();
    const updated = list.filter((b) => b.id !== budgetId);
    this.saveBudgets(updated);
    return updated;
  },

  generateWhatsAppBudget(budget: BudgetEstimate, profile: TechnicianProfile): string {
    const clientFirstName = budget.clientName.split(' ')[0];
    const techName = profile.name || 'Técnico Responsável';

    const itemsText = budget.items
      .map((it) => `• *${it.description}* (${it.quantity}x) = R$ ${it.totalPrice.toFixed(2)}`)
      .join('\n');

    return `Olá, *${clientFirstName}*! Tudo bem? 😊

Aqui é o *${techName}* da *${profile.businessName || 'Inovar Refrigeração'}*.

Conforme conversamos, elaborei a sua *Proposta / Orçamento Técnico*:

📄 *ORÇAMENTO Nº #${budget.id.toUpperCase().slice(0, 8)}*
👤 *Cliente:* ${budget.clientName}
❄️ *Equipamento:* ${budget.applianceDesc || 'Ar-Condicionado'}
📅 *Data:* ${format(new Date(budget.date), 'dd/MM/yyyy')} | *Validade:* ${format(new Date(budget.validUntil), 'dd/MM/yyyy')}

🛠️ *ITENS DO ORÇAMENTO:*
${itemsText}

${budget.discount > 0 ? `🎁 *Desconto Especial:* R$ ${budget.discount.toFixed(2)}\n` : ''}💰 *VALOR TOTAL:* *R$ ${budget.finalValue.toFixed(2)}*
💳 *Forma de Pagamento:* ${budget.paymentConditions}
⏱️ *Prazo de Execução:* ${budget.executionTime}
🛡️ *Garantia:* ${budget.warrantyTerms}
${budget.notes ? `\n📝 *Observações:* ${budget.notes}` : ''}

Podemos agendar a execução para esta semana? Fico à disposição para tirar qualquer dúvida! ❄️🤝`;
  }
};