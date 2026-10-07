-- Keep only the last three days of successfully accepted WhatsApp messages in
-- the operational outbox. Failed and pending messages remain available for
-- support and retries. Deleting the queue row also removes its attempt records
-- through the existing ON DELETE CASCADE foreign key.
create index if not exists whatsapp_queue_sent_cleanup_idx
  on public.whatsapp_message_queue (sent_at)
  where status = 'enviado' and sent_at is not null;

create or replace function public.purge_sent_whatsapp_message_queue()
returns integer
language plpgsql
security definer
set search_path = pg_catalog, public
as $$
declare
  deleted_count integer;
begin
  delete from public.whatsapp_message_queue
   where status = 'enviado'
     and sent_at is not null
     and sent_at < statement_timestamp() - interval '3 days';

  get diagnostics deleted_count = row_count;
  return deleted_count;
end;
$$;

revoke all on function public.purge_sent_whatsapp_message_queue() from public, anon, authenticated;
grant execute on function public.purge_sent_whatsapp_message_queue() to service_role;

-- Run once per day. Messages are retained for at least three days; the daily
-- sweep removes them shortly after they cross that retention window.
do $$
declare
  existing_job record;
begin
  for existing_job in
    select jobid from cron.job where jobname = 'inovar-whatsapp-fila-limpeza'
  loop
    perform cron.unschedule(existing_job.jobid);
  end loop;

  perform cron.schedule(
    'inovar-whatsapp-fila-limpeza',
    '17 3 * * *',
    'select public.purge_sent_whatsapp_message_queue();'
  );
end;
$$;
