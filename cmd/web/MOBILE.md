# Casca móvel Capacitor

Android e iOS reutilizam `internal/ui/webapp` exportada como WebAssembly e o
mesmo Core Go/API. `capacitor.config.ts` aponta para os arquivos estáticos
gerados em `mobile/www`; essa pasta não é fonte editável e fica fora do Git.

Antes do sync, configure `GOAPP_API_BASE_URL` como
`https://inovarapp.vercel.app/api/go` (ou o endereço HTTPS equivalente do
dispatcher Go implantado). A reescrita Vercel encaminha `/api/go/api/...` e
`/api/go/d/...` para a função Go preservando a rota original. A UI passa essa URL às chamadas autenticadas; a origem local
Capacitor nunca é usada como servidor de API. O backend Go libera CORS somente
para `https://localhost` (Android) e `capacitor://localhost` (iOS), mantendo
autorização JWT e RLS.

Antes de compilar/sincronizar, `scripts/build-mobile.ps1` consulta
`<GOAPP_API_BASE_URL>/api/whatsapp` e exige uma resposta JSON da API Go (o GET
deve ser recusado pelo handler como método não permitido). Se receber HTML, o
deploy ainda está servindo o frontend legado ou o dispatcher não está ativo; o
script interrompe a sincronização para não empacotar um app móvel sem backend
Go.

## OAuth nativo

Login Google, autorização de Google Contacts e Google Calendar abrem o navegador
seguro do sistema e retornam ao app por
`com.inovarapp.mobile://oauth-callback/`. `@capacitor/app` encaminha o deep link
à UI, que valida e processa os tokens no Core Go; no desktop/Web/PWA o redirect
web existente continua sendo usado. Cadastre o callback móvel em Supabase Auth →
URL Configuration → Additional Redirect URLs antes de testar login no dispositivo.
O redirect OAuth do provedor continua sendo o callback padrão do Supabase.

## Android

1. Instale Android Studio, SDK Platform 36 e JDK 21 ou superior.
2. Configure `GOAPP_API_BASE_URL` para o dispatcher Go HTTPS publicado; quando
   definido, o sync confirma que o endpoint responde à API Go antes do bundle.
3. Na raiz, execute `npm run mobile:android:debug` para compilar Go/WASM,
   sincronizar Capacitor e gerar `android/app/build/outputs/apk/debug/app-debug.apk`.
4. Abra o projeto no Android Studio com `npm run mobile:android` para executar
   em emulador/dispositivo ou gerar build assinado de publicação.

O appId e o esquema de callback são `com.inovarapp.mobile`. O APK inclui os
plugins App, Browser, Contacts e Push Notifications; o manifest mesclado declara
`READ_CONTACTS`, `POST_NOTIFICATIONS` e `INTERNET`, remove `WRITE_CONTACTS` e
registra o intent filter `oauth-callback`. Ao escolher “Puxar contato do celular”,
o app solicita leitura de contatos. Fotos e backups usam o seletor de arquivos do
Android via WebView; não pedem permissão ampla de armazenamento nem câmera.
OAuth real, permissões, deep link e seleção de arquivos devem ser aceitos em
dispositivo/emulador antes da publicação. O host Windows atual não tem AVD nem
dispositivo ADB conectado.

Para push Android, registre o app `com.inovarapp.mobile` no Firebase, mantenha o
`google-services.json` em `android/app/` (arquivo local de configuração do app)
e configure `FIREBASE_SERVICE_ACCOUNT_JSON` como segredo no backend Go. O backend
envia tokens Android pelo FCM HTTP v1 e remove registros que o Firebase marcar
como não registrados. Não coloque a chave de conta de serviço no app nem no Git.
Sem esse arquivo local do Firebase, o APK ainda compila, mas o registro/entrega
FCM real não pode ser validado.

## iOS Capacitor

O alvo iOS hospeda o bundle estático da mesma UI Go/WASM (`internal/ui/webapp`)
e usa a mesma API/Core Go remoto; Swift fica restrito ao ciclo de vida exigido
por UIKit/Capacitor. O esquema de origem é `capacitor://localhost`, o bundle ID
é `com.inovarapp.mobile` e o deployment target é iOS 15. O comando
`npm run mobile:ios` compila o Core para WASM, exporta a UI, liga a ponte de
plugins, sincroniza CocoaPods/SPM do Capacitor e verifica o projeto/entitlements.
`GOAPP_API_BASE_URL` pode ser omitido para preparar o bundle, mas, se definido,
deve ser HTTPS e o script verifica que aponta para o dispatcher Go.

O OAuth abre no navegador seguro e retorna pelo URL scheme customizado. Cold-start
e retomada passam pelos delegates UIKit/Capacitor; erros de OAuth recebidos no
callback mostram o fluxo de erro Go. Contatos usam o seletor nativo
`@capacitor/contacts` sem solicitar acesso amplo à agenda. Fotos e backups continuam
usando os seletores de arquivo do WebView, mantendo os mesmos limites/fluxos Go.
Push registra token APNs através de `@capacitor/push-notifications` e o entrega à
API Go; o entitlement `aps-environment` está declarado para Debug/Release.

Para compilar, abrir simulador, assinar, instalar e distribuir, use macOS com Xcode.
Cadastre `com.inovarapp.mobile://oauth-callback/` no Supabase Auth. Push real também
requer a chave `.p8`, Team ID, Key ID, entitlement ativo no App ID e perfil de
provisionamento correspondente. Esses serviços e o ciclo de assinatura/TestFlight
exigem uma conta Apple e não podem ser concluídos neste workspace Windows.
