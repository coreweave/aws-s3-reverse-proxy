package main

import (
	"context"
	"fmt"
	"github.com/coreweave/aws-s3-reverse-proxy/internal/cache"
	"github.com/coreweave/aws-s3-reverse-proxy/internal/cfg"
	"github.com/coreweave/aws-s3-reverse-proxy/internal/handler"
	"github.com/coreweave/aws-s3-reverse-proxy/internal/server"
	"go.uber.org/zap"
	"net/http"
	_ "net/http/pprof"
	"os"
	"sync"
	"time"
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

	if err != nil {
		fmt.Printf("unable to build logger: %s", err.Error())
		os.Exit(2)
	}

	adminClient := handler.NewRgwAdminClient(opts.RgwAdminAccessKey, opts.RgwAdminSecretKey, opts.RgwAdminEndpoint)
	authCache := cache.NewAuthCache(adminClient, logger, time.Duration(opts.ExpireCacheMinutes)*time.Minute, time.Duration(opts.EvictCacheMinutes)*time.Minute)
	//Load initial key state
	if err = authCache.Load(); err != nil {
		logger.Sugar().Errorf("unable to load initial rgw user keys due to: %s", err.Error())
		os.Exit(3)
	}

	// Runs async cach syncing every 5 minutes for new users and deleted users
	authCache.RunSync(5*time.Minute, ctx)

	proxyHandler, err := handler.NewAwsS3ReverseProxy(ctx, logger, opts, authCache, false)
	if err != nil {
		logger.Sugar().Fatalf("unable to build proxy handler: %s", err.Error())
	}

	httpsProxyHandler, err := handler.NewAwsS3ReverseProxy(ctx, logger, opts, authCache, true)
	if err != nil {
		logger.Sugar().Fatalf("unable to build proxy handler: %s", err.Error())
	}

	for _, subnet := range proxyHandler.AllowedSourceSubnet {
		logger.Sugar().Debugf("Allowing connections from %v.", subnet)
	}

	var wrappedHandler http.Handler = proxyHandler
	var wrappedHttpsHandler http.Handler = httpsProxyHandler

	// Server
	srv := server.ProxyServer{
		Handler:          wrappedHandler,
		HttpsHandler:     wrappedHttpsHandler,
		Log:              logger,
		Cert:             opts.CertFile,
		Key:              opts.KeyFile,
		UpstreamInsecure: opts.UpstreamInsecure,
	}

	wg := &sync.WaitGroup{}
	srv.StartServer(wg)
}
