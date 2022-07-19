package main

import (
	"bytes"
	"fmt"
	"io/ioutil"
	"net"
	"net/http"
	"net/http/httputil"
	"net/url"
	"regexp"
	"time"

	v4 "github.com/aws/aws-sdk-go/aws/signer/v4"
	log "github.com/sirupsen/logrus"
)

var awsAuthorizationCredentialRegexp = regexp.MustCompile("Credential=([a-zA-Z0-9]+)/[0-9]+/([a-z]+-[a-z]+-[0-9]+)/s3/aws4_request")
var awsAuthorizationSignedHeadersRegexp = regexp.MustCompile("SignedHeaders=([a-zA-Z0-9;-]+)")

// Handler is a special handler that re-signs any AWS S3 request and sends it upstream
type Handler struct {
	// Print debug information
	Debug bool

	// http or https
	UpstreamScheme string

	// Upstream S3 endpoint URL
	UpstreamEndpoint string

	// Allowed endpoint, i.e., Host header to accept incoming requests from
	AllowedSourceEndpoint string

	// Allowed source IPs and subnets for incoming requests
	AllowedSourceSubnet []*net.IPNet

	// AWS Credentials, AWS_ACCESS_KEY_ID and AWS_SECRET_ACCESS_KEY
	AWSCredentials map[string]string

	// AWS Signature v4
	Signers map[string]*v4.Signer

	// Reverse Proxy
	Proxy *httputil.ReverseProxy
}

func (h *Handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	proxyReq, err := h.buildUpstreamRequest(r)
	if err != nil {
		log.WithError(err).Error("unable to proxy request")
		w.WriteHeader(http.StatusBadRequest)

		// for security reasons, only write detailed error information in debug mode
		if h.Debug {
			w.Write([]byte(err.Error()))
		}
		return
	}

	url := url.URL{Scheme: proxyReq.URL.Scheme, Host: proxyReq.Host}
	proxy := httputil.NewSingleHostReverseProxy(&url)
	proxy.FlushInterval = 1
	proxy.ServeHTTP(w, proxyReq)
}

func (h *Handler) sign(signer *v4.Signer, req *http.Request, region string) error {
	return h.signWithTime(signer, req, region, time.Now())
}

func (h *Handler) signWithTime(signer *v4.Signer, req *http.Request, region string, signTime time.Time) error {
	body := bytes.NewReader([]byte{})
	if req.Body != nil {
		b, err := ioutil.ReadAll(req.Body)
		if err != nil {
			return err
		}
		body = bytes.NewReader(b)
	}

	_, err := signer.Sign(req, body, "s3", region, signTime)
	return err
}

func copyHeaderWithoutOverwrite(dst http.Header, src http.Header) {
	for k, v := range src {
		if _, ok := dst[k]; !ok {
			for _, vv := range v {
				dst.Add(k, vv)
			}
		}
	}
}

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

func (h *Handler) assembleUpstreamReq(signer *v4.Signer, req *http.Request, region string) (*http.Request, error) {
	upstreamEndpoint := h.UpstreamEndpoint
	if len(upstreamEndpoint) == 0 {
		upstreamEndpoint = fmt.Sprintf("s3.%s.amazonaws.com", region)
		log.Infof("Using %s as upstream endpoint", upstreamEndpoint)
	}

	proxyURL := *req.URL
	proxyURL.Scheme = h.UpstreamScheme
	proxyURL.Host = upstreamEndpoint
	proxyURL.RawPath = req.URL.Path
	proxyReq, err := http.NewRequest(req.Method, proxyURL.String(), req.Body)
	if err != nil {
		return nil, err
	}
	if val, ok := req.Header["Content-Type"]; ok {
		proxyReq.Header["Content-Type"] = val
	}
	if val, ok := req.Header["Content-Md5"]; ok {
		proxyReq.Header["Content-Md5"] = val
	}

	// Sign the upstream request
	if err := h.sign(signer, proxyReq, region); err != nil {
		log.Infof("Unable to Sing request")
		return nil, err
	}

	// Add origin headers after request is signed (no overwrite)
	copyHeaderWithoutOverwrite(proxyReq.Header, req.Header)

	return proxyReq, nil
}

// Do validates the incoming request and create a new request for an upstream server
func (h *Handler) buildUpstreamRequest(req *http.Request) (*http.Request, error) {
	// Ensure the request was sent from an allowed IP address
	err := h.validateIncomingSourceIP(req)
	if err != nil {
		return nil, err
	}

	keys := make([]string, 0, len(h.AWSCredentials))
	for k := range h.AWSCredentials {
		keys = append(keys, k)
	}

	accessKeyID := keys[0]

	// Get the AWS Signature signer for this AccessKey
	signer, ok := h.Signers[accessKeyID]
	if !ok {
		return nil, fmt.Errorf("Signer not found")
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
