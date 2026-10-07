-- ============================================================================
-- InovarApp — PATCH COMO EXECUTAR: Supabase Dashboard → SQL Editor → colar
-- tudo aqui e clicar em RUN. Idempotente: pode ser executado mais de uma vez.
--
-- O que este patch faz:
--   1. Cria as colunas do ciclo de vida dos serviços (data_inicio, data_conclusao,
--      data_cancelamento, motivo_cancelamento) + backfill dos marcadores.
--   2. Corrige a permissão da EQUIPE: as roles TECNICO e ADMIN passam a ler e
--      escrever TODOS os dados operacionais (services, appointments,
--      service_history, budgets). Sem isso, contas do tipo TECNICO entram no app
--      e veem clientes, mas a Agenda/OS/Chamados/Orçamentos ficam VAZIAS.
-- ============================================================================

-- ----------------------------------------------------------------------------
-- 1) CICLO DE VIDA DOS SERVIÇOS
-- ----------------------------------------------------------------------------
alter table public.services
  add column if not exists data_inicio date,
  add column if not exists data_conclusao date,
  add column if not exists data_cancelamento date,
  add column if not exists motivo_cancelamento text;

comment on column public.services.data_agendamento is 'Data planejada do atendimento';
comment on column public.services.data_inicio is 'Data em que o técnico iniciou a execução';
comment on column public.services.data_conclusao is 'Data em que o serviço foi concluído';
comment on column public.services.data_cancelamento is 'Data em que o serviço foi cancelado';
comment on column public.services.motivo_cancelamento is 'Motivo informado para o cancelamento';

create index if not exists services_lifecycle_status_idx on public.services (status);
create index if not exists services_lifecycle_dates_idx on public.services (data_agendamento, data_inicio, data_conclusao, data_cancelamento);

-- Backfill: recupera datas gravadas como marcadores nas observações
update public.services
set data_conclusao = coalesce(data_conclusao, nullif(substring(coalesce(observacoes, '') from '\[DATA_CONCLUSAO:(\d{4}-\d{2}-\d{2})\]'), '')::date)
where status = 'CONCLUIDO' and data_conclusao is null;

update public.services
set data_inicio = coalesce(data_inicio, nullif(substring(coalesce(observacoes, '') from '\[DATA_INICIO:(\d{4}-\d{2}-\d{2})\]'), '')::date)
where data_inicio is null;

update public.services
set data_cancelamento = coalesce(data_cancelamento, nullif(substring(coalesce(observacoes, '') from '\[DATA_CANCELAMENTO:(\d{4}-\d{2}-\d{2})\]'), '')::date),
    motivo_cancelamento = coalesce(motivo_cancelamento, nullif(substring(coalesce(observacoes, '') from '\[MOTIVO_CANCELAMENTO:([^\]]+)\]'), ''))
where status = 'CANCELADO' and data_cancelamento is null;

-- ----------------------------------------------------------------------------
-- 2) EQUIPE (TECNICO/ADMIN) ENXERGA E OPERA TUDO
-- Políticas permissivas aditivas: não alteram as políticas existentes de
-- clientes (cada cliente continua vendo SOMENTE os próprios dados).
-- ----------------------------------------------------------------------------
do $$
declare
  t text;
  tabelas text[] := array['services', 'appointments', 'service_history', 'budgets'];
begin
  foreach t in array tabelas loop
    execute format('drop policy if exists "inovar_equipe_le_%1$s" on public.%1$I', t);
    execute format(
      'create policy "inovar_equipe_le_%1$s" on public.%1$I for select to authenticated
         using (exists (
           select 1 from public.profiles p
           where p.id = auth.uid() and p.tipo in (''TECNICO'', ''ADMIN'')
         ))', t);

    execute format('drop policy if exists "inovar_equipe_operar_%1$s" on public.%1$I', t);
    execute format(
      'create policy "inovar_equipe_operar_%1$s" on public.%1$I for all to authenticated
         using (exists (
           select 1 from public.profiles p
           where p.id = auth.uid() and p.tipo in (''TECNICO'', ''ADMIN'')
         ))
         with check (exists (
           select 1 from public.profiles p
           where p.id = auth.uid() and p.tipo in (''TECNICO'', ''ADMIN'')
         ))', t);
  end loop;
end $$;

-- Pronto! Recarregue o app (as contas TECNICO agora veem Agenda, OS,
-- Chamados, Orçamentos e Financeiro; clientes continuam isolados).
