package handler

import (
	"fmt"
	"github.com/coreweave/aws-s3-reverse-proxy/internal"
	"regexp"
	"strings"
)

var (
	accessKeyRegexp      = regexp.MustCompile("Credential=([a-zA-Z0-9]+)")
	accessKeySplitter    = "="
	altAccessKeyRegexp   = regexp.MustCompile("AWS ([a-zA-Z0-9]+)")
	altAccessKeySplitter = " "
)

type AccessKeyPattern struct {
	pattern  *regexp.Regexp
	splitter string
}

func (a *AccessKeyPattern) match(value string) (string, error) {
	if found := a.pattern.Find([]byte(value)); found == nil {
		return "", fmt.Errorf("could not find access key for pattern")
	} else {
		return string(found), nil
	}
}

func (a *AccessKeyPattern) Get(value string) (string, error) {
	if found, err := a.match(value); err == nil {
		return strings.Split(found, a.splitter)[1], nil
	}
	return "", internal.ErrNoAccessKeyFound
}

func NewAccessKeyParser() *AccessKeyParser {
	return &AccessKeyParser{
		formats: []AccessKeyPattern{
			{
				accessKeyRegexp, accessKeySplitter},
			{
				altAccessKeyRegexp, altAccessKeySplitter,
			},
		},
	}
}

type AccessKeyParser struct {
	formats []AccessKeyPattern
}

func (a *AccessKeyParser) FindAccessKey(authHeader string) (string, error) {
	for _, format := range a.formats {
		if found, err := format.Get(authHeader); err == nil {
			return found, nil
		}
	}
	return "", internal.ErrNoAccessKeyFound
}
