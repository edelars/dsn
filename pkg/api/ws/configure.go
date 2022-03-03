package ws

import (
	"fmt"
	"net/http"
	"net/http/pprof"
	"runtime/debug"

	"github.com/gorilla/mux"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"github.com/rs/zerolog/log"
	hnd "gl.dev.boquar.com/backend/device-status-aggregator/pkg/api/ws/handler"
	"gl.dev.boquar.com/backend/device-status-aggregator/pkg/config"
	"gl.dev.boquar.com/backend/device-status-aggregator/pkg/router"
)

func NewServer(router *router.RouterHandler, env config.Environment) (*http.Server, error) {

	m := mux.NewRouter()
	m.Use(middlewareLogging, middlewareRecover)

	m.HandleFunc("/ping", func(rw http.ResponseWriter, req *http.Request) {
		rw.WriteHeader(http.StatusOK)
	})

	m.HandleFunc("/debug/pprof/cmdline", pprof.Cmdline)
	m.HandleFunc("/debug/pprof/profile", pprof.Profile)
	m.HandleFunc("/debug/pprof/symbol", pprof.Symbol)
	m.HandleFunc("/debug/pprof/trace", pprof.Trace)
	m.PathPrefix("/debug/pprof/").HandlerFunc(pprof.Index)

	m.Handle("/metrics", promhttp.Handler())

	m.Path("/ws/devices/status").
		Methods("GET").
		Handler(hnd.NewDeviceStatusHandler(router, env))

	return &http.Server{
		Addr:    fmt.Sprintf("%s:%d", "", env.WebSocketPort),
		Handler: m,
	}, nil
}

func middlewareRecover(h http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer func() {
			if e := recover(); e != nil {
				var err error
				switch errTyped := e.(type) {
				case error:
					err = errTyped
				default:
					err = fmt.Errorf("untyped panic: %v", e)
				}

				logger := log.Ctx(r.Context())
				logger.Error().Err(err).Msgf("panic:\n%s", string(debug.Stack()))
				w.WriteHeader(http.StatusInternalServerError)
			}
		}()
		h.ServeHTTP(w, r)
	})
}

func middlewareLogging(h http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {

		var requestID string
		if requestIDArr, ok := r.URL.Query()["request_id"]; ok {
			requestID = requestIDArr[0]
		}

		log := log.Logger.With().Str("METHOD", r.Method).Str("PATH", r.URL.Path).Str("REQUEST_ID", requestID).Logger()
		h.ServeHTTP(w, r.WithContext(log.WithContext(r.Context())))
	})
}
