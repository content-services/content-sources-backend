package api

// UserPreferenceResponse represents a single user preference
type UserPreferenceResponse struct {
	Label string `json:"label"` // Preference label
	Value string `json:"value"` // Preference value
}

// UserPreferencesResponse is a list of user preferences
type UserPreferencesResponse []UserPreferenceResponse
