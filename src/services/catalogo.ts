// Catálogo de tipos de serviço — FONTE ÚNICA usada pela aba Serviços, checklist,
// orçamentos, agendamentos e menu mobile. Os 6 tipos fixos da Inovar continuam
// vindo daqui, mas agora podem ser criados, editados e removidos.
import type { ComponentType } from 'react';
import { ServiceType, TechnicianProfile, DadosTipoServico } from '../types';
import { Sparkles, Zap, Wrench, Flame, ShieldCheck, Wind } from 'lucide-react';

export interface ServiceCardDef {
  type: ServiceType;
  title: string;
  subtitle: string;
  desc: string;
  tempoMedio: string;
  garantiaPadrao: string;
  itensInclusos: string[];
  valorMedioRef: string;
  badge: string;
  icon: ComponentType<{ className?: string }>;
  accentColor: string;
  gradientClass: string;
  color: 'emerald' | 'sky' | 'amber' | 'purple' | 'blue' | 'slate';
}

export const SERVICOS_FIXOS: ServiceCardDef[] = [
  {
    type: 'Limpeza de Ar',
    title: 'Limpeza de Ar Completa',
    subtitle: 'Com bactericida hospitalar e lavagem de serpentina',
    desc: 'Desmontagem da carenagem, limpeza com bolsa coletora e lava jato pressurizado, assepsia de serpentina, turbina e bandeja de dreno.',
    tempoMedio: '1h20 min',
    garantiaPadrao: '90 dias',
    itensInclusos: [
      'Lavagem de filtros e carenagem plástica',
      'Desinfecção de serpentina com bactericida',
      'Limpeza profunda da turbina de ventilação',
      'Desobstrução do dreno e lavagem da bandeja',
      'Medição de salto térmico (ΔT) e corrente'
    ],
    valorMedioRef: 'R$ 280',
    badge: 'Mais Executado',
    icon: Sparkles,
    accentColor: 'text-emerald-400',
    gradientClass: 'from-emerald-950/40 via-slate-900 to-slate-900 border-emerald-500/30',
    color: 'emerald'
  },
  {
    type: 'Instalação',
    title: 'Instalação Split / Inverter',
    desc: 'Instalação em conformidade com as normas dos fabricantes, vácuo controlado abaixo de 500 microns e teste de estanqueidade.',
    subtitle: 'Split Hi-Wall, Multi-Split e Inverter',
    tempoMedio: '3h - 4h',
    garantiaPadrao: '180 a 365 dias',
    itensInclusos: [
      'Fixação com suporte nivelado e buchas apropriadas',
      'Tubulação de cobre com isolamento térmico blindado',
      'Vácuo profundo com vacuômetro digital (< 500 µ)',
      'Teste de estanqueidade e vazamento com N2',
      'Liberação controlada de fluido refrigerante'
    ],
    valorMedioRef: 'R$ 1.300',
    badge: 'Padrão Técnico',
    icon: Zap,
    accentColor: 'text-sky-400',
    gradientClass: 'from-sky-950/40 via-slate-900 to-slate-900 border-sky-500/30',
    color: 'sky'
  },
  {
    type: 'Manutenção Corretiva',
    title: 'Conserto & Troca de Peças',
    desc: 'Diagnóstico de falhas elétricas e mecânicas, substituição de componentes danificados com teste de carga e rendimento.',
    subtitle: 'Capacitores, placas, ventiladores e sensores',
    tempoMedio: '1h - 2h',
    garantiaPadrao: '90 dias',
    itensInclusos: [
      'Diagnóstico com capacímetro e osciloscópio/multímetro',
      'Substituição de capacitor de partida / ventilação',
      'Reparo ou troca de placa eletrônica / display',
      'Troca de sensor de temperatura e degelo',
      'Teste de funcionamento contínuo e desarme'
    ],
    valorMedioRef: 'R$ 250 - 625 + peças',
    badge: 'Diagnóstico Preciso',
    icon: Wrench,
    accentColor: 'text-amber-400',
    gradientClass: 'from-amber-950/40 via-slate-900 to-slate-900 border-amber-500/30',
    color: 'amber'
  },
  {
    type: 'Recarga de Gás',
    title: 'Carga de Gás Refrigerante',
    desc: 'Localização e correção de microvazamentos em flanges/soldas e reposição exata de fluido refrigerante por peso na balança digital.',
    subtitle: 'R-410A, R-32, R-22 na balança de precisão',
    tempoMedio: '1h30 min',
    garantiaPadrao: '90 dias',
    itensInclusos: [
      'Teste de pressurização com nitrogênio e detector',
      'Reaperto e refazimento de flanges com vazamento',
      'Vácuo para retirada de umidade do circuito',
      'Carga em fase líquida por balança (gramas)',
      'Medição de superaquecimento e sub-resfriamento'
    ],
    valorMedioRef: 'R$ 305 - 625',
    badge: 'Pesagem Exata',
    icon: Flame,
    accentColor: 'text-purple-400',
    gradientClass: 'from-purple-950/40 via-slate-900 to-slate-900 border-purple-500/30',
    color: 'purple'
  },
  {
    type: 'Manutenção Preventiva',
    title: 'Manutenção Preventiva / PMOC',
    desc: 'Plano de Manutenção, Operação e Controle para empresas, consultórios e residências com laudo técnico e conformidade Anvisa.',
    subtitle: 'Conformidade legal e máxima economia energética',
    tempoMedio: 'Mensal / Trimestral',
    garantiaPadrao: 'Contrato ativo',
    itensInclusos: [
      'Limpeza de ar periódica programada',
      'Medição de consumo e corrente elétrica (A)',
      'Aferição de pressões e temperatura insuflada',
      'Emissão de Laudo Técnico e Termo de Garantia',
      'Prioridade de atendimento em chamados'
    ],
    valorMedioRef: 'Contratos / Sob Consulta',
    badge: 'Empresarial & Residencial',
    icon: ShieldCheck,
    accentColor: 'text-blue-400',
    gradientClass: 'from-blue-950/40 via-slate-900 to-slate-900 border-blue-500/30',
    color: 'blue'
  },
  {
    type: 'Avaliação Técnica',
    title: 'Avaliação & Visita Técnica',
    desc: 'Visita para dimensionamento de carga térmica (BTUs por m²), vistoria de instalações elétricas e laudo de orçamento.',
    subtitle: 'Cálculo de carga térmica e vistoria',
    tempoMedio: '45 min',
    garantiaPadrao: 'Orçamento 15 dias',
    itensInclusos: [
      'Cálculo de incidência solar e metragem cúbica',
      'Inspeção do quadro elétrico e aterramento',
      'Avaliação das tubulações e pontos de dreno',
      'Proposta orçamentária detalhada'
    ],
    valorMedioRef: 'R$ 110 - 170 (abatível)',
    badge: 'Visita Técnica',
    icon: Wind,
    accentColor: 'text-slate-300',
    gradientClass: 'from-slate-800/40 via-slate-900 to-slate-900 border-slate-700',
    color: 'slate'
  }
];

