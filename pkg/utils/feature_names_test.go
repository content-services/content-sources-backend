package utils

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestSplitFeatureNames(t *testing.T) {
	assert.Nil(t, SplitFeatureNames(""))
	assert.Equal(t, []string{"a", "b"}, SplitFeatureNames("a, b"))
	assert.Equal(t, []string{"RHEL-EEUS-x86_64", "RHEL-E4S-x86_64"},
		SplitFeatureNames("RHEL-EEUS-x86_64,RHEL-E4S-x86_64"))
}

func TestNormalizeUniqueSortedFeatureNamesFromCSV(t *testing.T) {
	assert.Equal(t, []string{"RHEL-E4S-x86_64", "RHEL-EEUS-x86_64"},
		NormalizeUniqueSortedFeatureNamesFromCSV("RHEL-EEUS-x86_64,RHEL-E4S-x86_64,RHEL-EEUS-x86_64"))
}

func TestEntitledToAnyFeatureInCSV(t *testing.T) {
	ent := []string{"RHEL-E4S-x86_64"}
	assert.True(t, EntitledToAnyFeatureInCSV(ent, "RHEL-EEUS-x86_64,RHEL-E4S-x86_64"))
	assert.False(t, EntitledToAnyFeatureInCSV(ent, "RHEL-EEUS-x86_64"))
	assert.True(t, EntitledToAnyFeatureInCSV(ent, ""))
}

func TestImportFeatureMatches(t *testing.T) {
	filter := []string{"RHEL-OS-x86_64", "RHEL-E4S-x86_64"}
	assert.True(t, ImportFeatureMatches("RHEL-EEUS-x86_64,RHEL-E4S-x86_64", filter))
	assert.False(t, ImportFeatureMatches("RHEL-EEUS-x86_64", filter))
}
