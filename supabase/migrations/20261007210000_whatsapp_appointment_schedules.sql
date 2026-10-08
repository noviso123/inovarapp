-- Each appointment has at most one active message of each reminder kind.
with ranked as (
  select id, row_number() over (partition by source_entity_id, event_type
    order by (status = 'processando') desc, created_at, id) as position
  from public.whatsapp_message_queue where status in ('pendente', 'processando')
    and event_type in ('lembrete_agendamento_vespera', 'lembrete_agendamento_uma_hora')
    and source_entity_id is not null
)
update public.whatsapp_message_queue q set status = 'cancelado', locked_until = null,
  updated_at = now(), last_error = 'Aviso duplicado consolidado; registro mantido no histórico'
from ranked where ranked.id = q.id and ranked.position > 1;
create unique index if not exists whatsapp_one_active_appointment_reminder
on public.whatsapp_message_queue (source_entity_id, event_type)
where status in ('pendente', 'processando') and source_entity_id is not null
  and event_type in ('lembrete_agendamento_vespera', 'lembrete_agendamento_uma_hora');

-- HH:MM and HH:MM:SS represent the same appointment, even after queue cleanup.
insert into public.whatsapp_delivery_receipts (idempotency_key, accepted_at)
select regexp_replace(idempotency_key, '(:[0-9]{2}:[0-9]{2}):[0-9]{2}$', '\1'), accepted_at
from public.whatsapp_delivery_receipts where idempotency_key like 'agenda-%'
on conflict (idempotency_key) do nothing;
create or replace function public.keep_appointment_delivery_receipt()
returns trigger language plpgsql security definer set search_path = pg_catalog, public as $$
begin
  if new.status = 'enviado' and new.event_type in ('lembrete_agendamento_vespera', 'lembrete_agendamento_uma_hora') then
    insert into public.whatsapp_delivery_receipts (idempotency_key, accepted_at)
    values (regexp_replace(new.idempotency_key, '(:[0-9]{2}:[0-9]{2}):[0-9]{2}$', '\1'), coalesce(new.sent_at, now()))
    on conflict (idempotency_key) do nothing;
  end if;
  return new;
end; $$;
revoke all on function public.keep_appointment_delivery_receipt() from public, anon, authenticated;
drop trigger if exists whatsapp_appointment_delivery_receipt on public.whatsapp_message_queue;
create trigger whatsapp_appointment_delivery_receipt after insert or update on public.whatsapp_message_queue
for each row execute function public.keep_appointment_delivery_receipt();

create or replace function public.schedule_appointment_whatsapp(p_payload jsonb)
returns text language plpgsql security definer set search_path = pg_catalog, public, auth as $$
declare
  service public.services;
  existing public.whatsapp_message_queue;
  service_id text := p_payload->>'source_entity_id';
  kind text := p_payload->>'event_type';
  message_key text := p_payload->>'idempotency_key';
  scheduled timestamptz;
  expiry timestamptz;
begin
  if coalesce(auth.role(), '') <> 'service_role' then raise exception 'service role required'; end if;
  if service_id is null or message_key is null or kind not in ('lembrete_agendamento_vespera', 'lembrete_agendamento_uma_hora') then raise exception 'invalid appointment reminder'; end if;
  perform pg_advisory_xact_lock(hashtextextended('whatsapp-appointment:' || service_id || ':' || kind, 0));
  select * into service from public.services where id::text = service_id for share;
  if not found or service.status <> 'AGENDADO'
    or service.data_agendamento::text is distinct from p_payload->'metadata'->>'data_agendamento'
    or left(coalesce(service.hora_agendamento::text, ''), 5) <> left(coalesce(p_payload->'metadata'->>'hora_agendamento', ''), 5)
    or not exists (select 1 from public.customers where id = service.cliente_id and coalesce(ativo, true)) then return 'obsolete'; end if;
  if kind = 'lembrete_agendamento_vespera' then
    scheduled := ((service.data_agendamento - 1) + time '09:00') at time zone 'America/Sao_Paulo';
    expiry := ((service.data_agendamento - 1) + time '20:00') at time zone 'America/Sao_Paulo';
  else
    if service.hora_agendamento is null then return 'missing_time'; end if;
    expiry := (service.data_agendamento + service.hora_agendamento::time) at time zone 'America/Sao_Paulo';
    scheduled := expiry - interval '1 hour';
  end if;
  if expiry <= now() then return 'expired'; end if;
  if exists (select 1 from public.whatsapp_delivery_receipts where idempotency_key in
    (message_key, regexp_replace(message_key, '(:[0-9]{2}:[0-9]{2}):[0-9]{2}$', '\1'))) then return 'already_sent'; end if;
  select * into existing from public.whatsapp_message_queue q where q.source_entity_id = service_id and q.event_type = kind
    and (q.idempotency_key = message_key or (q.metadata->>'data_agendamento' = service.data_agendamento::text
      and left(coalesce(q.metadata->>'hora_agendamento', ''), 5) = left(coalesce(service.hora_agendamento::text, ''), 5)))
  order by case q.status when 'enviado' then 0 when 'processando' then 1 when 'pendente' then 2 else 3 end, q.created_at limit 1 for update;
  if found then
    if existing.status = 'cancelado' and existing.attempt_count = 0 and existing.last_error in
      ('Agendamento alterado; aviso anterior cancelado', 'Agendamento alterado ou serviço encerrado; aviso anterior cancelado') then
      if exists (select 1 from public.whatsapp_message_queue where source_entity_id = service_id and event_type = kind and status = 'processando') then return 'processing'; end if;
      update public.whatsapp_message_queue set status = 'cancelado', updated_at = now(), last_error = 'Agendamento alterado; aviso anterior cancelado'
      where source_entity_id = service_id and event_type = kind and status = 'pendente';
      update public.whatsapp_message_queue set status = 'pendente', scheduled_at = scheduled, next_attempt_at = scheduled,
        expires_at = expiry, last_error = null, locked_until = null, recipient_phone = p_payload->>'recipient_phone',
        message_text = p_payload->>'message_text', metadata = p_payload->'metadata', updated_at = now() where id = existing.id;
      return 'pendente';
    end if;
    if existing.status = 'pendente' and existing.attempt_count = 0 then
      update public.whatsapp_message_queue set scheduled_at = scheduled, next_attempt_at = scheduled, expires_at = expiry,
        recipient_phone = p_payload->>'recipient_phone', message_text = p_payload->>'message_text',
        metadata = p_payload->'metadata', updated_at = now() where id = existing.id;
    end if;
    return existing.status;
  end if;
  if exists (select 1 from public.whatsapp_message_queue where source_entity_id = service_id and event_type = kind and status = 'processando') then return 'processing'; end if;
  update public.whatsapp_message_queue set status = 'cancelado', updated_at = now(), last_error = 'Agendamento alterado; aviso anterior cancelado'
  where source_entity_id = service_id and event_type = kind and status = 'pendente';
  insert into public.whatsapp_message_queue (idempotency_key, event_type, recipient_phone, message_text,
    scheduled_at, next_attempt_at, expires_at, source_entity_type, source_entity_id, metadata)
  values (message_key, kind, p_payload->>'recipient_phone', p_payload->>'message_text', scheduled, scheduled, expiry, 'servico', service_id, p_payload->'metadata')
  on conflict (idempotency_key) do nothing;
  return 'pendente';
end; $$;
revoke all on function public.schedule_appointment_whatsapp(jsonb) from public, anon, authenticated;
grant execute on function public.schedule_appointment_whatsapp(jsonb) to service_role;
notify pgrst, 'reload schema';
