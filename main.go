package main

import (
	"context"
	"fmt"
	"github.com/Kriechi/aws-s3-reverse-proxy/internal/cfg"
	"github.com/Kriechi/aws-s3-reverse-proxy/internal/handler"
	"github.com/Kriechi/aws-s3-reverse-proxy/internal/server"
	"go.uber.org/zap"
	"net/http"
	_ "net/http/pprof"
	"os"
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
	
	if err != nil {
		fmt.Printf("unable to build logger: %s", err.Error())
		os.Exit(2)
	}

	proxyHandler, err := handler.NewAwsS3ReverseProxy(ctx, logger, opts)
	if err != nil {
		logger.Sugar().Fatalf("unable to build proxy handler: %s", err.Error())
	}

	for _, subnet := range proxyHandler.AllowedSourceSubnet {
		logger.Sugar().Debugf("Allowing connections from %v.", subnet)
	}

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
