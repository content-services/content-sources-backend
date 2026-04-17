package utils

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestParseFeatures(t *testing.T) {
	assert.Nil(t, ParseFeatures(""))
	assert.Equal(t, []string{"a", "b"}, ParseFeatures("a, b"))
	// Underscores stay inside tokens; comma or plus separates (plus is for readability only).
	assert.Equal(t, []string{"repo_feature_alpha_x86_64", "repo_feature_beta_x86_64"},
		ParseFeatures("repo_feature_beta_x86_64 + repo_feature_alpha_x86_64"))
	assert.Equal(t, []string{"repo_feature_alpha_x86_64", "repo_feature_beta_x86_64"},
		ParseFeatures("repo_feature_alpha_x86_64, repo_feature_beta_x86_64"))
	assert.Equal(t, []string{"RHEL-E4S-x86_64", "RHEL-EEUS-x86_64"},
		ParseFeatures("RHEL-EEUS-x86_64,RHEL-E4S-x86_64"))
	assert.Equal(t, []string{"RHEL-E4S-x86_64", "RHEL-EEUS-x86_64"},
		ParseFeatures("RHEL-EEUS-x86_64,RHEL-E4S-x86_64,RHEL-EEUS-x86_64"))
}

func TestAnyFeatureMatch(t *testing.T) {
	ent := []string{"RHEL-E4S-x86_64"}
	assert.True(t, AnyFeatureMatch("RHEL-EEUS-x86_64,RHEL-E4S-x86_64", ent))
	assert.True(t, AnyFeatureMatch("RHEL-EEUS-x86_64 + RHEL-E4S-x86_64", ent))
	assert.False(t, AnyFeatureMatch("RHEL-EEUS-x86_64", ent))
	assert.True(t, AnyFeatureMatch("", ent))

	filter := []string{"RHEL-OS-x86_64", "RHEL-E4S-x86_64"}
	assert.True(t, AnyFeatureMatch("RHEL-EEUS-x86_64,RHEL-E4S-x86_64", filter))
	assert.False(t, AnyFeatureMatch("RHEL-EEUS-x86_64", filter))
	assert.True(t, AnyFeatureMatch("", filter))

	under := []string{"svc_feature_one_x86_64"}
	assert.True(t, AnyFeatureMatch("svc_feature_one_x86_64 + svc_feature_two_x86_64", under))
	assert.False(t, AnyFeatureMatch("svc_feature_two_x86_64", under))
}
