package proxyclient

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestGetURLScheme(t *testing.T) {
	var scheme string
	var err error

	scheme, err = GetURLScheme("https://www.google.com/")
	assert.Equal(t, "https", scheme)
	assert.Nil(t, err)

	_, err = GetURLScheme("this is not a valid URL")
	assert.NotNil(t, err)
}
