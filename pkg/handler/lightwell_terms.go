package handler

import (
	"net/http"

	"github.com/content-services/content-sources-backend/pkg/api"
	"github.com/content-services/content-sources-backend/pkg/clients/terms_service_client"
	ce "github.com/content-services/content-sources-backend/pkg/errors"
	"github.com/content-services/content-sources-backend/pkg/rbac"
	"github.com/labstack/echo/v4"
	"github.com/redhatinsights/platform-go-middlewares/v2/identity"
	"github.com/rs/zerolog/log"
)

type LightwellTermsHandler struct {
	TermsServiceClient terms_service_client.TermsServiceClient
}

func RegisterLightwellTermsRoutes(engine *echo.Group, tsClient *terms_service_client.TermsServiceClient) {
	if tsClient == nil {
		panic("tsClient is nil")
	}
	h := LightwellTermsHandler{TermsServiceClient: *tsClient}
	addRepoRoute(engine, http.MethodGet, "/lightwell/terms/required", h.GetTermsRequired, rbac.RbacVerbRead)
	addRepoRoute(engine, http.MethodGet, "/lightwell/terms/details", h.GetTermsDetails, rbac.RbacVerbRead)
	addRepoRoute(engine, http.MethodPut, "/lightwell/terms/accept", h.AcceptTerms, rbac.RbacVerbWrite)
}

// getLogin extracts the username from the request identity, guarding against
// nil User (e.g. service accounts). Returns empty string if unavailable.
func getLogin(c echo.Context) string {
	id := identity.GetIdentity(c.Request().Context())
	if id.Identity.User != nil && id.Identity.User.Username != "" {
		return id.Identity.User.Username
	}
	// TODO: Confirm Identity.User.Username reliability for service accounts
	if id.Identity.ServiceAccount != nil {
		return id.Identity.ServiceAccount.Username
	}
	return ""
}

// GetTermsRequired godoc
// @Summary      Check if Lightwell terms acceptance is required
// @ID           getLightwellTermsRequired
// @Description  Check whether the current user must accept Lightwell terms before access.
// @Tags         lightwell
// @Produce      json
// @Success      200 {object} api.TermsRequiredResponse
// @Failure      500 {object} ce.ErrorResponse
// @Router       /lightwell/terms/required [get]
func (h *LightwellTermsHandler) GetTermsRequired(c echo.Context) error {
	ctx := c.Request().Context()

	if err := CheckLightwellTermsAccessible(ctx); err != nil {
		return c.JSON(http.StatusOK, api.TermsRequiredResponse{Required: false})
	}

	login := getLogin(c)
	if login == "" {
		return c.JSON(http.StatusOK, api.TermsRequiredResponse{Required: false})
	}

	required, err := h.TermsServiceClient.IsTermsAcceptanceRequired(ctx, login)
	if err != nil {
		log.Ctx(ctx).Error().Err(err).Msg("error checking terms acceptance requirement")
		return ce.NewErrorResponse(http.StatusInternalServerError, "Error checking terms requirement", err.Error())
	}

	return c.JSON(http.StatusOK, api.TermsRequiredResponse{Required: required})
}

// GetTermsDetails godoc
// @Summary      Get required Lightwell terms details
// @ID           getLightwellTermsDetails
// @Description  Retrieve term names, PDF links, and IDs for terms the user must accept.
// @Tags         lightwell
// @Produce      json
// @Success      200 {object} api.TermsDetailsResponse
// @Failure      400 {object} ce.ErrorResponse
// @Failure      500 {object} ce.ErrorResponse
// @Router       /lightwell/terms/details [get]
func (h *LightwellTermsHandler) GetTermsDetails(c echo.Context) error {
	ctx := c.Request().Context()

	if err := CheckLightwellTermsAccessible(ctx); err != nil {
		return err
	}

	login := getLogin(c)
	if login == "" {
		return ce.NewErrorResponse(http.StatusBadRequest, "Error getting terms details", "Unable to determine user login")
	}

	terms, err := h.TermsServiceClient.GetRequiredTerms(ctx, login)
	if err != nil {
		log.Ctx(ctx).Error().Err(err).Msg("error getting required terms")
		return ce.NewErrorResponse(http.StatusInternalServerError, "Error getting terms details", err.Error())
	}

	mapped := make([]api.TermDetailResponse, 0, len(terms))
	for _, t := range terms {
		translations := make([]api.TermTranslationResponse, 0, len(t.Translations))
		for _, tr := range t.Translations {
			translations = append(translations, api.TermTranslationResponse{
				ID:                     tr.ID,
				TermsPdfID:             tr.TermsPdfID,
				LocaleCode:             tr.LocaleCode,
				TranslatedTermsName:    tr.TranslatedTermsName,
				TranslatedDescription:  tr.TranslatedDescription,
				TranslatedInstructions: tr.TranslatedInstructions,
				IsDefault:              tr.IsDefault,
				PdfDownloadURL:         tr.PdfDownloadURL,
			})
		}
		mapped = append(mapped, api.TermDetailResponse{
			ID:                   t.ID,
			URLToDisplayThisTerm: t.URLToDisplayThisTerm,
			URLToDisplayAllTerms: t.URLToDisplayAllTerms,
			IsOptional:           t.IsOptional,
			Translations:         translations,
		})
	}

	return c.JSON(http.StatusOK, api.TermsDetailsResponse{Terms: mapped})
}

// AcceptTerms godoc
// @Summary      Accept Lightwell terms
// @ID           acceptLightwellTerms
// @Description  Record the user's acceptance of a specific Lightwell term.
// @Tags         lightwell
// @Accept       json
// @Produce      json
// @Param        body  body  api.TermsAcceptRequest  true  "Term acceptance request"
// @Success      200 {object} api.TermsAcceptResponse
// @Failure      400 {object} ce.ErrorResponse
// @Failure      500 {object} ce.ErrorResponse
// @Router       /lightwell/terms/accept [put]
func (h *LightwellTermsHandler) AcceptTerms(c echo.Context) error {
	ctx := c.Request().Context()

	if err := CheckLightwellTermsAccessible(ctx); err != nil {
		return err
	}

	var req api.TermsAcceptRequest
	if err := c.Bind(&req); err != nil {
		return ce.NewErrorResponse(http.StatusBadRequest, "Error accepting terms", err.Error())
	}
	if req.TermsPdfID == "" {
		return ce.NewErrorResponse(http.StatusBadRequest, "Error accepting terms", "terms_pdf_id is required")
	}

	login := getLogin(c)
	if login == "" {
		return ce.NewErrorResponse(http.StatusBadRequest, "Error accepting terms", "Unable to determine user login")
	}

	if err := h.TermsServiceClient.AcceptTerm(ctx, login, req.TermsPdfID); err != nil {
		log.Ctx(ctx).Error().Err(err).Msg("error accepting terms")
		return ce.NewErrorResponse(http.StatusInternalServerError, "Error accepting terms", err.Error())
	}

	return c.JSON(http.StatusOK, api.TermsAcceptResponse{Accepted: true})
}
