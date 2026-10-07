-- Ciclo de vida dos serviços/OS
-- Aplicar no Supabase SQL Editor ou via pipeline de migrações.
-- Idempotente: pode ser executada mais de uma vez.

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

-- Backfill seguro para registros existentes.
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

-- A aplicação continua usando fallback em observações enquanto a migração não for aplicada.
