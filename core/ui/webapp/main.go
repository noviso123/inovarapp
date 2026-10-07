package webapp

import (
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/maxence-charriere/go-app/v11/pkg/app"
	"inovarapp/core/adapter/alertcron"
	"inovarapp/core/adapter/cep"
	mailadapter "inovarapp/core/adapter/email"
	"inovarapp/core/adapter/googlecalendar"
	"inovarapp/core/adapter/googlecontacts"
	"inovarapp/core/adapter/httpapi"
	"inovarapp/core/adapter/supabase"
	"inovarapp/core/adapter/webpush"
	"inovarapp/core/adapter/whatsapp"
	"inovarapp/core/adapter/whatsappqueue"
	"inovarapp/core/domain"
)

type serviceCatalogPage struct {
	app.Compo

	email                             string
	password                          string
	confirmation                      string
	passwordVisible                   bool
	confirmationVisible               bool
	portalPasswordVisible             bool
	portalPasswordConfirmVisible      bool
	portalData                        *domain.CustomerPortalData
	portalLoading                     bool
	portalError                       string
	portalLoadedFor                   string
	portalBusinessWhats               string
	portalBusinessWhatsLoaded         string
	portalBusinessWhatsLoading        bool
	portalServiceStatuses             map[string]string
	offlineMode                       bool
	teamServices                      []map[string]any
	teamAppointments                  []map[string]any
	teamCustomers                     []map[string]any
	teamBudgets                       []domain.BudgetEstimate
	teamBudgetOpenID                  string
	teamBudgetForm                    *teamBudgetForm
	teamBudgetPrompt                  *teamBudgetPrompt
	teamBudgetSearch                  string
	teamBudgetFilter                  string
	teamBudgetMonth                   string
	teamFinanceMonth                  string
	teamFinanceAll                    bool
	teamFinanceMessage                string
	teamFinanceCopied                 bool
	notificationCenterOpen            bool
	notificationFilter                string
	notifications                     []localNotification
	notificationToastTitle            string
	notificationToastText             string
	teamBudgetMessage                 string
	teamBudgetWhatsAppURL             string
	teamBudgetBusy                    bool
	teamBudgetConfirm                 string
	teamBudgetScheduleID              string
	teamBudgetScheduleDate            string
	teamBudgetScheduleTime            string
	teamBudgetScheduleMessage         string
	teamReturnHistory                 []map[string]any
	teamDashboard                     teamDashboardSummary
	teamActiveSection                 string
	teamWhatsAppQueue                 []whatsAppQueueItem
	teamWhatsAppQueueCounts           map[string]int
	teamWhatsAppQueueLoading          bool
	teamWhatsAppQueueError            string
	teamWhatsAppQueueLoaded           bool
	teamWhatsAppQueueUpdatedAt        string
	teamWhatsAppQueueRefreshScheduled bool
	teamQRCodeMode                    string
	teamQRCodePhone                   string
	teamQRCodeMessage                 string
	teamQRCodeCustom                  string
	teamQRCodePayload                 string
	teamQRCodePNG                     string
	teamQRCodeSVG                     string
	teamQRCodeError                   string
	teamNavOpen                       bool
	teamQueueSection                  string
	teamQueueFilter                   string
	teamQueueSearch                   string
	teamQueueSelectedKey              string
	teamRequestFilter                 string
	teamRequestSearch                 string
	teamCustomerSearch                string
	teamCustomerExpandedID            string
	teamCustomerForm                  *teamCustomerForm
	teamCustomerReturnToBudget        bool
	teamCustomerTemporaryAccess       *teamCustomerTemporaryAccess
	teamCustomerAccessCopied          bool
	teamCustomerAccessNotice          string
	teamApplianceForm                 *teamApplianceForm
	teamPhotoApplianceID              string
	teamAppliancePhotos               []appliancePhoto
	teamPhotoNotice                   string
	teamPhotoBusy                     bool
	teamHistoryApplianceID            string
	teamHistoryForm                   *teamHistoryForm
	teamHistoryRows                   []map[string]any
	teamHistoryLegacyRows             []map[string]any
	teamHistoryNotice                 string
	teamServiceDetail                 map[string]any
	teamServiceReceiptEdit            bool
	teamServiceDeleteNotice           string
	teamServicePhotoID                string
	teamServicePhotos                 []appliancePhoto
	teamServicePhotoBusy              bool
	teamServicePhotoNotice            string
	teamServiceWhatsAppURL            string
	teamPDFBusy                       bool
	teamPDFNotice                     string
	teamServiceEdit                   *teamServiceEditForm
	teamCalendarConnected             bool
	teamCalendarLastSync              string
	teamCalendarBusy                  bool
	teamCalendarNotice                string
	authLifecycleStop                 func()
	notificationLifecycleStop         func()
	teamAgendaMonth                   string
	teamAgendaView                    string
	teamAgendaStatus                  string
	teamAgendaWeek                    string
	pushBusy                          bool
	pushChecking                      bool
	pushEnabled                       bool
	pushNotice                        string
	localNotificationsEnabled         bool
	locationBusy                      bool
	locationCoordinates               string
	locationAccuracy                  string
	locationNotice                    string
	teamCustomerNotice                string
	teamCustomerConfirm               string
	teamCustomerActionSaving          bool
	teamLoading                       bool
	teamMutationRetryScheduled        bool
	teamError                         string
	teamLoadedFor                     string
	teamScheduleServiceID             string
	teamScheduleDate                  string
	teamScheduleTime                  string
	teamCallScheduleServiceID         string
	teamCallScheduleDate              string
	teamCallScheduleTime              string
	teamCallScheduleNotes             string
	teamCallScheduleMessage           string
	teamScheduleSaving                bool
	teamNewAppointment                *teamAppointmentForm
	teamNewAppointmentSaving          bool
	teamNewAppointmentNotice          string
	teamDirectServiceForm             *teamDirectServiceForm
	teamDirectServiceSaving           bool
	teamStartServiceForm              *teamStartServiceForm
	teamStartServiceSaving            bool
	teamCancelServiceID               string
	teamCancelSaving                  bool
	teamActionMessage                 string
	teamProfile                       domain.TechnicianProfile
	teamCatalogForm                   *teamCatalogForm
	teamCatalogSaving                 bool
	teamCatalogNotice                 string
	teamCatalogConfirmKey             string
	teamSettingsOpen                  bool
	teamSettingsForm                  *teamSettingsForm
	teamSettingsSaving                bool
	teamSettingsNotice                string
	teamEmailUsername                 string
	teamEmailPassword                 string
	teamEmailPasswordSaved            bool
	teamEmailGoogleConnected          bool
	teamEmailGoogleReady              bool
	teamEmailGoogleAccount            string
	teamEmailBusy                     bool
	teamEmailNotice                   string
	teamWhatsAppStatus                map[string]any
	teamWhatsAppQR                    string
	teamWhatsAppPairing               string
	teamWhatsAppPhone                 string
	teamWhatsAppBusy                  bool
	teamWhatsAppNotice                string
	teamWhatsAppConfirm               string
	teamWhatsAppMetaExpanded          bool
	teamWhatsAppGeneration            uint64
	teamWhatsAppSaved                 bool
	teamMessageKey                    string
	teamMessageDraft                  string
	teamMessageInterval               string
	teamMessageSaving                 bool
	teamMessageNotice                 string
	teamCompletion                    *teamCompletionForm
	teamReceiptURL                    string
	portalRequestType                 string
	portalRequestProblem              string
	portalRequestNotes                string
	portalRequestDate                 string
	portalRequestAppliance            string
	portalRequestBusy                 bool
	portalRequestFeedback             string
	portalRequestOpen                 bool
	portalProfilePhotoURL             string
	teamProfilePhotoURL               string
	teamProfilePhotoLoaded            string
	teamProfilePhotoBusy              bool
	teamProfilePhotoError             string
	teamProfilePhotoConfirm           bool
	portalProfilePhotoBusy            bool
	portalProfilePhotoError           string
	portalProfilePhotoNotice          string
	portalProfilePhotoLoaded          string
	portalProfilePhotoConfirm         bool
	portalProfileOpen                 bool
	portalProfileForm                 customerProfileForm
	portalProfileBusy                 bool
	portalProfileNotice               string
	portalProfileSaved                bool
	portalProfileCEPLooking           bool
	portalProfileCEPGeneration        uint64
	portalProfileCEPMessage           string
	portalProfileCEPSuggestion        *cep.Address
	portalPasswordOpen                bool
	portalPassword                    string
	portalPasswordConfirm             string
	portalPasswordBusy                bool
	portalPasswordError               string
	portalPasswordNotice              string
	portalApplianceOpen               bool
	portalApplianceBrand              string
	portalApplianceModel              string
	portalApplianceBTUs               string
	portalApplianceType               string
	portalApplianceRoom               string
	portalApplianceBusy               bool
	portalApplianceFeedback           string
	portalApplianceSaved              bool
	budgetToSign                      map[string]any
	budgetSignature                   string
	budgetBusy                        bool
	budgetFeedback                    string
	budgetDecline                     map[string]any
	fullName                          string
	whatsapp                          string
	address                           string
	neighborhood                      string
	city                              string
	postalCode                        string
	signupPostalLoading               bool
	signupPostalGeneration            uint64
	signupPostalMessage               string
	signupPostalSuggestion            *cep.Address
	authMode                          string
	authOpen                          bool
	recoveryOpen                      bool
	recoveryRequired                  bool
	recoveryDone                      bool
	authBusy                          bool
	refreshingSession                 bool
	authError                         string
	authSuccess                       string
	authNotice                        string
	session                           *supabase.AuthSession
	caller                            *supabase.Caller
}

