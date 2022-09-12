package handler

import (
	"github.com/stretchr/testify/assert"
	"strings"
	"testing"
)

func TestEndpointsSplit(t *testing.T) {
	endpoints := "https://object.las1.coreweave.com,https://object.lga1.coreweave.com,https://object.ord1.coreweave.com"

	values := strings.Split(endpoints, ",")
	assert.Len(t, values, 3)
	assert.Equal(t, values[0], "https://object.las1.coreweave.com")
	assert.Equal(t, values[1], "https://object.lga1.coreweave.com")
	assert.Equal(t, values[2], "https://object.ord1.coreweave.com")
}
