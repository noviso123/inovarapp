-- Durable WhatsApp outbox for automated reminders and user-triggered messages.
-- Apply this migration before enabling the new queue APIs in production.
create table if not exists public.whatsapp_message_queue (
  id uuid primary key default gen_random_uuid(),
  idempotency_key text not null unique,
  event_type text not null,
  recipient_phone text not null,
  message_text text not null default '',
  document_url text,
  document_storage_path text,
  document_name text,
  scheduled_at timestamptz not null default now(),
  next_attempt_at timestamptz not null default now(),
  expires_at timestamptz,
  status text not null default 'pendente'
    check (status in ('pendente', 'processando', 'enviado', 'falha', 'cancelado', 'expirado')),
  attempt_count integer not null default 0 check (attempt_count >= 0),
  locked_until timestamptz,
  last_error text,
  sensitive boolean not null default false,
  source_entity_type text,
  source_entity_id text,
  metadata jsonb not null default '{}'::jsonb,
  created_at timestamptz not null default now(),
  updated_at timestamptz not null default now(),
  sent_at timestamptz
);

create index if not exists whatsapp_queue_due_idx
  on public.whatsapp_message_queue (next_attempt_at, scheduled_at)
  where status = 'pendente';
create index if not exists whatsapp_queue_status_created_idx
  on public.whatsapp_message_queue (status, created_at desc);
create index if not exists whatsapp_queue_source_idx
  on public.whatsapp_message_queue (source_entity_type, source_entity_id);

create table if not exists public.whatsapp_message_attempts (
  id uuid primary key default gen_random_uuid(),
  message_id uuid not null references public.whatsapp_message_queue(id) on delete cascade,
  attempt_number integer not null,
  status text not null check (status in ('enviado', 'falha')),
  error_message text,
  started_at timestamptz not null,
  finished_at timestamptz not null,
  created_at timestamptz not null default now()
);
create index if not exists whatsapp_attempts_message_idx
  on public.whatsapp_message_attempts (message_id, attempt_number desc);
create unique index if not exists whatsapp_attempts_message_attempt_unique
  on public.whatsapp_message_attempts (message_id, attempt_number);

alter table public.whatsapp_message_queue enable row level security;
alter table public.whatsapp_message_attempts enable row level security;

drop policy if exists "team can read WhatsApp queue" on public.whatsapp_message_queue;
create policy "team can read WhatsApp queue"
  on public.whatsapp_message_queue for select to authenticated
  using (exists (
    select 1 from public.profiles p
    where p.id = auth.uid() and p.tipo in ('TECNICO', 'ADMIN')
  ));

drop policy if exists "team can read WhatsApp attempts" on public.whatsapp_message_attempts;
create policy "team can read WhatsApp attempts"
  on public.whatsapp_message_attempts for select to authenticated
  using (exists (
    select 1 from public.profiles p
    where p.id = auth.uid() and p.tipo in ('TECNICO', 'ADMIN')
  ));

revoke all on public.whatsapp_message_queue from anon, authenticated;
revoke all on public.whatsapp_message_attempts from anon, authenticated;
grant select on public.whatsapp_message_queue to authenticated;
grant select on public.whatsapp_message_attempts to authenticated;
grant all on public.whatsapp_message_queue to service_role;
grant all on public.whatsapp_message_attempts to service_role;

create or replace function public.claim_whatsapp_message_queue(batch_size integer default 25, lease_seconds integer default 150)
returns setof public.whatsapp_message_queue
language plpgsql
security definer
set search_path = pg_catalog, public, auth
as $$
begin
  if coalesce(auth.role(), '') <> 'service_role' then
    raise exception 'service role required';
  end if;

  update public.whatsapp_message_queue
     set status = 'expirado', message_text = '',
         last_error = 'mensagem expirada antes do envio', updated_at = now(), locked_until = null
   where status = 'pendente' and expires_at is not null and expires_at <= now();

  update public.whatsapp_message_queue
     set status = 'pendente', locked_until = null,
         next_attempt_at = least(next_attempt_at, now()),
         last_error = coalesce(last_error, 'tentativa anterior interrompida; será retomada'),
         updated_at = now()
   where status = 'processando' and locked_until is not null and locked_until <= now();

  return query
  with candidates as (
    select q.id
      from public.whatsapp_message_queue q
     where q.status = 'pendente'
       and q.next_attempt_at <= now()
       and q.scheduled_at <= now()
       and (q.locked_until is null or q.locked_until <= now())
       and (q.expires_at is null or q.expires_at > now())
     order by q.next_attempt_at, q.scheduled_at, q.created_at
     for update skip locked
     limit greatest(1, least(batch_size, 100))
  )
  update public.whatsapp_message_queue q
     set status = 'processando', locked_until = now() + make_interval(secs => greatest(30, least(lease_seconds, 600))),
         attempt_count = q.attempt_count + 1, updated_at = now()
    from candidates c
   where q.id = c.id
  returning q.*;
end;
$$;

revoke all on function public.claim_whatsapp_message_queue(integer, integer) from public, anon, authenticated;
grant execute on function public.claim_whatsapp_message_queue(integer, integer) to service_role;

create or replace function public.whatsapp_message_queue_counts()
returns jsonb
language plpgsql
security definer
set search_path = pg_catalog, public, auth
as $$
declare result jsonb;
begin
  if coalesce(auth.role(), '') <> 'service_role' then
    raise exception 'service role required';
  end if;
  select coalesce(jsonb_object_agg(status, total), '{}'::jsonb)
    into result
    from (
      select status, count(*)::integer as total
        from public.whatsapp_message_queue
       group by status
    ) counts;
  return result;
