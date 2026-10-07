alter table public.budgets
  drop constraint if exists budgets_status_check;

alter table public.budgets
  add constraint budgets_status_check
  check (status = any (array[
    'RASCUNHO'::text,
    'ENVIADO'::text,
    'APROVADO'::text,
    'RECUSADO'::text,
    'EXPIRADO'::text,
    'CANCELADO'::text
  ]));
