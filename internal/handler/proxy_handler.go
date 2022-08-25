package handler

import (
	"context"
	"errors"
	"fmt"
	"github.com/Kriechi/aws-s3-reverse-proxy/internal"
	"github.com/Kriechi/aws-s3-reverse-proxy/internal/cache"
	"github.com/Kriechi/aws-s3-reverse-proxy/internal/cfg"
	"github.com/Kriechi/aws-s3-reverse-proxy/internal/proxy"
	v4 "github.com/aws/aws-sdk-go/aws/signer/v4"
	log "github.com/sirupsen/logrus"
	"go.uber.org/zap"
	"net"
	"net/http"
	"net/http/httputil"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"time"
)

var (
	// Headers
	authorizationHeader   = "Authorization"
	contentMd5Header      = "Content-Md5"
	contentTypeHeader     = "Content-Type"
	contentAmzCacheHeader = "x-amz-cache"

	//upstreamRegex = regexp.MustCompile("(^.*).s3.(las1)|(lga1)|(ord1)|.coreweave.com")
	upstreamRegex = regexp.MustCompile(".s3.")
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

	// Reverse Proxy
	Proxy *httputil.ReverseProxy

	//Auth Header parser
	AuthParser *AccessKeyParser

	// Auth Cache
	AuthCache internal.AuthCache
}

// NewAwsS3ReverseProxy parses all options and creates a new HTTP Handler
func NewAwsS3ReverseProxy(ctx context.Context, log *zap.Logger, opts cfg.Options) (*Handler, error) {

	scheme := "https"
	if opts.UpstreamInsecure {
		log.Debug("upstream is insecure..setting to http")
		scheme = "http"
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
	if opts.RgwAdminEndpoint == "" || opts.RgwAdminAccessKey == "" || opts.RgwAdminSecretKey == "" {
		log.Sugar().Errorf("missing one of the rgw endpoint variables, please ensure they are set")
		return nil, errors.New("missing required variable")
	}
	adminClient := NewRgwAdminClient(opts.RgwAdminAccessKey, opts.RgwAdminSecretKey, opts.RgwAdminEndpoint)
	authCache := cache.NewAuthCache(adminClient, log, time.Duration(opts.ExpireCacheMinutes)*time.Minute, time.Duration(opts.EvictCacheMinutes)*time.Minute)
	//Load initial key state
	if err := authCache.Load(); err != nil {
		log.Sugar().Errorf("unable to load initial rgw user keys due to: %s", err.Error())
		return nil, err
	}

	// Runs async cach syncing every 5 minutes for new users and deleted users
	authCache.RunSync(5*time.Minute, ctx)

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
	handler := &Handler{
		UpstreamScheme:      scheme,
		UpstreamEndpoint:    opts.UpstreamEndpoint,
		AllowedSourceSubnet: parsedAllowedSourceSubnet,
		AuthParser:          parser,
		AuthCache:           authCache,
		log:                 log,
		UpstreamProxyHelper: upstreamProxyHelper,
	}
	return handler, nil
}

func (h *Handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	h.log.Sugar().Infof("Original request Host: %s", r.Host)
	h.log.Sugar().Infof("Original Request headers: %v", r.Header)
	proxyReq, err := h.BuildUpstreamRequest(r)
	if err != nil {
		h.log.Sugar().Errorf("unable to proxy request due to error: %s", err.Error())
		w.WriteHeader(http.StatusBadRequest)
		return
	}

	upstreamUrl := url.URL{Scheme: proxyReq.URL.Scheme, Host: proxyReq.Host}
	upstreamProxy := httputil.NewSingleHostReverseProxy(&upstreamUrl)
	upstreamProxy.FlushInterval = 1
	upstreamProxy.ServeHTTP(w, proxyReq)
}

// BuildUpstreamRequest Validates the incoming request and create a new request for an upstream server
func (h *Handler) BuildUpstreamRequest(req *http.Request) (*http.Request, error) {
	// Ensure the request was sent from an allowed IP address
	err := h.validateIncomingSourceIP(req)
	if err != nil {
		return nil, err
	}

	accessKey := req.Header.Get(authorizationHeader)

	h.log.Sugar().Debugf("original host: %s", req.Host)
	if accessKey == "" {
		return nil, errors.New("no access key")
	}

	var key string

	if key, err = h.AuthParser.FindAccessKey(accessKey); err != nil {
		log.Errorf("unable to find an accessKey in aut header: %s", err.Error())
		return nil, err
	}

	// Get the AWS Signature signer for this AccessKey
	signer, err := h.AuthCache.GetRequestSigner(key)
	if err != nil {

	}
	// Assemble a new upstream request
	proxyReq, err := h.assembleUpstreamReq(signer, req, "")
	if err != nil {
		log.Infof("Unable to assemble request")
		return nil, err
	}

	// Disable Go's "Transfer-Encoding: chunked" madness
	proxyReq.ContentLength = req.ContentLength

	if log.GetLevel() == log.DebugLevel {
		proxyReqDump, _ := httputil.DumpRequest(proxyReq, false)
		log.Debugf("Proxying request: %v", string(proxyReqDump))
	}

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

func (h *Handler) assembleUpstreamReq(signer *v4.Signer, req *http.Request, region string) (proxyReq *http.Request, err error) {

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

	// Sign the upstream request
	if err := proxy.SignRequest(signer, proxyReq, region); err != nil {
		log.Infof("Unable to Sing request")
		return nil, err
	}

	// Add origin headers after request is signed (no overwrite)
	proxy.CopyHeaderWithoutOverwrite(proxyReq.Header, req.Header)

	return proxyReq, nil
}
