-- Run server-side WhatsApp reminders from Supabase so the 15-minute cadence
-- works independently of Vercel Hobby Cron limits.
create extension if not exists pg_cron with schema pg_catalog;
create schema if not exists extensions;
create extension if not exists pg_net with schema extensions;

do $$
declare
  existing record;
begin
  for existing in
    select jobid
    from cron.job
    where jobname in ('inovar-whatsapp-alertas', 'inovar-whatsapp-agenda-1h')
  loop
    perform cron.unschedule(existing.jobid);
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
            select decrypted_secret
            from vault.decrypted_secrets
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
            select decrypted_secret
            from vault.decrypted_secrets
            where name = 'inovar_vercel_cron_secret'
          )
        ),
        body := '{}'::jsonb,
        timeout_milliseconds := 55000
      ) as request_id;
    $job$
  );
end $$;
