package ws

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"time"

	gws "github.com/gobwas/ws"
	"github.com/gobwas/ws/wsutil"
	"github.com/rs/zerolog"
	"gl.dev.boquar.com/backend/device-status-aggregator/pkg/aggregator"
	"gl.dev.boquar.com/backend/device-status-aggregator/pkg/config"
	"gl.dev.boquar.com/backend/device-status-aggregator/pkg/model"
	"gl.dev.boquar.com/backend/device-status-aggregator/pkg/router"
)

const (
	readDeadline           = 30 * time.Second
	writeDeadline          = 15 * time.Second
	pingPeriod             = (readDeadline * 9) / 10
	connectionCloseTimeout = 5 * time.Second
	state                  = gws.StateServerSide
)

var codes = map[string]int{"200": 1000, "401": 4001, "403": 4003, "404": 4004, "500": 1011}

type DeviceStatusHandler struct {
	env    config.Environment
	router *router.RouterHandler
}

func NewDeviceStatusHandler(router *router.RouterHandler, env config.Environment) *DeviceStatusHandler {
	return &DeviceStatusHandler{
		env:    env,
		router: router,
	}
}

func (hnd *DeviceStatusHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {

	logger := zerolog.Ctx(r.Context())

	logger.Debug().Msg("get new connect")

	con, _, _, err := gws.UpgradeHTTP(r, w)
	if err != nil {
		logger.Err(err).Msg("failed to upgrade connection to WS, will close")
		return
	}

	var token string
	tokenArray := r.URL.Query()["token"]
	if len(tokenArray) < 1 {
		logger.Debug().Msg("no token in query")
	} else {
		token = tokenArray[0]
	}

	worker := NewDeviceStatusWorker(con, hnd.env, logger, hnd.router, token)
	go worker.Run(token)
}

type DeviceStatusWorker struct {
	aggregator      *aggregator.AggregatorStatusHandler
	eventsCloseChan chan *closeEvent
	inputChan       chan *model.RequestMessage
	pongChan        chan bool
	pinger          *time.Ticker
	conn            net.Conn
	env             config.Environment
	logger          *zerolog.Logger
	router          *router.RouterHandler
	token           string
}

func NewDeviceStatusWorker(conn net.Conn, env config.Environment, logger *zerolog.Logger, router *router.RouterHandler, token string) *DeviceStatusWorker {
	return &DeviceStatusWorker{conn: conn, env: env, logger: logger, router: router, token: token}
}

type closeEvent struct {
	force  bool
	reason error
	code   int
}

func (hnd *DeviceStatusWorker) Run(token string) {

	ctx, cancelServe := context.WithCancel(context.Background())

	hnd.pinger = time.NewTicker(pingPeriod)
	hnd.inputChan = make(chan *model.RequestMessage, 5)
	hnd.eventsCloseChan = make(chan *closeEvent, 2)
	hnd.pongChan = make(chan bool, 2)

	defer func() {
		cancelServe()
		hnd.pinger.Stop()
		close(hnd.inputChan)
		close(hnd.eventsCloseChan)
		close(hnd.pongChan)
		if hnd.aggregator != nil {
			hnd.aggregator.Stop()
		}
		if err := (hnd.conn).Close(); err != nil {
			hnd.logger.Err(err).Msg("failed to close connection on shutdown")
		}
	}()

	if token == "" {
		if err := hnd.sendCloseWSocket(codes["403"], "no token"); err != nil {
			hnd.logger.Err(err).Msg("failed to send close")
		}
		return
	}

	go hnd.listenConnection(ctx)

	hnd.aggregator = aggregator.NewAggregatorStatusHandler(ctx, hnd.env, hnd.logger, hnd.router, hnd.token)

loop:
	for {
		select {
		case msg := <-hnd.aggregator.RespMessageAggregate:
			if closeCode := msg.GetCloseCode(); closeCode != 0 {
				hnd.eventsCloseChan <- &closeEvent{
					force:  false,
					reason: nil,
					code:   closeCode,
				}
				continue loop
			}

			hnd.logger.Debug().Msgf("try to send messsage to ws client, id %s", msg.Id)
			if closed, err := hnd.sendMessage(msg); err != nil {
				hnd.logger.Err(err).Msg("failed to send message")
				if closed {
					hnd.eventsCloseChan <- &closeEvent{
						force:  false,
						reason: nil,
						code:   500,
					}
					continue loop
				}
			}

		case msg := <-hnd.inputChan:
			hnd.logger.Debug().Msgf("Reciv msg  %s\n", time.Now().String()) //rem
			hnd.aggregator.SubscribeDevices(ctx, msg.Ids)

		case <-hnd.pinger.C:
			hnd.logger.Debug().Msg("WS: time to ping client")

			if err := hnd.sendPingPongMessage(gws.OpPing); err != nil {
				hnd.logger.Err(err).Msg("ping fail")
				break loop
			}
			hnd.logger.Debug().Msg("ping sent")

		case <-hnd.pongChan:
			if err := hnd.sendPingPongMessage(gws.OpPong); err != nil {
				hnd.logger.Err(err).Msg("pong fail")
				break loop
			}
			hnd.logger.Debug().Msg("pong sent")

		case evt := <-hnd.eventsCloseChan:
			defer (hnd.conn).Close()
			if evt.force {
				hnd.logger.Warn().Err(evt.reason).Msg("force close requested")

				if err := (hnd.conn).Close(); err != nil {
					hnd.logger.Err(err).Msg("failed to forcibly close connection")
				}
				break loop
			}

			errMsg := ""
			if evt.reason != nil {
				errMsg = evt.reason.Error()
			}

			if err := hnd.sendCloseWSocket(evt.code, errMsg); err != nil {
				hnd.logger.Err(err).Msg("failed to gracefully close conection, will force")
				hnd.eventsCloseChan <- &closeEvent{
					force:  true,
					reason: err,
					code:   500,
				}

			}

			hnd.logger.Debug().Msg("close conn")
			break loop

		case <-ctx.Done():
			break loop
		}
	}
}

func (hnd *DeviceStatusWorker) sendPingPongMessage(Op gws.OpCode) (err error) {

	if Op != gws.OpPing && Op != gws.OpPong {
		return fmt.Errorf("wrong OpCode")
	}

	if err = (hnd.conn).SetWriteDeadline(time.Now().Add(writeDeadline)); err != nil {
		return fmt.Errorf("failed to set write deadline: %w", err)
	}

	writer := wsutil.NewWriter(hnd.conn, state, Op)
	var buf []byte
	if err = hnd.writeBuffer(writer, &buf); err != nil {
		err = fmt.Errorf("failed to send ping-pong, will close connection, %w", err)
		hnd.eventsCloseChan <- &closeEvent{
			force:  false,
			reason: fmt.Errorf("failed to send ping-pong: %w", err),
			code:   500,
		}
	}
	return nil
}

func (hnd *DeviceStatusWorker) listenConnection(ctx context.Context) {

	reader := wsutil.NewReader((hnd.conn), state)
	decoder := json.NewDecoder(reader)

loop:
	for {

		if ctx.Err() != nil {
			break loop
		}
		if err := (hnd.conn).SetReadDeadline(time.Now().Add(readDeadline)); err != nil {
			hnd.eventsCloseChan <- &closeEvent{
				force:  true,
				reason: fmt.Errorf("failed to initially set read deadline: %w", err),
				code:   500,
			}
			break loop
		}

		hdr, err := reader.NextFrame()

		switch {

		case err != nil:
			hnd.logger.Err(err).Msg("read error")
			hnd.eventsCloseChan <- &closeEvent{
				force:  true,
				reason: fmt.Errorf("read error %w", err),
				code:   500,
			}
			break loop

		case hdr.OpCode == gws.OpClose:
			hnd.eventsCloseChan <- &closeEvent{
				force:  true,
				reason: fmt.Errorf("client send close event"),
				code:   200,
			}
			break loop

		case hdr.OpCode == gws.OpPong:
			hnd.logger.Debug().Msgf("got pong")
			continue

		case hdr.OpCode == gws.OpPing:
			hnd.logger.Debug().Msgf("got ping, we will send pong")
			hnd.pongChan <- true
			continue

		case hdr.OpCode == gws.OpText:
			var req model.RequestMessage
			if err := decoder.Decode(&req); err != nil {
				hnd.logger.Err(err).Msg("failed to decode json")
				continue loop
			}
			hnd.inputChan <- &req

		default:
			hnd.logger.Err(err).Msgf("Strange ws.opCode:%d", hdr.OpCode)
		}
	}

}

//Return true if conn is closed. Msg == model.ResponseMessage or model.CloseMessage
func (hnd *DeviceStatusWorker) sendMessage(msg interface{}) (bool, error) {

	if err := (hnd.conn).SetWriteDeadline(time.Now().Add(writeDeadline)); err != nil {
		return true, fmt.Errorf("failed to set write deadline: %w", err)
	}
	writer := wsutil.NewWriter((hnd.conn), state, gws.OpText)

	var encoder = json.NewEncoder(writer)

	if err := encoder.Encode(&msg); err != nil {
		return false, fmt.Errorf("failed to encode json: %w", err)
	}
	if err := writer.Flush(); err != nil {
		return true, fmt.Errorf("failed to write ws: %w", err)
	}

	return false, nil
}

func (hnd *DeviceStatusWorker) sendCloseWSocket(code int, text string) error {

	msg := model.NewCloseMessage(code, text)
	if _, err := hnd.sendMessage(msg); err != nil {
		return err
	}
	w := wsutil.NewWriter((hnd.conn), gws.StateServerSide, gws.OpClose)
	frameBody := gws.NewCloseFrameBody(gws.StatusCode(code), text)
	frame := gws.NewCloseFrame(frameBody)
	err := gws.WriteFrame(w, frame)
	return err
}

func (hnd *DeviceStatusWorker) writeBuffer(writer *wsutil.Writer, buffer *[]byte) error {
	var err error

	if _, err = writer.Write(*buffer); err != nil {
		return err
	}

	if err = writer.Flush(); err != nil {
		return err
	}

	return nil
}
