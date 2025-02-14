package handler

import (
	"context"
	"errors"
	"fmt"
	v4 "github.com/aws/aws-sdk-go/aws/signer/v4"
	"github.com/coreweave/aws-s3-reverse-proxy/internal"
	"github.com/coreweave/aws-s3-reverse-proxy/internal/cfg"
	"github.com/coreweave/aws-s3-reverse-proxy/internal/proxy"
	"go.uber.org/zap"
	"net"
	"net/http"
	"net/http/httputil"
	"net/url"
	"regexp"
	"strconv"
	"strings"
)

var (
	// Headers
	authorizationHeader = "Authorization"
	contentMd5Header    = "Content-Md5"
	contentTypeHeader   = "Content-Type"
)

// Handler is a special handler that re-signs any AWS S3 request and sends it upstream
type Handler struct {
	// Zap Logger
	log *zap.Logger

	// handler or https
	UpstreamScheme string

	// Upstream S3 endpoint URL
	UpstreamEndpoint string

	// Experimental -- Upstream prefix to swap
	UpstreamProxyHelper *UpstreamHelper

	// Allowed endpoint, i.e., Host header to accept incoming requests from
	AllowedSourceEndpoint string

	// Allowed source IPs and subnets for incoming requests
	AllowedSourceSubnet []*net.IPNet

	// Reverse Proxies
	Proxies map[url.URL]*httputil.ReverseProxy

	//Auth Header parser
	AuthParser *AccessKeyParser

	// Auth Cache
	AuthCache internal.AuthCache
}

// NewAwsS3ReverseProxy parses all options and creates a new HTTP Handler
func NewAwsS3ReverseProxy(ctx context.Context, log *zap.Logger, opts cfg.Options, cache internal.AuthCache, https bool) (*Handler, error) {

	scheme := "http"

	if https {
		scheme = "https"
	}

	var parsedAllowedSourceSubnet []*net.IPNet
	for _, sourceSubnet := range opts.AllowedSourceSubnet {
		_, subnet, err := net.ParseCIDR(sourceSubnet)
		if err != nil {
			return nil, fmt.Errorf("Invalid allowed source subnet: %v", sourceSubnet)
		}
		parsedAllowedSourceSubnet = append(parsedAllowedSourceSubnet, subnet)
	}

	parser := NewAccessKeyParser()
	if opts.RgwAdminEndpoints == "" || opts.RgwAdminAccessKeys == "" || opts.RgwAdminSecretKeys == "" {
		log.Sugar().Errorf("missing one of the rgw endpoint variables, please ensure they are set")
		return nil, errors.New("missing required variable")
	}

	var upstreamProxyHelper *UpstreamHelper
	var upstreamEndpoint *string
	if len(opts.UpstreamEndpoint) != 0 {
		upstreamEndpoint = &opts.UpstreamEndpoint
	}
	var upstreamReplacers []UpstreamReplacer
	for _, val := range opts.UpstreamMatchers {
		if upstreamReplacers == nil {
			upstreamReplacers = make([]UpstreamReplacer, 0)
		}
		keys := strings.Split(val, ":")
		pattern, replacePattern, replaceValue := keys[0], keys[1], keys[2]
		levelDeep, err := strconv.ParseInt(keys[3], 10, 32)
		if err != nil {
			log.Sugar().Errorf("unable to parse levels value from upstream-matchers, invalid: %s", err.Error())
			return nil, err
		}
		upstreamReplacers = append(upstreamReplacers, UpstreamReplacer{
			MatchPattern:   regexp.MustCompile(pattern),
			ReplacePattern: regexp.MustCompile(replacePattern),
			ReplaceWith:    replaceValue,
			LevelsDeep:     int(levelDeep),
		})

	}
	upstreamProxyHelper, err := NewUpstreamHelper(log, upstreamEndpoint, upstreamReplacers)
	if err != nil {
		log.Fatal("unable to build upstream helper due to missing params")
	}
	proxies := make(map[url.URL]*httputil.ReverseProxy)
	handler := &Handler{
		UpstreamScheme:      scheme,
		UpstreamEndpoint:    opts.UpstreamEndpoint,
		AllowedSourceSubnet: parsedAllowedSourceSubnet,
		AuthParser:          parser,
		AuthCache:           cache,
		log:                 log,
		UpstreamProxyHelper: upstreamProxyHelper,
		Proxies:             proxies,
	}
	return handler, nil
}

