package relayer

import (
	"context"
	"reflect"
	"sync"
	"time"

	"github.com/polynetwork/bridge-common/log"
	"github.com/top/top-relayer/config"
)

type Server struct {
	ctx    context.Context
	wg     *sync.WaitGroup
	config *config.Config
	roles  []Handler
}

func Start(ctx context.Context, wg *sync.WaitGroup, config *config.Config) error {
	server := &Server{ctx, wg, config, nil}
	return server.Start()
}

func (s *Server) Start() (err error) {
	// Create handlers
	for id, chain := range s.config.Chains {
		if s.config.Active(id) {
			s.parseHandlers(id, chain.HeaderSync[0], chain.HeaderSync[1])
		}
	}

	// Initialize
	for i, handler := range s.roles {
		log.Info("Initializing role", "index", i, "total", len(s.roles), "type", reflect.TypeOf(handler), "chain", handler.Chain())
		err = handler.Init(s.ctx, s.wg)
		if err != nil {
			return
		}
	}

	// Start the roles
	for i, handler := range s.roles {
		log.Info("Starting role", "index", i, "total", len(s.roles), "type", reflect.TypeOf(handler), "chain", handler.Chain())
		err = handler.Start()
		if err != nil {
			return
		}
	}
	return
}

func (s *Server) parseHandlers(chain uint64, confs ...interface{}) {
	for _, conf := range confs {
		handler := s.parseHandler(chain, conf)
		if handler != nil {
			s.roles = append(s.roles, handler)
		}
	}
}

func (s *Server) parseHandler(chain uint64, conf interface{}) (handler Handler) {
	if reflect.ValueOf(conf).IsZero() || !reflect.ValueOf(conf).Elem().FieldByName("Enabled").Interface().(bool) {
		return
	}

	switch c := conf.(type) {
	case *config.HeaderSyncConfig:
		handler = NewHeaderSyncHandler(c)
	default:
		log.Error("Unknown config type", "conf", conf)
	}
	if handler != nil {
		log.Info("Creating handler", "type", reflect.TypeOf(handler))
		log.Json(log.TRACE, conf)
	}
	return
}

func retry(f func() error, interval time.Duration) {
	var err error
	for {
		err = f()
		if err == nil {
			return
		}
		time.Sleep(interval)
	}
}
