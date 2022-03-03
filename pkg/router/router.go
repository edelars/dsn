package router

import (
	"context"

	uuid "github.com/gofrs/uuid"
	"github.com/rs/zerolog"
	"gl.dev.boquar.com/backend/device-status-aggregator/pkg/config"
	"gl.dev.boquar.com/backend/device-status-aggregator/pkg/model"
	"gl.dev.boquar.com/backend/device-status-aggregator/pkg/rightverifier"
	"gl.dev.boquar.com/backend/device-status-aggregator/pkg/router/mapstore"
	"gl.dev.boquar.com/backend/device-status-aggregator/pkg/worker"
)

type RouterHandler struct {
	idList        *mapstore.Store
	logger        *zerolog.Logger
	env           config.Environment
	rightVerifier *rightverifier.RightVerifierHandler
}

func NewRouterHandler(logger *zerolog.Logger, env config.Environment) *RouterHandler {

	routerHandler := RouterHandler{
		idList:        mapstore.NewStore(),
		logger:        logger,
		env:           env,
		rightVerifier: rightverifier.NewRightVerifierHandler(env),
	}

	return &routerHandler
}

func (hnd *RouterHandler) AddIds(ids []uuid.UUID, respMessagechan *chan *model.ResponseMessage, token string, ctxAggregator context.Context) {

	for _, id := range ids {

		if !hnd.checkPermit(id, respMessagechan, token) {
			continue
		}
		ctx, cancelFunc := context.WithCancel(context.Background())

		newListItem := mapstore.NewItemStore(worker.NewRequesterStatusHandler(id, hnd.env, hnd.logger), cancelFunc)

		listItem, exist := hnd.idList.GetOrCreate(id, newListItem, respMessagechan, ctxAggregator)

		if listItem == nil {
			cancelFunc()
			break
		}

		if !exist {
			hnd.logger.Debug().Msgf("Start new route goroutine for id: %s ", id.String())
			listItem.Worker.Run(ctx, listItem.WorkerChan)
			hnd.startRoute(ctx, id, listItem)
		} else {
			hnd.logger.Debug().Msgf("Route goroutine exist for id: %s ", id.String())
			cancelFunc()
		}

	}
}

func (hnd *RouterHandler) startRoute(ctx context.Context, id uuid.UUID, listItem *mapstore.ItemStore) {
	go func() {
	loop:
		for {
			select {
			case msg := <-listItem.WorkerChan:

				hnd.idList.PreFlight()
				aggregatorChanArray := listItem.GetAggregatorChanArray()

				if len(aggregatorChanArray) == 0 {
					hnd.logger.Debug().Msgf("Stop route for id: %s ", id.String())
					listItem.GetWorkerCancel()()
					hnd.idList.Delete(id)
					break loop
				}

				for _, ch := range aggregatorChanArray {
					*ch <- msg
					hnd.logger.Debug().Msgf("Route for id: %s ", id.String())
				}

				hnd.idList.AfterFlight()
				continue loop
			case <-ctx.Done():
				hnd.logger.Debug().Msgf("Stop route goroutine for id: %s , because ctx.Done()", id.String())
				break loop
			}
		}
		hnd.logger.Debug().Msgf("Quit from loop route goroutine for id: %s", id.String())
	}()
}

func (hnd *RouterHandler) checkPermit(id uuid.UUID, respMessagechan *chan *model.ResponseMessage, token string) bool {

	code, err := hnd.rightVerifier.Validate(id, token)

	switch code {

	case 200:
		return true
	case 401:
		*respMessagechan <- model.NewErrorResponseMessageTokenOutdated()
		hnd.logger.Err(err).Msgf("Token outdated: %s ", id.String())
	case 403:
		*respMessagechan <- model.NewErrorResponseMessageNoAccess(err, id)
		hnd.logger.Err(err).Msgf("Access denied: %s ", id.String())
	default:
		*respMessagechan <- model.NewErrorResponseMessageInternalError(err, id)
		hnd.logger.Err(err).Msgf("Unknown status code from auth server: %d", code)
	}

	return false
}

func (hnd *RouterHandler) Stop() {
	hnd.logger.Debug().Msg("Stop router")
	for _, workerCancel := range hnd.idList.GetAllWorkerCancelArray() {
		workerCancel()
	}
}
