# InovarApp Desktop — Wails

O runner exibe a interface `internal/ui/webapp` em uma WebView e reutiliza o
mesmo servidor Go e os mesmos handlers de API. A UI, o CSS e o WebAssembly são
os mesmos do alvo web. O hook de pré-build do Wails compila e sincroniza o WASM
antes de gerar o executável.

## Build Windows

1. Execute `wails build` dentro de `cmd/web/desktop` para gerar o bundle.
2. Para compilar diretamente sem o CLI: execute `scripts/build-web.ps1` e depois `go build -o dist/InovarApp.exe ./cmd/web/desktop` na raiz do projeto.

Wails v2.16.0 hospeda a aplicação em WebView2. O bundle não inclui credenciais
Supabase administrativas; a autenticação do usuário e as políticas RLS seguem
sendo aplicadas pelas chamadas da UI.

## Recursos nativos

- APIs e interface Go rodam na mesma origem, com servidor limitado a `127.0.0.1`.
- OAuth abre no navegador do sistema e retorna por
  `inovarapp-desktop://oauth-callback/`. Inclua esse callback na allowlist do
  Supabase Auth. O build NSIS registra o protocolo no Windows; no macOS ele é
  registrado no `Info.plist` do bundle.
- Exportação de PDFs e backup usa o diálogo nativo para salvar; imagens e
  restauração continuam usando o seletor de arquivos da WebView.
- Avisos locais usam o serviço de notificações Wails; cópia usa o clipboard
  do runtime nativo como fallback.
- O instalador pode ser gerado com `wails build -nsis` quando o compilador NSIS
  (`makensis`) está instalado. O build portátil padrão gera o executável, mas
  não registra o protocolo OAuth no Windows.
- Sem NSIS, `scripts/package-desktop.ps1` gera um ZIP portátil contendo o
  executável e os scripts por usuário `install-desktop.ps1`/
  `uninstall-desktop.ps1`; instalar registra o protocolo OAuth no perfil atual,
  sem exigir privilégios de administrador. O pacote leva somente a URL e a
  chave anônima pública do Supabase; service-role e segredos de provedores não
  são incluídos.
  Extraia o ZIP e execute `powershell -ExecutionPolicy Bypass -File .\install-desktop.ps1`;
  para remover, execute o `uninstall-desktop.ps1` do próprio pacote.

O runtime Microsoft Edge WebView2 precisa estar disponível no Windows de
destino; Wails usa o instalador oficial por download se o runtime estiver
ausente.
