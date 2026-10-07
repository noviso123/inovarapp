import { createClient } from '@supabase/supabase-js';
import { BudgetEstimate, BudgetItem } from '../types';

const SUPABASE_URL = import.meta.env.VITE_SUPABASE_URL || 'https://ycpswioserctavijhnre.supabase.co';
const SUPABASE_ANON_KEY = import.meta.env.VITE_SUPABASE_ANON_KEY || 'eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9.eyJpc3MiOiJzdXBhYmFzZSIsInJlZiI6InljcHN3aW9zZXJjdGF2aWpobnJlIiwicm9sZSI6ImFub24iLCJpYXQiOjE3ODgxMTExOTIsImV4cCI6MjEwMzY4NzE5Mn0.mcxQsa2COarpWCV0v_qdEDuyZiTU4qA1p5PV05X40H0';

export const supabase = createClient(SUPABASE_URL, SUPABASE_ANON_KEY);

export interface SupabaseProfile {
  id: string;
  nome: string;
  email: string;
  telefone?: string;
  tipo: 'CLIENTE' | 'TECNICO' | 'ADMIN';
}

export interface SupabaseCustomer {
  id: string;
  profile_id?: string;
  nome: string;
  cpf_cnpj?: string;
  whatsapp?: string;
  endereco?: string;
  numero?: string;
  complemento?: string;
  bairro?: string;
  cidade?: string;
  estado?: string;
  observacoes?: string;
  ativo: boolean;
}

export interface SupabaseAirConditioner {
  id: string;
  cliente_id: string;
  tipo?: string;
  marca: string;
  modelo?: string;
  btus?: number;
  numero_serie?: string;
  ambiente?: string;
  local_instalacao?: string;
  data_instalacao?: string;
  ultima_manutencao?: string;
  observacoes?: string;
}

export interface SupabaseService {
  id: string;
  cliente_id: string;
  aparelho_id?: string;
  tecnico_id?: string;
  tipo: 'INSTALACAO' | 'MANUTENCAO_PREVENTIVA' | 'MANUTENCAO_CORRETIVA' | 'LIMPEZA' | 'RECARGA_GAS' | 'AVALIACAO' | 'OUTRO';
  descricao?: string;
  problema?: string;
  data_solicitacao?: string;
  data_agendamento?: string;
  hora_agendamento?: string;
  data_inicio?: string;
  data_conclusao?: string;
  data_cancelamento?: string;
  motivo_cancelamento?: string;
  status: 'PENDENTE' | 'AGENDADO' | 'EM_ANDAMENTO' | 'CONCLUIDO' | 'CANCELADO';
  valor?: number;
  observacoes?: string;
  created_at?: string;
}

export interface SupabaseBudget {
  id: string;
  numero: string;
  cliente_id: string;
  service_id?: string;
  data: string;
  validade?: string;
  descricao?: string;
  tipo_servico?: string;
  btus?: number;
  valor_mao_obra: number;
  valor_material: number;
  valor_total: number;
  condicoes?: string;
  responsavel_tecnico?: string;
  status: 'RASCUNHO' | 'ENVIADO' | 'APROVADO' | 'RECUSADO' | 'EXPIRADO';
  created_at?: string;
  updated_at?: string;
}

// Converte um orçamento do banco (budgets) para o formato interno do app.
// Os itens completos ficam em JSON na coluna descricao (mantém o schema do site).
export function mapSupabaseBudgetToLocal(sb: SupabaseBudget): BudgetEstimate {
  let meta: any = {};
  try {
    meta = JSON.parse(sb.descricao || '{}');
  } catch {
    meta = {};
  }

  const items: BudgetItem[] = Array.isArray(meta.items) && meta.items.length > 0
    ? meta.items
    : sb.descricao
      ? [{
          id: '1',
          description: String(sb.descricao).slice(0, 200),
          quantity: 1,
          unitPrice: Number(sb.valor_total) || 0,
          totalPrice: Number(sb.valor_total) || 0,
          category: 'servico'
        }]
      : [];

  return {
    id: sb.id,
    numero: meta.numero || sb.numero,
    clientId: sb.cliente_id,
    clientName: meta.clientName || 'Cliente Inovar',
    clientPhone: meta.clientPhone || '',
    clientAddress: meta.clientAddress || '',
    clientDocument: meta.clientDocument || '',
    equipmentName: meta.equipmentName || '',
    applianceDesc: meta.applianceDesc || sb.tipo_servico || '',
    date: sb.data,
    validUntil: sb.validade || sb.data,
    items,
    totalValue: Number(sb.valor_mao_obra || 0) + Number(sb.valor_material || 0) || meta.totalValue || 0,
    discount: Number(meta.discount) || 0,
    finalValue: Number(sb.valor_total) || 0,
    paymentConditions: sb.condicoes || meta.paymentConditions || 'A combinar',
    executionTime: meta.executionTime || 'A combinar',
    warrantyTerms: meta.warrantyTerms || 'Não informado — parametrização não registrada neste orçamento',
    status: sb.status === 'APROVADO' ? 'aprovado' : sb.status === 'RECUSADO' ? 'recusado' : 'pendente',
    notes: meta.notes || '',
    assinatura: meta.assinatura || null,
    assinaturaEm: meta.assinatura_em || null,
    pago: !!meta.pago,
    pagoEm: meta.pago_em || null,
    valorRecebido: meta.valor_recebido != null ? Number(meta.valor_recebido) : null,
    applianceId: meta.applianceId || null,
    serviceId: sb.service_id || meta.service_id || null
  };
}