func (p *serviceCatalogPage) Render() app.UI {
	isTeam := p.caller != nil && (p.caller.Role == domain.RoleTechnician || p.caller.Role == domain.RoleAdmin)
	headerClass := "topbar"
	rootClass := "inovar-app"
	if p.caller == nil {
		headerClass += " topbar--guest"
		rootClass += " inovar-app--guest"
	}
	if isTeam {
		headerClass += " topbar--team"
		rootClass += " inovar-app--team"
	}
	headerItems := []app.UI{}
	brand := app.Div().Class("brand").Body(
		app.Img().Src("/web/icon-192.png").Alt("InovarApp").Class("brand__mark"),
		app.Div().Body(
			app.P().Class("brand__name").Body(app.Text("INOVAR APP")),
			app.P().Class("brand__subtitle").Body(app.Text("REFRIGERAÇÃO & CLIMATIZAÇÃO")),
		),
	)
	if p.caller == nil {
		headerItems = append(headerItems, brand)
	} else {
		headerItems = append(headerItems, brand, app.Div().Class("topbar__actions").Body(p.notificationCenter(), p.accountControl()))
	}
	if isTeam {
		headerItems = append(headerItems, p.teamWorkspaceNavigation())
	}
	header := app.Header().Class(headerClass).Body(headerItems...)

	workspace := make([]app.UI, 0, 3)
	if p.offlineMode {
		workspace = append(workspace, app.Div().Class("auth-notice").Body(app.Text("Sem conexão: mostrando os últimos dados salvos desta conta. Cadastros, edições/exclusões de clientes, aparelhos, ordens, agenda, orçamentos e ajustes de perfil/mensagens compatíveis ficam na fila e sincronizam ao reconectar; anexos, contas de acesso e integrações externas precisam de internet.")))
	}
	if p.authNotice != "" {
		workspace = append(workspace, app.Div().Class("auth-notice").Body(app.Text(p.authNotice)))
	}
	if !isTeam {
		if p.caller == nil {
			workspace = append(workspace, app.Section().Class("guest-welcome").Body(
				app.Div().Class("guest-welcome__brand").Body(
					app.Img().Src("/web/icon-192.png").Alt("InovarApp").Class("guest-welcome__mark"),
					app.Div().Body(app.Strong().Body(app.Text("INOVAR")), app.Span().Body(app.Text("REFRIGERAÇÃO"))),
				),
				app.H1().Class("guest-welcome__title").Body(app.Text("Bem-vindo ao InovarApp")),
				app.P().Class("guest-welcome__description").Body(app.Text("Acompanhe seus aparelhos, solicite atendimentos e consulte garantias. Clientes e técnicos entram com o mesmo login usado no site da Inovar.")),
				app.Button().Class("guest-welcome__primary").Type("button").OnClick(p.openAuth).Body(app.Span().Body(app.Text("↪")), app.Text("Entrar no App")),
				app.Button().Class("guest-welcome__secondary").Type("button").OnClick(p.openSignup).Body(app.Text("Sou Cliente • Criar Minha Conta")),
				app.Div().Class("guest-welcome__features").Body(
					app.Div().Body(app.Span().Class("guest-welcome__feature-icon guest-welcome__feature-icon--safe").Body(app.Text("♢")), app.Span().Body(app.Text("Dados protegidos"))),
					app.Div().Body(app.Span().Class("guest-welcome__feature-icon guest-welcome__feature-icon--service").Body(app.Text("≋")), app.Span().Body(app.Text("Todos os serviços"))),
					app.Div().Body(app.Span().Class("guest-welcome__feature-icon guest-welcome__feature-icon--sync").Body(app.Text("▯")), app.Span().Body(app.Text("Site + App sincronizados"))),
				),
			))
		}
	}
	if p.caller != nil && p.caller.Role == domain.RoleCustomer {
		workspace = append(workspace, p.customerPortalSection())
	}
	if p.caller != nil && (p.caller.Role == domain.RoleTechnician || p.caller.Role == domain.RoleAdmin) {
		workspace = append(workspace, p.teamOperationsSection())
	}
	body := []app.UI{header, app.Main().Class("workspace").Body(workspace...)}
	if p.teamCustomerForm != nil {
		body = append(body, p.teamCustomerDialog())
	}
	if p.teamApplianceForm != nil {
		body = append(body, p.teamApplianceDialog())
	}
	if p.teamCustomerConfirm != "" {
		body = append(body, teamCustomerActionDialog(p, p.teamCustomerConfirm))
	}
	if p.teamCustomerTemporaryAccess != nil {
		body = append(body, p.teamCustomerTemporaryAccessDialog())
	}
	if p.teamScheduleServiceID != "" {
		body = append(body, p.teamScheduleDialog())
	}
	if p.teamCancelServiceID != "" {
		body = append(body, p.teamCancellationDialog())
	}
	if p.notificationToastTitle != "" {
		body = append(body, p.notificationToast())
	}
	if p.recoveryOpen {
		body = append(body, p.passwordRecoveryModal())
	}
	if p.authOpen {
		body = append(body, p.authModal())
	}
	if p.teamHistoryForm != nil {
		body = append(body, p.teamHistoryDialog())
	}
	if p.teamServiceDetail != nil {
		body = append(body, p.teamServiceDetailDialog())
	}
	if p.teamSettingsOpen {
		body = append(body, p.teamSettingsDialog())
	}
	if p.teamBudgetConfirm != "" {
		body = append(body, p.teamBudgetConfirmDialog())
	}
	if p.teamBudgetPrompt != nil {
		body = append(body, p.teamBudgetPromptDialog())
	}
	if p.teamProfilePhotoConfirm {
		body = append(body, p.teamProfilePhotoRemovalDialog())
	}
	return app.Div().Class(rootClass).Body(body...)
}

