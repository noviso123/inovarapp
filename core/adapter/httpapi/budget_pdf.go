package httpapi

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strings"

	"github.com/google/uuid"
	"inovarapp/core/adapter/pdf"
	"inovarapp/core/adapter/supabase"
	"inovarapp/core/domain"
)

type BudgetPDFHandler struct{ Supabase *supabase.Client }

type budgetPDFRequest struct {
	BudgetID string `json:"budget_id"`
}

func (h BudgetPDFHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeResourceJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "Método não permitido"})
		return
	}
	if h.Supabase == nil {
		writeResourceJSON(w, http.StatusInternalServerError, map[string]string{"error": "Backend não configurado"})
		return
	}
	caller, err := h.Supabase.AuthenticateCaller(r.Context(), supabase.BearerToken(r.Header.Get("Authorization")))
	if err != nil {
		writeResourceJSON(w, http.StatusUnauthorized, map[string]string{"error": "Não autenticado"})
		return
	}
	if !isTeamRole(caller.Role) {
		writeResourceJSON(w, http.StatusForbidden, map[string]string{"error": "Somente a equipe Inovar pode gerar o PDF do orçamento"})
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, 32<<10)
	var request budgetPDFRequest
	if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
		writeResourceJSON(w, http.StatusBadRequest, map[string]string{"error": "Informe um orçamento válido"})
		return
	}
	if _, err := uuid.Parse(strings.TrimSpace(request.BudgetID)); err != nil {
		writeResourceJSON(w, http.StatusBadRequest, map[string]string{"error": "Informe um orçamento válido"})
		return
	}
	budget, err := h.loadBudgetForPDF(r.Context(), caller, request.BudgetID)
	if err != nil {
		writeResourceJSON(w, http.StatusNotFound, map[string]string{"error": "Orçamento não encontrado"})
		return
	}
	profile := h.loadBudgetPDFProfile(r.Context())
	output, err := pdf.BuildBudgetPDF(budget, profile)
	if err != nil {
		writeResourceJSON(w, http.StatusInternalServerError, map[string]string{"error": "Não foi possível gerar o PDF do orçamento"})
		return
	}
	name := unsafeFileNamePattern.ReplaceAllString(budget.ClientName, "_")
	if name == "" {
		name = "Cliente"
	}
	identifier := unsafeFileNamePattern.ReplaceAllString(budget.Number, "_")
	if identifier == "" {
		identifier = unsafeFileNamePattern.ReplaceAllString(budget.ID, "_")
	}
	w.Header().Set("Content-Type", "application/pdf")
	w.Header().Set("Content-Disposition", fmt.Sprintf(`attachment; filename="ORCAMENTO_INOVAR_%s_%s.pdf"`, name, identifier))
	w.Header().Set("Cache-Control", "no-store")
	_, _ = w.Write(output)
}

func (h BudgetPDFHandler) loadBudgetForPDF(ctx context.Context, caller supabase.Caller, id string) (domain.BudgetEstimate, error) {
	query := url.Values{"id": {"eq." + id}, "select": {"*"}}
	result, err := h.Supabase.UserRequest(ctx, caller.Token, "/rest/v1/budgets?"+query.Encode(), supabase.RequestOptions{Method: http.MethodGet})
	if err != nil || result.StatusCode != http.StatusOK {
		return domain.BudgetEstimate{}, fmt.Errorf("read budget: status=%d err=%v", result.StatusCode, err)
	}
	var rows []supabase.SupabaseBudget
	if err := json.Unmarshal(result.Body, &rows); err != nil || len(rows) != 1 {
		return domain.BudgetEstimate{}, fmt.Errorf("budget is not visible")
	}
	return supabase.MapSupabaseBudgetToLocal(rows[0]), nil
}

func (h BudgetPDFHandler) loadBudgetPDFProfile(ctx context.Context) domain.TechnicianProfile {
	profile := domain.TechnicianProfile{}
	config, err := (SettingsHandler{Supabase: h.Supabase}).loadSettings(ctx)
	if err != nil {
		config = defaultTechnicianSettings()
	}
	encoded, _ := json.Marshal(config)
	_ = json.Unmarshal(encoded, &profile)
	if profile.Name == "" {
		profile.Name = "Gabriel"
	}
	return profile
}