func (h *Handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	proxyReq, err := h.BuildUpstreamRequest(r)
	if err != nil {
		if !errors.Is(err, internal.ErrNoAccessKeyFound) {
			h.log.Sugar().Infow("unable to proxy request due to error", "error", err.Error(), "request", r.Header)
			dumpReq, _ := httputil.DumpRequest(r, false)
			h.log.Sugar().Infow("Unauthenticated request proxied", "request", string(dumpReq))
			w.WriteHeader(http.StatusBadRequest)
			return
		}

	}
	upstreamUrl := url.URL{Scheme: proxyReq.URL.Scheme, Host: proxyReq.Host}
	h.log.Sugar().Debugf("upstreamURL found: %s://%s", upstreamUrl.Scheme, upstreamUrl.Host)
	if _, ok := h.Proxies[upstreamUrl]; !ok {
		h.Proxies[upstreamUrl] = httputil.NewSingleHostReverseProxy(&upstreamUrl)
		h.Proxies[upstreamUrl].FlushInterval = -1
	}
	h.Proxies[upstreamUrl].ServeHTTP(w, proxyReq)
}

// BuildUpstreamRequest Validates the incoming request and create a new request for an upstream server
func (h *Handler) BuildUpstreamRequest(req *http.Request) (*http.Request, error) {

	signRequest := false
	var signer *v4.Signer

	accessKey := req.Header.Get(authorizationHeader)

	if accessKey != "" {
		signRequest = true
		var key string
		var err error
		if key, err = h.AuthParser.FindAccessKey(accessKey); err != nil {
			h.log.Sugar().Errorf("unable to find an accessKey in auth header: %s", err.Error())
			return nil, err
		}
		// Get the AWS Signature signer for this AccessKey
		signer, err = h.AuthCache.GetRequestSigner(key)
		if err != nil {
			h.log.Sugar().Errorf("unable to find signer for key: %s", err.Error())
			return nil, err
		}
	}

	// Assemble a new upstream request
	proxyReq, err := h.assembleUpstreamReq(signer, req, "", signRequest)
	if err != nil {
		h.log.Sugar().Infof("Unable to assemble request: %s", err.Error())
		return nil, err
	}

	// Disable Go's "Transfer-Encoding: chunked" madness
	proxyReq.ContentLength = req.ContentLength

	return proxyReq, nil
}

// Private functions

func (h *Handler) validateIncomingSourceIP(req *http.Request) error {
	allowed := false
	for _, subnet := range h.AllowedSourceSubnet {
		ip, _, _ := net.SplitHostPort(req.RemoteAddr)
		userIP := net.ParseIP(ip)
		if subnet.Contains(userIP) {
			allowed = true
		}
	}
	if !allowed {
		return fmt.Errorf("source IP not allowed: %v", req)
	}
	return nil
}

func (h *Handler) assembleUpstreamReq(signer *v4.Signer, req *http.Request, region string, sign bool) (proxyReq *http.Request, err error) {

	proxyURL := req.URL
	h.log.Sugar().Debugf("URL: %s", proxyURL.String())
	h.log.Sugar().Debugf("proxyURL: %s", proxyURL.Host)
	currentHost := req.Host
	proxyURL.Host, err = h.UpstreamProxyHelper.PrepHost(currentHost)
	if err != nil {
		return nil, err
	}
	h.log.Sugar().Debugf("Using New Host: %s", proxyURL.Host)
	proxyURL.Scheme = h.UpstreamScheme
	proxyURL.RawPath = req.URL.Path
	proxyReq, err = http.NewRequest(req.Method, proxyURL.String(), req.Body)
	if err != nil {
		return nil, err
	}
	if val, ok := req.Header[contentTypeHeader]; ok {
		proxyReq.Header[contentTypeHeader] = val
	}
	if val, ok := req.Header[contentMd5Header]; ok {
		proxyReq.Header[contentMd5Header] = val
	}
	// Only sign if we have the key and a signed request.
	if sign {
		// Sign the upstream request
		if err = proxy.SignRequest(signer, proxyReq, region); err != nil {
			h.log.Sugar().Infof("Unable to Sing request")
			return nil, err
		}
	}

	// Add origin headers after request is signed (no overwrite)
	proxy.CopyHeaderWithoutOverwrite(proxyReq.Header, req.Header)

	return proxyReq, nil
}
