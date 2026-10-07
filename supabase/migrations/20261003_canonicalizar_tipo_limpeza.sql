-- Unifica o tipo legado da limpeza no valor canônico já usado pela aplicação.
-- Idempotente: pode ser aplicado novamente sem mudar os registros já corrigidos.
update public.services
set tipo = 'LIMPEZA'
where tipo = 'HIGIENIZACAO';
