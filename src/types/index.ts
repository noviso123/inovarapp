export type ReturnStatus = 'atrasado' | 'esta_semana' | 'em_breve' | 'em_dia' | 'sem_historico';

export type ServiceType = 
  | 'Limpeza de Ar'
  | 'Instalação'
  | 'Manutenção Corretiva'
  | 'Recarga de Gás'
  | 'Manutenção Preventiva'
  | 'Avaliação Técnica'
  | 'Outro';

export interface Appliance {
  id: string;
  clientId: string;
  brand: string; // LG, Gree, Midea, Samsung, Daikin, Carrier, Elgin, Fujitsu, etc.
  model?: string;
  type: 'Split Hi-Wall' | 'Inverter' | 'Cassete' | 'Piso Teto' | 'Janela' | 'Multi Split' | 'Portátil';
  capacityBtu: string; // '9.000', '12.000', '18.000', '24.000', etc.
  room: string; // 'Sala', 'Quarto', 'Quarto Casal', 'Escritório', etc.
  serialNumber?: string;
  installationSite?: string;
  gasType?: 'R-410A' | 'R-32' | 'R-22' | 'Outro';
  voltage?: '220V' | '110V' | 'Bivolt';
  installDate?: string;
  notes?: string;
}

export interface ChecklistData {
  // Limpeza de ar
  filtrosLavados?: boolean;
  serpentinaHigienizada?: boolean;
  turbinaLimpa?: boolean;
  drenoDesobstruido?: boolean;
  bandejaSanitizada?: boolean;
  condensadoraLavada?: boolean;
  aplicacaoBactericida?: boolean;
  
  // Medições e Testes Técnicos
  testeEletricoCorrente?: boolean;
  correnteAmperes?: string;
  pressaoGasPSI?: string;
  saltoTermicoDeltaT?: string;
  temperaturaInsuflamento?: string;
  temperaturaRetorno?: string;

  // Instalação
  suporteNivelado?: boolean;
  vacuoMicrons?: string;
  testeNitrogenio?: boolean;
  valvulasLiberadas?: boolean;
  superaquecimentoUtil?: string;

  // Manutenção Corretiva e Carga de Gás
  capacitorTestado?: string;
  gasAdicionadoGramas?: string;
  pecasSubstituidas?: string;
  diagnosticoTecnico?: string;
}

export interface MaintenanceRecord {
  id: string;
  clientId: string;
  applianceId: string;
  date: string; // YYYY-MM-DD — data de conclusão; legado pode usar a data do serviço
  scheduledDate?: string; // YYYY-MM-DD — data planejada
  scheduledTime?: string; // HH:MM — horário planejado
  completionDate?: string; // YYYY-MM-DD — preenchida somente ao finalizar
  completedAt?: string | null;
  returnDate: string; // YYYY-MM-DD — somente após conclusão
  serviceType: ServiceType;
  price: number;
  laborPrice?: number;
  partsPrice?: number;
  partsUsed?: string;
  paymentMethod: 'PIX' | 'Cartão Crédito' | 'Cartão Débito' | 'Dinheiro' | 'A Faturar';
  warrantyDays: number; // 30, 90, 180, 365 days
  notes?: string;
  checklist?: ChecklistData;
  startedAt?: string | null;
  cancelledAt?: string | null;
  cancellationReason?: string;
  status: 'concluido' | 'agendado' | 'em_andamento' | 'cancelado';
  contactedAt?: string;
  attachments?: { name: string; path?: string; url?: string; type?: string; size?: number }[];
}

export interface Client {
  id: string;
  name: string;
  phone: string;
  document?: string;
  address?: string;
  neighborhood?: string;
  city?: string;
  notes?: string;
  createdAt: string;
  appliances: Appliance[];
}

export interface TechnicianProfile {
  name: string;
  businessName: string;
  phone: string;
  pixKey: string;
  pixType: 'cpf' | 'cnpj' | 'email' | 'telefone' | 'aleatoria';
  defaultReturnMonths: number; // 6 months
  defaultWarrantyDays: number; // 90 days
  defaultPrice: number;
  cnpj?: string;
  address?: string;
  assinatura?: string | null; // assinatura do técnico (PNG base64) — sai na OS
  tiposServicosCustom?: TipoServicoCustom[]; // tipos cadastrados pelo admin
  mensagensWhats?: Record<string, string>; // textos personalizados das mensagens automáticas (Central de Mensagens)
  lembrete_intervalo_dias?: number; // reenvio de lembrete de ciclo vencido (dias; padrão 7)
  tiposFixosRemovidos?: string[]; // tipos fixos do catálogo removidos pelo admin
  tiposFixosEditados?: TipoFixoEditado[]; // edição completa de tipos fixos
  calendario_token?: string; // token do feed ICS de assinatura do calendário (Google/Apple/Outlook)
}

// Campos editáveis de qualquer tipo de serviço (fixo ou personalizado)
export interface DadosTipoServico {
  nome: string;
  preco: number;
  desc?: string;
  tempoMedio?: string;
  garantiaPadrao?: string;
  badge?: string;
  itens?: string[]; // procedimento padrão, um por linha
}

export interface TipoServicoCustom extends DadosTipoServico {}

export interface TipoFixoEditado extends DadosTipoServico {
  tipo: string; // tipo fixo original sendo editado
}

export interface BudgetItem {
  id: string;
  description: string;
  quantity: number;
  unitPrice: number;
  totalPrice: number;
  category: 'servico' | 'peca' | 'material';
}

export interface BudgetEstimate {
  id: string;
  numero?: string;
  clientId: string;
  clientName: string;
  clientPhone: string;
  clientAddress?: string;
  clientDocument?: string;
  equipmentName?: string;
  applianceDesc?: string;
  date: string;
  validUntil: string;
  items: BudgetItem[];
  totalValue: number;
  discount: number;
  finalValue: number;
  paymentConditions: string;
  executionTime: string;
  warrantyTerms: string;
  status: 'pendente' | 'aprovado' | 'recusado' | 'cancelado';
  canceladoEm?: string | null;
  notes?: string;
  assinatura?: string | null; // PNG base64 (assinatura digital do cliente)
  assinaturaEm?: string | null;
  pago?: boolean; // valor recebido pelo tecnico
  pagoEm?: string | null;
  valorRecebido?: number | null; // valor efetivamente recebido
  applianceId?: string | null; // aparelho vinculado ao servico
  serviceId?: string | null; // OS gerada a partir deste orçamento (dedupe do faturamento)
}