export interface TipoCatalogo {
  key: string;                 // 'fixo:<tipo original>' | 'custom:<índice>'
  fixo?: ServiceType;          // tipo original quando é um fixo da Inovar
  nome: string;                // nome atual (considerando renomeação)
  preco: number;               // preço padrão (0 = não definido / sob consulta)
  editado?: boolean;           // fixo com edições aplicadas
  desc?: string;               // descrição curta (sobrepõe o card do fixo)
  tempoMedio?: string;
  garantiaPadrao?: string;
  badge?: string;
  itens?: string[];            // procedimento padrão editável
  card?: ServiceCardDef;       // conteúdo rico (apenas tipos fixos)
}

// Corrige dados legados gravados com codificação incorreta (ex.:
// "Avaliação Técnica") antes de qualquer tela, PDF ou integração usá-los.
// Serviços personalizados permanecem intactos.
export function normalizarNomeServico(valor: unknown): string {
  const original = String(valor ?? '').trim();
  if (!original) return 'Outro';
  const chave = original
    .normalize('NFD').replace(/[\u0300-\u036f]/g, '')
    .replace(/\uFFFD/g, '')
    .replace(/[^a-zA-Z]/g, '')
    .toLowerCase();
  if (chave.includes('avali') && (chave.includes('tecn') || chave.includes('tcn'))) return 'Avaliação Técnica';
  if (chave === 'avaliacao') return 'Avaliação Técnica';
  // Mantém os registros antigos compatíveis, mas nunca volta a exibir a
  // nomenclatura anterior no PWA, documentos ou integrações.
  if (chave === 'limpeza' || chave === 'limpezadearcompleta') return 'Limpeza de Ar';
  return original;
}

// Monta o catálogo resolvido: fixos (menos removidos, com edições aplicadas) + personalizados
export function montarCatalogo(profile?: Partial<TechnicianProfile>): TipoCatalogo[] {
  const removidos = profile?.tiposFixosRemovidos || [];
  const editados = profile?.tiposFixosEditados || [];
  const fixos: TipoCatalogo[] = SERVICOS_FIXOS
    .filter((f) => !removidos.some((tipo) => normalizarNomeServico(tipo) === f.type))
    .map((f) => {
      const ed = editados.find((e) => normalizarNomeServico(e.tipo) === f.type);
      if (!ed) {
        return { key: 'fixo:' + f.type, fixo: f.type, nome: f.type, preco: extrairPrecoReferencia(f.valorMedioRef), card: f };
      }
      return {
        key: 'fixo:' + f.type,
        fixo: f.type,
        nome: normalizarNomeServico(ed.nome || f.type),
        preco: ed.preco || 0,
        editado: true,
        desc: ed.desc,
        tempoMedio: ed.tempoMedio,
        garantiaPadrao: ed.garantiaPadrao,
        badge: ed.badge,
        itens: ed.itens,
        card: f
      };
    });
  const customs: TipoCatalogo[] = (profile?.tiposServicosCustom || []).map((t: DadosTipoServico, idx) => ({
    key: 'custom:' + idx,
    nome: normalizarNomeServico(t.nome),
    preco: t.preco || 0,
    desc: t.desc,
    tempoMedio: t.tempoMedio,
    garantiaPadrao: t.garantiaPadrao,
    badge: t.badge,
    itens: t.itens
  }));
  return [...fixos, ...customs];
}

// Extrai um preço sugerido do texto de referência (ex.: 'R$ 210 - 310' -> 210)
export function extrairPrecoReferencia(ref: string): number {
  const m = (ref || '').match(/([\d.]+)/);
  if (!m) return 0;
  const n = Number(m[1].replace(/\./g, ''));
  return isNaN(n) ? 0 : n;
}
