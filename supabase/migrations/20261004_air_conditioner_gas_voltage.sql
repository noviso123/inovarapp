-- Preserve the equipment fields already supported by the React client model.
-- Defaults match the legacy form and keep existing rows fully readable.
alter table public.air_conditioners
  add column if not exists gas_tipo text not null default 'R-410A',
  add column if not exists tensao text not null default '220V';

alter table public.air_conditioners
  drop constraint if exists air_conditioners_gas_tipo_check,
  add constraint air_conditioners_gas_tipo_check
    check (gas_tipo in ('R-410A', 'R-32', 'R-22', 'Outro')),
  drop constraint if exists air_conditioners_tensao_check,
  add constraint air_conditioners_tensao_check
    check (tensao in ('220V', '110V', 'Bivolt'));
