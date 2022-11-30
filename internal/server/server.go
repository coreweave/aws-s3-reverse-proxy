package server

import (
	"go.uber.org/zap"
	"net/http"
	"sync"
)

type ProxyServer struct {
	Handler          http.Handler
	HttpsHandler     http.Handler
	Log              *zap.Logger
	Cert             string
	Key              string
	UpstreamInsecure bool
	EnableProfiling  bool
}

func (p *ProxyServer) StartServer(wg *sync.WaitGroup) {
	wg.Add(1)
	p.startHttp(wg)
	if p.Cert != "" && p.Key != "" {
		wg.Add(1)
		p.startHttps(wg)
	}
	if p.EnableProfiling {
		wg.Add(1)
		p.startPprof(wg)
	}
	wg.Add(1)
	p.startHealthCheck(wg)
	wg.Wait()
}

func (p *ProxyServer) startHealthCheck(wg *sync.WaitGroup) {
	p.Log.Info("Starting health check endpoint")
	go func() {
		if err := http.ListenAndServe(":8888", http.HandlerFunc(
			func(w http.ResponseWriter, r *http.Request) {
				_, _ = w.Write([]byte("ok"))
			},
		)); err != nil {
			p.Log.Error("error in healthcheck server")
			wg.Done()
		}
	}()
}

func (p *ProxyServer) startHttp(wg *sync.WaitGroup) {
	p.Log.Info("Starting http server...")
	go func() {
		p.Log.Info("Starting up http on listen address :8099")
		if err := http.ListenAndServe(":8099", p.Handler); err != nil {
			p.Log.Error("error in http server")
			wg.Done()
		}
	}()
}

func (p *ProxyServer) startHttps(wg *sync.WaitGroup) {
	p.Log.Info("Starting https server...")
	go func() {
		p.Log.Info("Starting up https on listen address :8090")
		if err := http.ListenAndServeTLS(":8090", p.Cert, p.Key, p.HttpsHandler); err != nil {
			p.Log.Error("error in https serve")
			wg.Done()
		}
	}()
}

func (p *ProxyServer) startPprof(wg *sync.WaitGroup) {
	go func() {
		// avoid leaking pprof to the main application http servers
		pprofMux := http.DefaultServeMux
		http.DefaultServeMux = http.NewServeMux()
		// https://golang.org/pkg/net/http/pprof/
		p.Log.Sugar().Infof("Listening for pprof connections on %s", ":3000")
		p.Log.Info("Starting up pprof server...")
		if err := http.ListenAndServe(":3000", pprofMux); err != nil {
			p.Log.Sugar().Errorf("error on pprof server: %s", err.Error())
		}
		p.Log.Info("Shutting down pprof server...")
		wg.Done()
	}()
}