func registerServerHandlers() http.Handler {
	registerAppRoutes()
	mux := http.NewServeMux()
	registerAPIHandlers(mux)
	mux.HandleFunc("/downloads", func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, "/downloads/", http.StatusTemporaryRedirect)
	})
	mux.Handle("/downloads/", downloadsFilesHandler())
	mux.Handle("/", newAppHandler())
	return mobileCORSMiddleware(mux)
}

// NewAPIHandler builds the Go HTTP API without the HTML/WASM shell, allowing
// serverless hosts to expose the same handlers independently of the UI.
func NewAPIHandler() http.Handler {
	mux := http.NewServeMux()
	registerAPIHandlers(mux)
	return mobileCORSMiddleware(mux)
}

func registerAPIHandlers(mux *http.ServeMux) {
	accountsClient, _ := serverSupabaseClient()
	webPushSender := webpush.Sender{PublicKey: configuredValue("VAPID_PUBLIC_KEY"), PrivateKey: configuredValue("VAPID_PRIVATE_KEY"), Subject: configuredValue("VAPID_SUBJECT")}
	androidPushSender, iosPushSender := configuredNativePushSenders()
	teamPushNotifier := httpapi.TeamPushNotifier{Supabase: accountsClient, WebSender: webPushSender, AndroidSender: androidPushSender, IOSSender: iosPushSender}
	mux.Handle("/api/google-contacts", googlecontacts.Handler{Supabase: accountsClient})
	mux.Handle("/api/google-calendar", googlecalendar.Handler{
		Supabase:     accountsClient,
		ClientID:     configuredValue("GOOGLE_CLIENT_ID"),
		ClientSecret: configuredValue("GOOGLE_CLIENT_SECRET"),
	})
	mux.Handle("/api/google-email", httpapi.GoogleEmailHandler{
		Supabase:     accountsClient,
		ClientID:     configuredValue("GOOGLE_CLIENT_ID"),
		ClientSecret: configuredValue("GOOGLE_CLIENT_SECRET"),
	})
	mux.Handle("/api/contas", httpapi.AccountsHandler{
		Supabase:         accountsClient,
		WhatsApp:         whatsapp.Sender{},
		WhatsAppDefaults: serverWhatsAppConfig(),
	})
	mux.Handle("/api/orcamento-resposta", httpapi.BudgetResponseHandler{
		Supabase:         accountsClient,
		Notifier:         teamPushNotifier,
		WhatsApp:         whatsapp.Sender{},
		WhatsAppDefaults: serverWhatsAppConfig(),
	})
	mux.Handle("/api/configuracoes", httpapi.SettingsHandler{
		Supabase:    accountsClient,
		EmailAPIKey: configuredValue("EMAIL_API_KEY"),
		EmailFrom:   configuredValue("EMAIL_FROM"),
	})
	mux.Handle("/api/backup", httpapi.BackupHandler{Supabase: accountsClient})
	mux.Handle("/api/email", httpapi.EmailHandler{Supabase: accountsClient, Sender: mailadapter.ConfiguredSender{}, Defaults: mailadapter.Config{
		Username: configuredValue("EMAIL_GMAIL_USER"), Password: configuredValue("EMAIL_GMAIL_PASS"),
		GoogleClientID: configuredValue("GOOGLE_CLIENT_ID"), GoogleClientSecret: configuredValue("GOOGLE_CLIENT_SECRET"),
	}})
	cronHandler := alertcron.Handler{
		Supabase: accountsClient, Secret: configuredValue("CRON_SECRET"),
		Months: configuredInt("ALERTA_MESES", 3), MaxLateDays: configuredInt("ALERTA_MAX_ATRASO_DIAS", 180),
		WhatsAppDefaults: serverWhatsAppConfig(),
		QueueSender:      whatsapp.Sender{},
		EmailDefaults:    mailadapter.Config{APIKey: configuredValue("EMAIL_API_KEY"), From: configuredValue("EMAIL_FROM"), Username: configuredValue("EMAIL_GMAIL_USER"), Password: configuredValue("EMAIL_GMAIL_PASS"), GoogleClientID: configuredValue("GOOGLE_CLIENT_ID"), GoogleClientSecret: configuredValue("GOOGLE_CLIENT_SECRET")},
		PushDefaults:     webpush.Sender{PublicKey: configuredValue("VAPID_PUBLIC_KEY"), PrivateKey: configuredValue("VAPID_PRIVATE_KEY"), Subject: configuredValue("VAPID_SUBJECT")},
	}
	mux.Handle("/api/cron/alertas", cronHandler)
	mux.Handle("/api/cron/agenda-uma-hora", cronHandler)
	mux.Handle("/api/cron/whatsapp-fila", cronHandler)
	mux.Handle("/api/portal/cliente", httpapi.CustomerPortalHandler{Supabase: accountsClient})
	mux.Handle("/api/cliente/perfil", httpapi.CustomerProfileHandler{Supabase: accountsClient})
	mux.Handle("/api/cliente/aparelhos", httpapi.CustomerAppliancesHandler{Supabase: accountsClient})
	mux.Handle("/api/servicos", httpapi.ServicesHandler{Supabase: accountsClient, ScheduleMaintenance: cronHandler.SyncMaintenance})
	mux.Handle("/api/clientes", httpapi.CustomersHandler{Supabase: accountsClient})
	mux.Handle("/api/orcamentos", httpapi.TeamBudgetsHandler{Supabase: accountsClient})
	mux.Handle("/api/orcamento-equipe", httpapi.TeamBudgetMutationHandler{Supabase: accountsClient})
	mux.Handle("/api/aparelhos", httpapi.TeamAppliancesHandler{Supabase: accountsClient})
	mux.Handle("/api/cliente/servicos", httpapi.CustomerServicesHandler{Supabase: accountsClient, Notifier: teamPushNotifier})
	mux.Handle("/api/agendamentos", httpapi.AppointmentsHandler{Supabase: accountsClient})
	mux.Handle("/api/historico", httpapi.ServiceHistoryHandler{Supabase: accountsClient, ScheduleMaintenance: cronHandler.SyncMaintenance})
	mux.Handle("/api/aparelho-manutencao", httpapi.ApplianceMaintenanceHandler{Supabase: accountsClient, ScheduleMaintenance: cronHandler.SyncMaintenance})
	mux.Handle("/api/os-pdf", httpapi.ServiceOrderPDFHandler{
		Supabase: accountsClient, WhatsApp: whatsapp.Sender{}, WhatsAppDefaults: serverWhatsAppConfig(),
	})
	mux.Handle("/api/orcamento-pdf", httpapi.BudgetPDFHandler{Supabase: accountsClient})
	mux.Handle("/api/orcamento-whatsapp", httpapi.BudgetWhatsAppHandler{Supabase: accountsClient, Sender: whatsapp.Sender{}, Defaults: serverWhatsAppConfig()})
	mux.Handle("/api/documentos", httpapi.DocumentsHandler{Supabase: accountsClient})
	mux.Handle("/api/notificacoes", httpapi.NotificationsHandler{Supabase: accountsClient, AndroidSender: androidPushSender, IOSSender: iosPushSender, Sender: webPushSender})
	mux.Handle("/api/whatsapp", httpapi.WhatsAppHandler{Supabase: accountsClient, Sender: whatsapp.Sender{}, Defaults: serverWhatsAppConfig()})
	mux.Handle("/api/whatsapp/fila", whatsappqueue.HTTPHandler{Supabase: accountsClient, SyncMaintenance: cronHandler.SyncMaintenance})
	mux.Handle("/d/", httpapi.ShortDocumentHandler{Supabase: accountsClient})
	mux.HandleFunc("/api/calendario-ics", func(w http.ResponseWriter, r *http.Request) {
		client, err := serverSupabaseClient()
		if err != nil {
			http.Error(w, "Serviço de calendário indisponível", http.StatusInternalServerError)
			return
		}
		client.ServeCalendarFeed(w, r)
	})

}

