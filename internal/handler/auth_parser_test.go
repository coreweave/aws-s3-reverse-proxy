package handler

import (
	"github.com/stretchr/testify/assert"
	"testing"
)

func TestAuthParser(t *testing.T) {
	testValue1 := "AWS4-HMAC-SHA256 Credential=BXXXX0XXXXNTXXXXXXX/20220816/default/s3/aws4_request"
	testValue2 := "AWS BXXXX0XXXXNTXXXXXXX:1pzMXThw6lbcI7L1FazG//LCKz4="

	authParser := AccessKeyParser{
		formats: []AccessKeyPattern{
			{accessKeyRegexp, accessKeySplitter},
			{altAccessKeyRegexp, altAccessKeySplitter},
		},
	}
	output, err := authParser.FindAccessKey(testValue1)
	assert.NoError(t, err)
	assert.Equal(t, "BXXXX0XXXXNTXXXXXXX", output)

	output, err = authParser.FindAccessKey(testValue2)
	assert.NoError(t, err)
	assert.Equal(t, "BXXXX0XXXXNTXXXXXXX", output)
}