end;
$$;
revoke all on function public.whatsapp_message_queue_counts() from public, anon, authenticated;
grant execute on function public.whatsapp_message_queue_counts() to service_role;

-- Record provider outcome, delivery attempt, and retry schedule atomically.
create or replace function public.finish_whatsapp_message_attempt(
  p_message_id uuid,
  p_attempt_number integer,
  p_attempt_status text,
  p_error_message text,
  p_started_at timestamptz,
  p_finished_at timestamptz,
  p_retry_at timestamptz default null
)
returns text
language plpgsql
security definer
set search_path = pg_catalog, public, auth
as $$
declare
  message_row public.whatsapp_message_queue%rowtype;
  final_status text;
begin
  if coalesce(auth.role(), '') <> 'service_role' then
    raise exception 'service role required';
  end if;
  if p_attempt_status not in ('enviado', 'falha') then
    raise exception 'invalid attempt status';
  end if;

  select * into message_row
    from public.whatsapp_message_queue
   where id = p_message_id
   for update;
  if not found or message_row.status <> 'processando' then
    return 'stale';
  end if;

  if p_attempt_status = 'enviado' then
    final_status := 'enviado';
  elsif message_row.expires_at is not null
        and (message_row.expires_at <= p_finished_at or p_retry_at is null or p_retry_at >= message_row.expires_at) then
    final_status := 'expirado';
  else
    final_status := 'pendente';
  end if;

  insert into public.whatsapp_message_attempts (
    message_id, attempt_number, status, error_message, started_at, finished_at
  ) values (
    p_message_id, p_attempt_number, p_attempt_status, nullif(p_error_message, ''), p_started_at, p_finished_at
  ) on conflict (message_id, attempt_number) do nothing;

  update public.whatsapp_message_queue
     set status = final_status,
         sent_at = case when final_status = 'enviado' then p_finished_at else sent_at end,
         next_attempt_at = case when final_status = 'pendente' then p_retry_at else next_attempt_at end,
         locked_until = null,
         last_error = case
           when final_status = 'enviado' then null
           when final_status = 'expirado' then coalesce(nullif(p_error_message, ''), 'mensagem expirada antes do envio')
           else nullif(p_error_message, '')
         end,
         message_text = case when final_status = 'enviado' and message_row.sensitive or final_status = 'expirado' then '' else message_text end,
         updated_at = p_finished_at
   where id = p_message_id;
  return final_status;
end;
$$;
revoke all on function public.finish_whatsapp_message_attempt(uuid, integer, text, text, timestamptz, timestamptz, timestamptz) from public, anon, authenticated;
grant execute on function public.finish_whatsapp_message_attempt(uuid, integer, text, text, timestamptz, timestamptz, timestamptz) to service_role;

-- The database scheduler wakes Vercel every minute. Leases and SKIP LOCKED
-- protect against overlapping invocations and recover interrupted work.
create extension if not exists pg_cron with schema pg_catalog;
create schema if not exists extensions;
create extension if not exists pg_net with schema extensions;

do $$
declare
  old_job record;
begin
  if not exists (
    select 1 from vault.decrypted_secrets where name = 'inovar_vercel_cron_secret'
  ) then
    raise exception 'Configure o secret Vault inovar_vercel_cron_secret com o mesmo valor de CRON_SECRET no Vercel antes de habilitar os crons';
  end if;

  for old_job in
    select jobid from cron.job
    where jobname in ('inovar-whatsapp-fila', 'inovar-whatsapp-alertas', 'inovar-whatsapp-agenda-1h')
  loop
    perform cron.unschedule(old_job.jobid);
  end loop;

  perform cron.schedule(
    'inovar-whatsapp-alertas',
    '*/15 * * * *',
    $job$
      select net.http_post(
        url := 'https://inovarapp.vercel.app/api/cron/alertas',
        headers := jsonb_build_object(
          'Content-Type', 'application/json',
          'Authorization', 'Bearer ' || (
            select decrypted_secret from vault.decrypted_secrets
            where name = 'inovar_vercel_cron_secret'
          )
        ),
        body := '{}'::jsonb,
        timeout_milliseconds := 55000
      ) as request_id;
    $job$
  );

  perform cron.schedule(
    'inovar-whatsapp-agenda-1h',
    '7,22,37,52 * * * *',
    $job$
      select net.http_post(
        url := 'https://inovarapp.vercel.app/api/cron/agenda-uma-hora',
        headers := jsonb_build_object(
          'Content-Type', 'application/json',
          'Authorization', 'Bearer ' || (
            select decrypted_secret from vault.decrypted_secrets
            where name = 'inovar_vercel_cron_secret'
          )
        ),
        body := '{}'::jsonb,
        timeout_milliseconds := 55000
      ) as request_id;
    $job$
  );

  perform cron.schedule(
    'inovar-whatsapp-fila',
    '* * * * *',
    $job$
      select net.http_post(
        url := 'https://inovarapp.vercel.app/api/cron/whatsapp-fila',
        headers := jsonb_build_object(
          'Content-Type', 'application/json',
          'Authorization', 'Bearer ' || (
            select decrypted_secret from vault.decrypted_secrets
            where name = 'inovar_vercel_cron_secret'
          )
        ),
        body := '{}'::jsonb,
        timeout_milliseconds := 55000
      ) as request_id;
    $job$
  );
end $$;
