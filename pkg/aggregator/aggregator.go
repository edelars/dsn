package aggregator

import (
	"context"
	"fmt"
	"time"

	uuid "github.com/gofrs/uuid"
	"github.com/rs/zerolog"
	"gl.dev.boquar.com/backend/device-status-aggregator/pkg/config"
	"gl.dev.boquar.com/backend/device-status-aggregator/pkg/model"
	"gl.dev.boquar.com/backend/device-status-aggregator/pkg/router"
)

const (
	readDeadline  = 30 * time.Second
	writeDeadline = 15 * time.Second
	pingPeriod    = (readDeadline * 9) / 10
)

type AggregatorStatusHandler struct {
	RespMessageAggregate chan *model.ResponseMessage
	env                  config.Environment
	logger               *zerolog.Logger
	router               *router.RouterHandler
	token                string
	CancelF              context.CancelFunc
}

func NewAggregatorStatusHandler(ctx context.Context, env config.Environment, logger *zerolog.Logger, router *router.RouterHandler, token string) *AggregatorStatusHandler {

	aggregatorStatusHandler := AggregatorStatusHandler{
		RespMessageAggregate: make(chan *model.ResponseMessage, 20),
		env:                  env,
		logger:               logger,
		router:               router,
		token:                token,
	}

	return &aggregatorStatusHandler
}

//Will subcribe to new list (ids) devices and unsub from all old
func (hnd *AggregatorStatusHandler) SubscribeDevices(ctx context.Context, ids []uuid.UUID) {

	hnd.Stop()

	ch := make(chan *model.ResponseMessage, 20)

	ctx, hnd.CancelF = context.WithCancel(ctx)

	hnd.router.AddIds(ids, &ch, hnd.token, ctx)

	hnd.logger.Debug().Msg("Subscribe to new devices: " + fmt.Sprint(ids))

	go func() {

	cicle:
		for {
			select {
			case msg := <-ch:
				hnd.RespMessageAggregate <- msg
			case <-ctx.Done():
				break cicle
			}
		}
	}()
}

//Make unsub from all devices. (stop goroutine)
func (hnd *AggregatorStatusHandler) Stop() {
	if hnd.CancelF != nil {
		hnd.CancelF()
	}
}
