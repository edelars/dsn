package worker

import (
	"context"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/url"
	"time"

	gws "github.com/gobwas/ws"
	"github.com/gobwas/ws/wsutil"
	uuid "github.com/gofrs/uuid"
	"github.com/rs/zerolog"
	"gl.dev.boquar.com/backend/device-status-aggregator/pkg/config"
	"gl.dev.boquar.com/backend/device-status-aggregator/pkg/model"
)

const (
	readDeadline   = 30 * time.Second
	writeDeadline  = 15 * time.Second
	pingPeriod     = (readDeadline * 9) / 10
	respawnTimeout = 5 * time.Second
	state          = gws.StateClientSide
)

type RequesterStatusHandler struct {
	id       uuid.UUID
	env      config.Environment
	stopChan chan bool
	logger   zerolog.Logger
}

func NewRequesterStatusHandler(id uuid.UUID, env config.Environment, logger *zerolog.Logger) *RequesterStatusHandler {
	return &RequesterStatusHandler{
		id:       id,
		env:      env,
		logger:   logger.With().Str("DEVICE_ID", id.String()).Logger(),
		stopChan: make(chan bool, 2),
	}
}

func (hnd *RequesterStatusHandler) Run(ctx context.Context, respMessagechan chan *model.ResponseMessage) {

	go func() {

		var isStop bool
		defer func() {
			if !isStop && ctx.Err() == nil {
				hnd.respawn(ctx, respMessagechan)
			}
		}()

		u := url.URL{
			Scheme:   "ws",
			Host:     hnd.env.DSNHostPort,
			Path:     fmt.Sprintf("/ws/status/%s", hnd.id.String()),
			RawQuery: fmt.Sprintf("%s=200", hnd.env.AuthFieldName),
		}

		hnd.logger.Debug().Msgf("Trying to connect to DSN: URL %s", u.String())

		dialer := gws.Dialer{
			Timeout: 5 * time.Second,
		}
		conn, _, _, err := dialer.Dial(ctx, u.String())

		defer func() {
			if conn != nil {
				conn.Close()
			}
		}()

		if err != nil {
			hnd.logger.Debug().Msgf("Error connect to DSN: %s", err)
			respMessagechan <- model.NewErrorResponseMessageInternalError(err, hnd.id)
			return
		}
		hnd.logger.Debug().Msgf("Connected to DSN")

		r := wsutil.NewReader(conn, state)
	loop:
		for {

			select {
			case isStop = <-hnd.stopChan:
				if isStop {
					hnd.sendCloseWSocket(&conn)
					hnd.logger.Debug().Msgf("Receive stopChan")
					break loop
				}

			case <-ctx.Done():
				hnd.sendCloseWSocket(&conn)
				break loop

			default:
				if err := conn.SetReadDeadline(time.Now().Add(readDeadline)); err != nil {
					break loop
				}

				hdr, err := r.NextFrame()

				hnd.logger.Debug().Msgf("Receive NextFrame DSN")

				switch {

				case err != nil:
					hnd.logger.Err(err).Msgf("read error from websocket DSN")
					break loop

				case hdr.OpCode == gws.OpClose:
					code, msg, err := hnd.readClosePacketBody(&conn, hdr)
					if err != nil {
						hnd.logger.Err(err).Msgf("error read close packet")
					}
					hnd.logger.Debug().Msgf("Receive close packet from DSN: code %d", code)
					respMessagechan <- model.NewErrorResponseMessage(code, msg, hnd.id)
					break loop

				case hdr.OpCode == gws.OpPong:
					hnd.logger.Debug().Msgf("Receive pong packet from DSN")
					continue loop

				case hdr.OpCode == gws.OpPing:
					hnd.logger.Debug().Msgf("Receive ping packet from DSN")
					if err := conn.SetWriteDeadline(time.Now().Add(writeDeadline)); err != nil {
						hnd.logger.Err(err).Msgf("failed to set write deadline")
						break loop
					}
					writer := wsutil.NewWriter(conn, state, gws.OpPong)
					var buf []byte
					if err := hnd.writeBuffer(writer, buf); err != nil {
						hnd.logger.Err(err).Msgf("failed to send ping DSN, will close connection")
						break loop
					}
					continue loop

				case hdr.OpCode == gws.OpText:
					var req model.DeviceStatusFromDSN
					decoder := json.NewDecoder(r)
					if err := decoder.Decode(&req); err != nil {
						hnd.logger.Err(err).Msgf("failed to decode json from DSN")
						continue loop
					}
					respMessagechan <- req.ResponseMessage(hnd.id)

				default:
					hnd.logger.Err(err).Msgf("Strange ws.opCode:%d", hdr.OpCode)
				}
			}

		}
	}()

}

//Will close conn after send packet
func (hnd *RequesterStatusHandler) sendCloseWSocket(conn *net.Conn) {
	w := wsutil.NewWriter(*conn, state, gws.OpClose)
	buf := make([]byte, 0)
	w.Write(buf)
	w.Flush()
}

func (hnd *RequesterStatusHandler) Stop() {
	hnd.logger.Debug().Msgf("Make stop to receive info from DSN, id: %s", hnd.id)
	hnd.stopChan <- true
}

func (hnd *RequesterStatusHandler) respawn(ctx context.Context, respMessagechan chan *model.ResponseMessage) {
	hnd.logger.Debug().Msgf("Respawn RequesterStatusHandler with timeout: %d", respawnTimeout)
	time.Sleep(respawnTimeout)
	hnd.Run(ctx, respMessagechan)
}

func (hnd *RequesterStatusHandler) writeBuffer(writer *wsutil.Writer, buffer []byte) error {
	var err error

	if _, err = writer.Write(buffer); err != nil {
		return err
	}

	if err = writer.Flush(); err != nil {
		return err
	}

	return nil
}

func (hnd *RequesterStatusHandler) readClosePacketBody(conn *net.Conn, hdr gws.Header) (code uint16, msg error, err error) {

	payload := make([]byte, hdr.Length)
	_, err = io.ReadFull(*conn, payload)
	if err != nil {
		err = fmt.Errorf("read error from websocket DSN")
	}

	code = uint16(5000)
	if len(payload) > 1 {
		code = binary.BigEndian.Uint16(payload[:2])
	}

	msg = fmt.Errorf("internal error") //TODO read error from socket
	return code, msg, err
}
