-- Keep a small delivery receipt after the operational queue's three-day purge.
-- Receipts contain no telephone number, message text or attachment.
create table if not exists public.whatsapp_delivery_receipts (
  idempotency_key text primary key,
  accepted_at timestamptz not null
);
alter table public.whatsapp_delivery_receipts enable row level security;
revoke all on public.whatsapp_delivery_receipts from public, anon, authenticated;
grant select, insert on public.whatsapp_delivery_receipts to service_role;

insert into public.whatsapp_delivery_receipts (idempotency_key, accepted_at)
select idempotency_key, coalesce(sent_at, updated_at)
from public.whatsapp_message_queue where status = 'enviado'
on conflict (idempotency_key) do nothing;

create or replace function public.guard_whatsapp_delivery_receipt()
returns trigger language plpgsql security definer
set search_path = pg_catalog, public as $$
begin
  if tg_op = 'INSERT' and exists (
    select 1 from public.whatsapp_delivery_receipts where idempotency_key = new.idempotency_key
  ) then return null; end if;
  if new.status = 'enviado' then
    insert into public.whatsapp_delivery_receipts (idempotency_key, accepted_at)
    values (new.idempotency_key, coalesce(new.sent_at, now()))
    on conflict (idempotency_key) do nothing;
  end if;
  return new;
end;
$$;
revoke all on function public.guard_whatsapp_delivery_receipt() from public, anon, authenticated;
drop trigger if exists whatsapp_delivery_receipt_guard on public.whatsapp_message_queue;
create trigger whatsapp_delivery_receipt_guard before insert or update
on public.whatsapp_message_queue for each row execute function public.guard_whatsapp_delivery_receipt();

-- Retain duplicate rows as cancelled history, without deleting customer data.
with ranked as (
  select id, row_number() over (
    partition by source_entity_type, source_entity_id
    order by (status = 'processando') desc, created_at, id
  ) as position
  from public.whatsapp_message_queue
  where event_type = 'lembrete_manutencao_recorrente'
    and status in ('pendente', 'processando') and source_entity_id is not null
)
update public.whatsapp_message_queue q
set status = 'cancelado', locked_until = null, updated_at = now(),
    last_error = 'Agendamento duplicado consolidado; registro mantido no histórico'
from ranked where ranked.id = q.id and ranked.position > 1;

create unique index if not exists whatsapp_one_active_maintenance_per_appliance
on public.whatsapp_message_queue (source_entity_type, source_entity_id)
where event_type = 'lembrete_manutencao_recorrente'
  and status in ('pendente', 'processando') and source_entity_id is not null;

-- Serialize changes per appliance and retire the previous cycle in the same
-- transaction as inserting the new schedule. The unique index is the backstop.
create or replace function public.schedule_maintenance_whatsapp(p_payload jsonb)
returns text language plpgsql security definer
set search_path = pg_catalog, public, auth as $$
declare
  appliance text := p_payload->>'source_entity_id';
  message_key text := p_payload->>'idempotency_key';
  latest_date date;
  existing public.whatsapp_message_queue;
begin
  if coalesce(auth.role(), '') <> 'service_role' then raise exception 'service role required'; end if;
  if appliance is null or message_key is null or p_payload->>'event_type' <> 'lembrete_manutencao_recorrente' then
    raise exception 'invalid maintenance schedule';
  end if;
  perform pg_advisory_xact_lock(hashtextextended('whatsapp-maintenance:' || appliance, 0));
  select max(d) into latest_date from (
    select ultima_manutencao::date as d from public.air_conditioners where id::text = appliance
    union all select data::date from public.service_history where aparelho_id::text = appliance
    union all select coalesce(data_conclusao, data_agendamento)::date from public.services
      where aparelho_id::text = appliance and status = 'CONCLUIDO'
  ) dates;
  if latest_date is null or latest_date <> (p_payload->'metadata'->>'ultima_manutencao')::date then
    return 'obsolete';
  end if;
  if exists (select 1 from public.whatsapp_delivery_receipts where idempotency_key = message_key) then return 'already_sent'; end if;
  select * into existing from public.whatsapp_message_queue where idempotency_key = message_key for update;
  if found then
	-- A technician may move the return date away and then back. Reuse the
	-- original row only when the system cancelled it before any send attempt.
	-- A cancellation explicitly requested by the user remains cancelled.
	if existing.status = 'cancelado' and existing.attempt_count = 0
	   and existing.last_error = 'Ciclo de manutenção atualizado' then
	  if exists (select 1 from public.whatsapp_message_queue where source_entity_id = appliance
	      and event_type = 'lembrete_manutencao_recorrente' and status = 'processando') then return 'processing'; end if;
	  update public.whatsapp_message_queue set status = 'cancelado', updated_at = now(),
	    last_error = 'Ciclo de manutenção atualizado'
	  where source_entity_id = appliance and event_type = 'lembrete_manutencao_recorrente' and status = 'pendente';
	  update public.whatsapp_message_queue set status = 'pendente', last_error = null,
	    scheduled_at = (p_payload->>'scheduled_at')::timestamptz,
	    next_attempt_at = (p_payload->>'next_attempt_at')::timestamptz,
	    expires_at = (p_payload->>'expires_at')::timestamptz,
	    recipient_phone = p_payload->>'recipient_phone', message_text = p_payload->>'message_text',
	    metadata = p_payload->'metadata', locked_until = null, updated_at = now()
	  where id = existing.id;
	  return 'pendente';
	end if;
    -- Preserve cancellations, delivery status, retries and the original ID.
    if existing.status = 'pendente' and existing.attempt_count = 0 then
      update public.whatsapp_message_queue set recipient_phone = p_payload->>'recipient_phone',
        message_text = p_payload->>'message_text', metadata = p_payload->'metadata', updated_at = now()
      where id = existing.id;
    end if;
    return existing.status;
  end if;
  if exists (select 1 from public.whatsapp_message_queue where source_entity_id = appliance
      and event_type = 'lembrete_manutencao_recorrente' and status = 'processando') then return 'processing'; end if;
  if exists (select 1 from public.whatsapp_message_queue where source_entity_id = appliance
      and event_type = 'lembrete_manutencao_recorrente' and status = 'pendente'
      and metadata->>'proxima_manutencao' = p_payload->'metadata'->>'proxima_manutencao'
      and (expires_at is null or expires_at > now())) then return 'pending_existing'; end if;
  update public.whatsapp_message_queue set status = 'cancelado', updated_at = now(),
    last_error = 'Ciclo de manutenção atualizado', locked_until = null
  where source_entity_id = appliance and event_type = 'lembrete_manutencao_recorrente' and status = 'pendente';
  insert into public.whatsapp_message_queue (
    idempotency_key, event_type, recipient_phone, message_text, scheduled_at,
    next_attempt_at, expires_at, source_entity_type, source_entity_id, metadata
  ) values (
    message_key, 'lembrete_manutencao_recorrente', p_payload->>'recipient_phone', p_payload->>'message_text',
    (p_payload->>'scheduled_at')::timestamptz, (p_payload->>'next_attempt_at')::timestamptz,
    (p_payload->>'expires_at')::timestamptz, 'aparelho', appliance, p_payload->'metadata'
  ) on conflict (idempotency_key) do nothing;
  return 'pendente';
end;
$$;
revoke all on function public.schedule_maintenance_whatsapp(jsonb) from public, anon, authenticated;
grant execute on function public.schedule_maintenance_whatsapp(jsonb) to service_role;
notify pgrst, 'reload schema';
