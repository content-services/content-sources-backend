package api

// TermsRequiredResponse is returned by the terms-required check endpoint.
type TermsRequiredResponse struct {
	Required bool `json:"required"`
}

// TermTranslationResponse represents a locale-specific translation of a term.
type TermTranslationResponse struct {
	ID                     string  `json:"id"`
	TermsPdfID             string  `json:"terms_pdf_id"`
	LocaleCode             string  `json:"locale_code"`
	TranslatedTermsName    string  `json:"translated_terms_name"`
	TranslatedDescription  *string `json:"translated_description"`
	TranslatedInstructions *string `json:"translated_instructions"`
	IsDefault              bool    `json:"is_default"`
	PdfDownloadURL         string  `json:"pdf_download_url"`
}

// TermDetailResponse represents a single term the user must accept.
type TermDetailResponse struct {
	ID                   string                    `json:"id"`
	URLToDisplayThisTerm *string                   `json:"url_to_display_this_term"`
	URLToDisplayAllTerms string                    `json:"url_to_display_all_terms"`
	IsOptional           bool                      `json:"is_optional"`
	Translations         []TermTranslationResponse `json:"translations"`
}

// TermsDetailsResponse is returned by the terms-details endpoint.
type TermsDetailsResponse struct {
	Terms []TermDetailResponse `json:"terms"`
}

// TermsAcceptRequest is the request body for the accept-terms endpoint.
type TermsAcceptRequest struct {
	TermsPdfID string `json:"terms_pdf_id"`
}

// TermsAcceptResponse is returned after successfully recording acceptance.
type TermsAcceptResponse struct {
	Accepted bool `json:"accepted"`
}
