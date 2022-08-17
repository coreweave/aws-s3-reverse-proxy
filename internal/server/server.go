package server

import (
	"go.uber.org/zap"
	"net/http"
	"sync"
)

type ProxyServer struct {
	Handler http.Handler
	Log     *zap.Logger
	Cert    string
	Key     string
}

func (p *ProxyServer) StartServer(wg *sync.WaitGroup) {
	wg.Add(1)
	p.startHttp(wg)
	if p.Cert != "" && p.Key != "" {
		wg.Add(1)
		p.startHttps(wg)
	}
	wg.Wait()
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
		if err := http.ListenAndServeTLS(":8090", p.Cert, p.Key, p.Handler); err != nil {
			p.Log.Error("error in https serve")
			wg.Done()
		}
	}()
}
