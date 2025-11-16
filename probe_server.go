package onlineconfbot

import (
	"context"
	stdlog "log"
	"net"
	"net/http"
	"sync"

	"github.com/rs/zerolog/log"
)

const (
	defaultProbeServerAddr      = "0.0.0.0:8000"
	defaultProbeServerUri       = "/probe"
	defaultProbeServerEnabled   = false
	defaultProbeServerLogPrefix = ""
)

type ProbeServer interface {
	Run(ctx context.Context) error
}

type probeServer struct {
	server *http.Server
	mu     sync.Mutex
	uri    string
}

var _ ProbeServer = &probeServer{}

func (ps *probeServer) Run(ctx context.Context) error {
	ps.mu.Lock()
	defer ps.mu.Unlock()

	if ps.server != nil {
		log.Info().Msg("Probe-Server was not configured. Do nothing")
		<-ctx.Done()
		return nil
	}

	ps.server.BaseContext = func(_ net.Listener) context.Context { return ctx }

	log.Info().Str("probe-addr", ps.server.Addr).Str("probe-uri", ps.uri).Msg("Starting Probe-Server")

	return ps.server.ListenAndServe()
}

func newProbeServer(addr string, uri string, enabled bool) ProbeServer {
	if !enabled {
		log.Info().Msg("Probe-Server starting is not enabled")
		return &probeServer{}
	}

	if addr == "" {
		addr = defaultProbeServerAddr
		log.Debug().Str("probe-addr", addr).Msg("Probe addres is not set. Using default")
	}

	if uri == "" {
		uri = defaultProbeServerUri
		log.Debug().Str("probe-uri", uri).Msg("Probe uri is not set. Using default")
	}

	mux := http.NewServeMux()
	mux.HandleFunc(uri, func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	return &probeServer{
		server: &http.Server{
			Addr:    addr,
			Handler: mux,
			ErrorLog: stdlog.New(
				log.Logger, defaultProbeServerLogPrefix, stdlog.LstdFlags),
		},
		uri: uri,
	}
}

func ProbeServerIfEnabled() ProbeServer {
	return newProbeServer(
		config.GetString("/probe/addr", ""),
		config.GetString("/probe/uri", ""),
		config.GetBool("/probe/enabled", defaultProbeServerEnabled),
	)
}
