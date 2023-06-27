package api

type FeatureSet map[string]Feature

type Feature struct {
	Enabled    bool `json:"enabled"`    // Whether the feature is enabled on the running server
	Accessible bool `json:"accessible"` // Whether the current user can access the feature
}