func serverListenAddress() string {
	port := strings.TrimSpace(configuredValue("PORT"))
	if port == "" {
		return ":8080"
	}
	if strings.HasPrefix(port, ":") {
		return port
	}
	return ":" + port
}

func registerAppRoutes() {
	app.Route("/", func() app.Composer { return &serviceCatalogPage{} })
}

// RunBrowser initializes the shared go-app routes and starts the browser-side
// WebAssembly application. Native servers use RunServer instead.
func RunBrowser() {
	registerAppRoutes()
	app.RunWhenOnBrowser()
}

func newAppHandler() *app.Handler {
	appVersion := frontendAssetVersion(webRoot(), configuredValue("APP_VERSION"))
	resources := newPWAResources(webRoot(), appVersion)
	return &app.Handler{
		Name:      "InovarApp • Inovar Refrigeração",
		ShortName: "InovarApp",
		Icon: app.Icon{
			Default:  "/web/icon-192.png",
			Large:    "/web/icon-512.png",
			SVG:      "/web/favicon.svg",
			Maskable: "/web/icon-512.png",
		},
		Title:              "InovarApp • Inovar Refrigeração",
		Description:        "Portal técnico e do cliente da Inovar: serviços, orçamentos com assinatura digital, histórico e garantias.",
		Author:             "Inovar Refrigeração",
		Lang:               "pt-BR",
		ThemeColor:         "#0B2D4E",
		BackgroundColor:    "#0b1329",
		LoadingLabel:       "Carregando InovarApp…",
		WasmContentLength:  webAssemblyContentLength(),
		Styles:             []string{resources.Resolve("/web/inovar.css")},
		Scripts:            []string{"/web/clean-auth-query.js"},
		CacheableResources: []string{"/web/apple-touch-icon.png", "/web/favicon.svg", resources.Resolve("/web/app.wasm")},
		Version:            appVersion,
		StartURL:           "/",
		Resources:          resources,
		Env:                publicSupabaseEnvironment(),
	}
}

func webAssemblyContentLength() string {
	info, err := os.Stat(filepath.Join(webRoot(), "web", "app.wasm"))
	if err != nil || info.Size() <= 0 {
		return ""
	}
	return strconv.FormatInt(info.Size(), 10)
}
