package api

// TermsRequiredResponse is returned by the terms-required check endpoint.
type TermsRequiredResponse struct {
	Required bool `json:"required"`
}
