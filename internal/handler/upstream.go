package handler

import (
	"errors"
	"go.uber.org/zap"
	"regexp"
	"strings"
)

var errNoHostMatch = errors.New("unable to modify host with upstream changes")
var errMissingUpstreamParameters = errors.New("missing valid parameters to format upstream requests")

var (
	regexPrefix  = "(^.*)."
	regexPostFix = ".(las1)|(lga1)|(ord1)|.coreweave.com"
)

type UpstreamReplacer struct {
	LevelsDeep     int
	MatchPattern   *regexp.Regexp
	ReplacePattern *regexp.Regexp
	ReplaceWith    string
}

func (u UpstreamReplacer) IsMatch(host string) bool {
	return u.MatchPattern.MatchString(host)
}

func (u UpstreamReplacer) MatchAndReplace(host string) (string, error) {
	if u.LevelsDeep != strings.Count(host, ".") {
		return "", errNoHostMatch
	}
	if u.MatchPattern.MatchString(host) {
		return u.ReplacePattern.ReplaceAllString(host, u.ReplaceWith), nil
	}
	return "", errNoHostMatch
}

type UpstreamHelper struct {
	log              *zap.Logger
	upstreamEndpoint *string
	replacers        []UpstreamReplacer
}

func NewUpstreamHelper(log *zap.Logger, upstreamEndpoint *string, replacers []UpstreamReplacer) (*UpstreamHelper, error) {
	if upstreamEndpoint == nil && replacers == nil {
		return nil, errMissingUpstreamParameters
	}

	return &UpstreamHelper{
		replacers:        replacers,
		upstreamEndpoint: upstreamEndpoint,
		log:              log,
	}, nil
}

func (u UpstreamHelper) PrepHost(originHost string) (result string, err error) {
	u.log.Sugar().Infof("origin host: %s", originHost)
	if u.upstreamEndpoint != nil {
		return *u.upstreamEndpoint, nil
	}

	for _, replace := range u.replacers {
		if result, err = replace.MatchAndReplace(originHost); err == nil {
			return result, nil
		}
	}
	u.log.Debug("did not match the origin format, err no host")
	return "", err
}