// Monta o payload da tabela budgets a partir do orçamento interno do app.
export function mapLocalBudgetToSupabase(
  budget: BudgetEstimate,
  numero: string,
  clienteId: string,
  responsavel: string
) {
  const valorMaoObra = budget.items
    .filter((i) => i.category === 'servico')
    .reduce((acc, i) => acc + (i.totalPrice || 0), 0);
  const valorMaterial = budget.items
    .filter((i) => i.category !== 'servico')
    .reduce((acc, i) => acc + (i.totalPrice || 0), 0);

  const descricao = JSON.stringify({
    numero,
    clientName: budget.clientName,
    clientPhone: budget.clientPhone,
    clientAddress: budget.clientAddress || '',
    clientDocument: budget.clientDocument || '',
    equipmentName: budget.equipmentName || '',
    applianceDesc: budget.applianceDesc,
    items: budget.items,
    discount: budget.discount,
    totalValue: budget.totalValue,
    executionTime: budget.executionTime,
    warrantyTerms: budget.warrantyTerms,
    notes: budget.notes,
    assinatura: budget.assinatura || null,
    assinatura_em: budget.assinaturaEm || null,
    pago: budget.pago || false,
    pago_em: budget.pagoEm || null,
    valor_recebido: budget.valorRecebido ?? null,
    applianceId: budget.applianceId || null,
    service_id: budget.serviceId || undefined
  });

  return {
    numero,
    cliente_id: clienteId,
    data: budget.date,
    validade: budget.validUntil,
    descricao,
    tipo_servico: budget.items[0]?.description?.slice(0, 120) || budget.applianceDesc?.slice(0, 120) || 'Serviço Inovar',
    btus: undefined,
    valor_mao_obra: valorMaoObra,
    valor_material: valorMaterial,
    valor_total: budget.finalValue,
    condicoes: budget.paymentConditions,
    responsavel_tecnico: responsavel,
    status: 'ENVIADO' as const
  };
}

