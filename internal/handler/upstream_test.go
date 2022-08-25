package handler

import (
	"github.com/aws/aws-sdk-go/aws"
	"github.com/stretchr/testify/assert"
	"go.uber.org/zap"
	"regexp"
	"testing"
)

func TestUpstreamProxy(t *testing.T) {
	//matching := regexp.MustCompile("(^.*).s3.(las1)|(lga1)|(ord1)|.coreweave.com")
	//origin := regexp.MustCompile(".s3.")
	log, _ := zap.NewDevelopment()
	replacers := []UpstreamReplacer{{
		MatchPattern:   regexp.MustCompile("obj.(las1)|(lga1)|(ord1)|.coreweave.com"),
		ReplacePattern: regexp.MustCompile("obj."),
		ReplaceWith:    "s3.",
		LevelsDeep:     3,
	}, {
		MatchPattern:   regexp.MustCompile("(^.*).object.(las1)|(lga1)|(ord1)|.coreweave.com"),
		ReplacePattern: regexp.MustCompile(".object."),
		ReplaceWith:    ".s3.",
		LevelsDeep:     4,
	},
	}
	upstream, _ := NewUpstreamHelper(log, nil, replacers)

	testValue := "my-bucket.object.las1.coreweave.com"
	expectedValue := "my-bucket.s3.las1.coreweave.com"

	result, err := upstream.PrepHost(testValue)

	assert.NoError(t, err)
	assert.Equal(t, expectedValue, result)

	testValue2 := "obj.las1.coreweave.com"
	expectedValue = "s3.las1.coreweave.com"
	result, err = upstream.PrepHost(testValue2)
	assert.NoError(t, err)
	assert.Equal(t, expectedValue, result)
}

func TestUpstreamWithEndpoint(t *testing.T) {
	log, _ := zap.NewDevelopment()
	upstream, _ := NewUpstreamHelper(log, aws.String("s3.las1.coreweave.com"), nil)

	testValue := "my-bucket.s3.las1.coreweave.com"
	expectedValue := "s3.las1.coreweave.com"

	result, err := upstream.PrepHost(testValue)

	assert.NoError(t, err)
	assert.Equal(t, expectedValue, result)
}
