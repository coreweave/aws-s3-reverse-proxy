package handler

import (
	"regexp"
	"testing"
)

func TestUpstreamMatch(t *testing.T) {
	//upstreamRegex = regexp.MustCompile("(^.*).s3.(las1)|(lga1)|(ord1)|.coreweave.com")
	s3Regex := regexp.MustCompile(".s3.")
	testValue := "my-bucket.s3.las1.coreweave.com"

	result := s3Regex.ReplaceAllString(testValue, ".object.")

	t.Logf("output: %s", result)
}
