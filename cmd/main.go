package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"sync"
	"syscall"

	"github.com/rs/zerolog/log"

	"github.com/jessevdk/go-flags"
	wsAPI "gl.dev.boquar.com/backend/device-status-aggregator/pkg/api/ws"
	"gl.dev.boquar.com/backend/device-status-aggregator/pkg/config"
	"gl.dev.boquar.com/backend/device-status-aggregator/pkg/router"
)

const (
	appName     = "device-status-aggregator"
	fqdnAppName = "device_status_aggregator"
)

func main() {
	var env config.Environment
	if _, err := flags.Parse(&env); err != nil {
		panic(err)
	}

	log.Logger = config.InitZerolog(appName, env.LogLevel)

	var wg sync.WaitGroup
	errs := make(chan error, 5)
	go waitInterruptSignal(errs)

	router := router.NewRouterHandler(&log.Logger, env)

	serverWS, err := wsAPI.NewServer(router, env)
	if err != nil {
		log.Panic().Err(err).Msg("unable to create websocket server")
	}
	log.Debug().Msgf("New websocket Server")

	wg.Add(1)
	go func() {
		defer wg.Done()
		errs <- serverWS.ListenAndServe()
	}()

	err = <-errs
	log.Err(err).Msg("trying to shutdown gracefully")

	wg.Add(1)
	go func() {
		defer wg.Done()
		err := serverWS.Shutdown(context.Background())
		log.Err(err).Msg("shutdown websocket server")
	}()

	wg.Add(1)
	go func() {
		defer wg.Done()
		router.Stop()
	}()
	wg.Wait()

	log.Info().Msg("gracefully stopped")
}

func waitInterruptSignal(errs chan<- error) {
	c := make(chan os.Signal, 3)
	signal.Notify(c, syscall.SIGINT, syscall.SIGTERM)
	errs <- fmt.Errorf("%s", <-c)
	signal.Stop(c)
}