export const SupabaseService = {
  // Authentication
  async signInWithGoogle() {
    return supabase.auth.signInWithOAuth({
      provider: 'google',
      options: {
        // Login pede somente os dados básicos. Agenda e contatos são
        // autorizados separadamente, evitando bloquear a entrada por escopo.
        scopes: 'email profile',
        redirectTo: window.location.origin
      }
    });
  },

  async connectGoogle(scopes = 'email profile') {
    return supabase.auth.linkIdentity({ provider: 'google', options: {
      redirectTo: window.location.origin, scopes,
      queryParams: { access_type: 'offline', prompt: 'consent' }
    } });
  },

  async requestPasswordReset(email: string) {
    return supabase.auth.resetPasswordForEmail(email, { redirectTo: window.location.origin + '/?recovery=1' });
  },

  async getSession() {
    const { data } = await supabase.auth.getSession();
    return data.session;
  },

  async getProfile(userId: string): Promise<SupabaseProfile | null> {
    const { data, error } = await supabase
      .from('profiles')
      .select('*')
      .eq('id', userId)
      .maybeSingle();

    if (error || !data) return null;
    return data as SupabaseProfile;
  },

  async signIn(email: string, password: string) {
    return await supabase.auth.signInWithPassword({ email, password });
  },

  async signOut() {
    return await supabase.auth.signOut();
  },

  async signUpCustomer(
    email: string,
    password: string,
    nome: string,
    whatsapp: string,
    endereco?: string,
    bairro?: string,
    cidade?: string
  ) {
    // O trigger handle_new_user() no banco cria automaticamente
    // o profile (CLIENTE) e o customer vinculados ao auth.users.
    const { data: authData, error: authError } = await supabase.auth.signUp({
      email,
      password,
      options: {
        data: { nome, telefone: whatsapp }
      }
    });

    if (authError || !authData.user) {
      throw authError || new Error('Não foi possível criar o usuário');
    }

    const { data: sessionData } = await supabase.auth.getSession();
    const hasSession = !!sessionData?.session;

    // Com sessão ativa, complementa o cadastro criado pelo trigger
    if (hasSession) {
      const { data: cust } = await supabase
        .from('customers')
        .select('id')
        .eq('profile_id', authData.user.id)
        .maybeSingle();

      if (cust?.id) {
        await supabase
          .from('customers')
          .update({
            whatsapp: whatsapp || undefined,
            endereco: endereco || undefined,
            bairro: bairro || undefined,
            cidade: cidade || undefined
          })
          .eq('id', cust.id);
      }
    }

    return { user: authData.user, hasSession };
  },

  // Customers & Appliances (Two-way sync)
  async fetchCustomersWithAppliances() {
    const { data: customers, error: custErr } = await supabase
      .from('customers')
      .select('*')
      .order('nome', { ascending: true });

    if (custErr || !customers) {
      console.error('Erro ao buscar clientes no Supabase:', custErr);
      return [];
    }

    const { data: appliances, error: appErr } = await supabase
      .from('air_conditioners')
      .select('*');

    const appMap: Record<string, SupabaseAirConditioner[]> = {};
    if (appliances) {
      appliances.forEach(a => {
        if (!appMap[a.cliente_id]) appMap[a.cliente_id] = [];
        appMap[a.cliente_id].push(a);
      });
    }

    return customers.map(c => ({
      ...c,
      appliances: appMap[c.id] || []
    }));
  },

  async createCustomer(customer: Partial<SupabaseCustomer>) {
    return await supabase.from('customers').insert(customer).select().single();
  },

  async updateCustomer(customerId: string, fields: Partial<SupabaseCustomer>) {
    return await supabase.from('customers').update(fields).eq('id', customerId);
  },

  async createAirConditioner(appliance: Partial<SupabaseAirConditioner>) {
    return await supabase.from('air_conditioners').insert(appliance).select().single();
  },

  async updateAirConditionerMaintenance(applianceId: string, lastMaintenanceDate: string) {
    return await supabase
      .from('air_conditioners')
      .update({ ultima_manutencao: lastMaintenanceDate })
      .eq('id', applianceId);
  },

  // Edição completa da ficha do aparelho (sincronizada entre plataformas)
  async updateAirConditioner(applianceId: string, fields: {
    tipo?: string;
    marca?: string;
    modelo?: string;
    btus?: number;
    numero_serie?: string;
    ambiente?: string;
    local_instalacao?: string;
    data_instalacao?: string;
    observacoes?: string;
  }) {
    return await supabase.from('air_conditioners').update(fields).eq('id', applianceId);
  },

  // Reagendar: altera a data do serviço e do agendamento vinculado
  async updateServiceDate(serviceId: string, novaData: string) {
    return await supabase.from('services').update({ data_agendamento: novaData }).eq('id', serviceId);
  },

  async updateAppointmentData(serviceId: string, novaData: string, hora?: string) {
    const fields: any = { data: novaData };
    if (hora) fields.hora = hora;
    return await supabase.from('appointments').update(fields).eq('service_id', serviceId);
  },

  // Services / OS
  async fetchServices() {
    return await supabase
      .from('services')
      .select(`
        *,
        customers:cliente_id (id, nome, whatsapp, endereco, bairro),
        air_conditioners:aparelho_id (id, marca, modelo, btus, ambiente)
      `)
      .order('data_solicitacao', { ascending: false });
  },

  async createService(service: Partial<SupabaseService>) {
    return await supabase.from('services').insert(service).select().single();
  },

  async updateServiceStatus(serviceId: string, status: SupabaseService['status'], valor?: number) {
    const updatePayload: any = { status };
    if (valor !== undefined) updatePayload.valor = valor;
    return await supabase
      .from('services')
      .update(updatePayload)
      .eq('id', serviceId);
  },

  // Edição completa de um serviço (sincronizada entre plataformas)
  async updateService(serviceId: string, fields: {
    tipo?: string;
    data_agendamento?: string;
    hora_agendamento?: string;
    data_inicio?: string;
    data_conclusao?: string;
    data_cancelamento?: string;
    motivo_cancelamento?: string;
    valor?: number;
    descricao?: string;
    problema?: string;
    observacoes?: string;
    status?: string;
  }) {
    return await supabase.from('services').update(fields).eq('id', serviceId).select().single();
  },

  // Edição de orçamento (valores + JSON de detalhes)
  async updateBudget(budgetId: string, fields: {
    data?: string;
    validade?: string;
    descricao?: string;
    tipo_servico?: string;
    valor_mao_obra?: number;
    valor_material?: number;
    valor_total?: number;
    condicoes?: string;
    status?: string;
    service_id?: string;
  }) {
    return await supabase.from('budgets').update(fields).eq('id', budgetId);
  },

  // Customer Portal Specific
  async fetchCustomerByProfileId(profileId: string) {
    const { data, error } = await supabase
      .from('customers')
      .select('*')
      .eq('profile_id', profileId)
      .maybeSingle();

    if (error || !data) return null;
    return data as SupabaseCustomer;
  },

  async fetchCustomerAppliances(customerId: string) {
    const { data, error } = await supabase
      .from('air_conditioners')
      .select('*')
      .eq('cliente_id', customerId);

    if (error || !data) return [];
    return data as SupabaseAirConditioner[];
  },

  async fetchCustomerServices(customerId: string) {
    const { data, error } = await supabase
      .from('services')
      .select('*')
      .eq('cliente_id', customerId)
      .order('data_solicitacao', { ascending: false });

    if (error || !data) return [];
    return data as SupabaseService[];
  },

  // Exclusões (somente ADMIN via RLS)
  async deleteCustomer(customerId: string) {
    return await supabase.from('customers').delete().eq('id', customerId);
  },

  async deleteAirConditioner(applianceId: string) {
    return await supabase.from('air_conditioners').delete().eq('id', applianceId);
  },

  async deleteService(serviceId: string) {
    return await supabase.from('services').delete().eq('id', serviceId);
  },

  // Histórico técnico completo (service_history)
  async insertServiceHistory(entry: {
    service_id?: string;
    cliente_id: string;
    aparelho_id?: string;
    data: string;
    descricao?: string;
    problema?: string;
    diagnostico?: string;
    solucao?: string;
    pecas_utilizadas?: string;
    observacoes?: string;
    valor?: number;
  }) {
    return await supabase.from('service_history').insert(entry).select().single();
  },

  async fetchServiceHistory(clienteId?: string) {
    let query = supabase
      .from('service_history')
      .select('*')
      .order('data', { ascending: false });

    if (clienteId) query = query.eq('cliente_id', clienteId);

    const { data, error } = await query;
    if (error || !data) return [];
    return data;
  },

  // Agendamentos (appointments)
  async fetchAppointments() {
    return await supabase
      .from('appointments')
      .select('*')
      .order('data', { ascending: true });
  },

  async createAppointment(appt: {
    service_id: string;
    cliente_id: string;
    data: string;
    hora?: string;
    status?: string;
    observacoes?: string;
  }) {
    return await supabase.from('appointments').insert(appt).select().single();
  },

  // ---------- ORÇAMENTOS (tabela budgets) ----------
  async fetchBudgets() {
    return await supabase
      .from('budgets')
      .select('*')
      .order('created_at', { ascending: false });
  },

  async proximoNumeroOrcamento(): Promise<string> {
    const { data } = await supabase.rpc('proximo_numero_orcamento');
    if (data && typeof data === 'string') return data;
    return new Date().getFullYear() + '-' + Date.now().toString().slice(-4);
  },

  async createBudget(payload: {
    numero: string;
    cliente_id: string;
    data: string;
    validade?: string;
    descricao?: string;
    tipo_servico?: string;
    btus?: number;
    valor_mao_obra: number;
    valor_material: number;
    valor_total: number;
    condicoes?: string;
    responsavel_tecnico?: string;
    status?: 'RASCUNHO' | 'ENVIADO' | 'APROVADO' | 'RECUSADO' | 'EXPIRADO';
  }) {
    return await supabase.from('budgets').insert(payload).select().single();
  },

  async updateBudgetStatus(budgetId: string, status: 'RASCUNHO' | 'ENVIADO' | 'APROVADO' | 'RECUSADO' | 'EXPIRADO') {
    return await supabase.from('budgets').update({ status }).eq('id', budgetId);
  },

  async deleteBudget(budgetId: string) {
    return await supabase.from('budgets').delete().eq('id', budgetId);
  }
};
