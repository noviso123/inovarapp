-- Validate the source again after claiming and before contacting WhatsApp.
-- A disconnected account must not deliver obsolete reminders on reconnect.
create or replace function public.validate_whatsapp_reminder(p_message_id uuid, p_attempt_number integer)
returns boolean language plpgsql security definer
set search_path = pg_catalog, public, auth as $$
declare
  message public.whatsapp_message_queue;
  latest_date date;
  valid boolean := true;
begin
  if coalesce(auth.role(), '') <> 'service_role' then raise exception 'service role required'; end if;
  select * into message from public.whatsapp_message_queue where id = p_message_id for update;
  if not found or message.status <> 'processando' or message.attempt_count <> p_attempt_number
     or message.locked_until is null or message.locked_until <= now() then return false; end if;
  if message.expires_at is not null and message.expires_at <= now() then
    update public.whatsapp_message_queue set status = 'expirado', locked_until = null,
      message_text = '', last_error = 'Prazo do lembrete encerrado', updated_at = now() where id = message.id;
    return false;
  end if;
  if message.event_type = 'lembrete_manutencao_recorrente' then
    valid := exists (select 1 from public.air_conditioners a join public.customers c on c.id = a.cliente_id
      where a.id::text = message.source_entity_id and coalesce(c.ativo, true));
    select max(d) into latest_date from (
      select ultima_manutencao as d from public.air_conditioners where id::text = message.source_entity_id
      union all select data from public.service_history where aparelho_id::text = message.source_entity_id
      union all select coalesce(data_conclusao, data_agendamento) from public.services
        where aparelho_id::text = message.source_entity_id and status = 'CONCLUIDO'
    ) dates;
    valid := valid and latest_date is not null
      and coalesce(message.metadata->>'ultima_manutencao', '') = latest_date::text;
    if valid then
      -- Honor a technician's explicit return date on the latest service.
      with return_dates as (
        select data as d, substring(observacoes from '\[PROXIMO_RETORNO:([0-9]{4}-[0-9]{2}-[0-9]{2})\]') as due
          from public.service_history where aparelho_id::text = message.source_entity_id
        union all select coalesce(data_conclusao, data_agendamento),
          substring(observacoes from '\[PROXIMO_RETORNO:([0-9]{4}-[0-9]{2}-[0-9]{2})\]')
          from public.services where aparelho_id::text = message.source_entity_id and status = 'CONCLUIDO'
      )
      select not exists (select 1 from return_dates where d = latest_date and due is not null)
        or exists (select 1 from return_dates where d = latest_date and due = message.metadata->>'proxima_manutencao') into valid;
    end if;
  elsif message.event_type in ('lembrete_agendamento_vespera', 'lembrete_agendamento_uma_hora') then
    valid := exists (select 1 from public.services s join public.customers c on c.id = s.cliente_id
      where s.id::text = message.source_entity_id and s.status = 'AGENDADO' and coalesce(c.ativo, true)
        and s.data_agendamento::text = message.metadata->>'data_agendamento'
        and left(coalesce(s.hora_agendamento::text, ''), 5) = left(coalesce(message.metadata->>'hora_agendamento', ''), 5));
  end if;
  if not coalesce(valid, false) then
    update public.whatsapp_message_queue set status = 'cancelado', locked_until = null,
      message_text = '', last_error = 'Lembrete desatualizado: manutenção, cadastro ou agendamento alterado', updated_at = now()
    where id = message.id;
    return false;
  end if;
  return true;
end;
$$;
revoke all on function public.validate_whatsapp_reminder(uuid, integer) from public, anon, authenticated;
grant execute on function public.validate_whatsapp_reminder(uuid, integer) to service_role;
notify pgrst, 'reload schema';
