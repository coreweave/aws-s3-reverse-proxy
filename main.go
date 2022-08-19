package main

import (
	"context"
	"github.com/Kriechi/aws-s3-reverse-proxy/internal/cfg"
	"github.com/Kriechi/aws-s3-reverse-proxy/internal/handler"
	"github.com/Kriechi/aws-s3-reverse-proxy/internal/server"
	"go.uber.org/zap"
	"net/http"
	_ "net/http/pprof"
	"sync"
)

func main() {
	var err error
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	var logger *zap.Logger
	opts := cfg.NewOptions()
	if opts.Debug {
		logger, err = zap.NewDevelopment()
	} else {
		logger, err = zap.NewProduction()
	}

	proxyHandler, err := handler.NewAwsS3ReverseProxy(ctx, logger, opts)
	if err != nil {
		logger.Sugar().Fatalf("unable to build proxy handler: %s", err.Error())
	}

	if len(proxyHandler.UpstreamEndpoint) == 0 {
		logger.Fatal("no endpoint provided for upstream")
	}

	logger.Sugar().Debugf("Sending requests to upstream Object Storage to endpoint %s://%s.", proxyHandler.UpstreamScheme, proxyHandler.UpstreamEndpoint)

	for _, subnet := range proxyHandler.AllowedSourceSubnet {
		logger.Sugar().Debugf("Allowing connections from %v.", subnet)
	}
	logger.Sugar().Debugf("Accepting incoming requests for this endpoint: %v", proxyHandler.AllowedSourceEndpoint)

	var wrappedHandler http.Handler = proxyHandler

	// Server
	srv := server.ProxyServer{
		Handler:          wrappedHandler,
		Log:              logger,
		Cert:             opts.CertFile,
		Key:              opts.KeyFile,
		UpstreamInsecure: opts.UpstreamInsecure,
	}

	wg := &sync.WaitGroup{}
	srv.StartServer(wg)
}
