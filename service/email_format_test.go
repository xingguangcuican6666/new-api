package service

import (
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestEmailFormatMatches(t *testing.T) {
	original := common.EmailFormatRegex
	t.Cleanup(func() { common.EmailFormatRegex = original })

	// Empty (or blank) configuration applies no extra constraint.
	common.EmailFormatRegex = ""
	assert.True(t, EmailFormatMatches("anything at all"))
	common.EmailFormatRegex = "   "
	assert.True(t, EmailFormatMatches("anything at all"))

	common.EmailFormatRegex = `^[^@]+@example\.com$`
	assert.True(t, EmailFormatMatches("user@example.com"))
	assert.False(t, EmailFormatMatches("user@other.com"))

	// A broken pattern is ignored instead of rejecting everything.
	common.EmailFormatRegex = `[`
	assert.True(t, EmailFormatMatches("user@other.com"))
}

func TestValidateAccountEmailAppliesCustomFormat(t *testing.T) {
	original := common.EmailFormatRegex
	t.Cleanup(func() { common.EmailFormatRegex = original })

	common.EmailFormatRegex = `^[^@]+@example\.com$`
	_, err := ValidateAccountEmail("User@Example.com")
	require.NoError(t, err)
	_, err = ValidateAccountEmail("user@other.com")
	assert.Equal(t, ErrAccountEmailInvalid, err)
}
