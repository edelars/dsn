package main_test

import (
	"context"
	"encoding/binary"
	"encoding/json"
	"net/http"
	ur "net/url"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/gobwas/ws"
	"github.com/gobwas/ws/wsutil"
	uuid "github.com/gofrs/uuid"
	"github.com/gorilla/mux"
	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
	"github.com/stretchr/testify/assert"
	wsAPI "gl.dev.boquar.com/backend/device-status-aggregator/pkg/api/ws"
	"gl.dev.boquar.com/backend/device-status-aggregator/pkg/config"
	"gl.dev.boquar.com/backend/device-status-aggregator/pkg/model"
	"gl.dev.boquar.com/backend/device-status-aggregator/pkg/router"
)

const (
	ok_device = "74a7b5f6-369d-4d10-88e2-dbdff3f4a0b9"
)

func Test_Flow(t *testing.T) {

	//ctx, CancelF := context.WithCancel(context.Background())
	//	defer CancelF()

	var env config.Environment
	env.LogLevel = "debug"
	env.DSNHostPort = "127.0.0.1:8099"
	env.WebSocketPort = 8091
	env.RightVerifURL = "http://127.0.0.1:9090/check/"
	env.AuthFieldName = "auth_result"

	log.Logger = zerolog.New(os.Stderr).With().Str("DSA-test", "DSA").Timestamp().Caller().Logger()

	router := router.NewRouterHandler(&log.Logger, env)

	serverWS, err := wsAPI.NewServer(router, env)
	if err != nil {
		t.Error("unable to create websocket server")
	}

	rf := startRF(env.RightVerifURL)

	go func() {

		time.Sleep(time.Second * 5)
		u := ur.URL{
			Scheme:   "ws",
			Host:     "127.0.0.1:8091",
			Path:     "/ws/devices/status",
			RawQuery: "token=200",
		}

		ctx, CancelRM := context.WithCancel(context.Background())
		go RunMockServer(env.DSNHostPort, t, ctx, ok_device)
		//1
		online := true
		reqExp := &model.ResponseMessage{
			TypeRes: "status",
			Id:      ok_device,
			Online:  &online,
		}
		req := &model.ResponseMessage{}
		SendQuery(ctx, u.String(), t, uuid.FromStringOrNil(ok_device), "adsas", env, req)
		assert.Equal(t, reqExp, req)

		CancelRM()

		ctx, CancelRM = context.WithCancel(context.Background())
		go RunMockServer(env.DSNHostPort, t, ctx, ok_device)

		//2
		idDevice := uuid.Must(uuid.NewV4())
		reqExp2 := &model.ResponseMessage{
			TypeRes: "sub-nack",
			Id:      idDevice.String(),
			ErrorResp: &model.ErrorResponseMessage{
				TypeRes: "NOT_FOUND",
			},
		}
		req2 := &model.ResponseMessage{}
		SendQuery(ctx, u.String(), t, idDevice, "adsas", env, req2)
		assert.Equal(t, reqExp2, req2)
		CancelRM()

		ctx, CancelRM = context.WithCancel(context.Background())
		go RunMockServer(env.DSNHostPort, t, ctx, ok_device)

		//3
		u.RawQuery = ""
		req3 := &model.CloseMessage{}
		SendQuery(ctx, u.String(), t, uuid.FromStringOrNil(ok_device), "adsas", env, req3)
		assert.Equal(t, &model.CloseMessage{TypeRes: "close", Code: 4003, Reason: "no token"}, req3)
		serverWS.Shutdown(ctx)

		CancelRM()

		rf.Shutdown(ctx)
		serverWS.Shutdown(ctx)
	}()

	serverWS.ListenAndServe()
}

func SendQuery(ctx context.Context, urlstr string, t *testing.T, id uuid.UUID, token string, env config.Environment, req interface{}) {

	dialer := ws.Dialer{}
	dialer.Timeout = 2 * time.Second
	conn, _, _, err := dialer.Dial(ctx, urlstr)

	if err != nil {
		t.Error(err)
	}

	defer conn.Close()

	msg := model.RequestMessage{
		TypeReq: "asd",
	}

	msg.Ids = append(msg.Ids, id)
	writer := wsutil.NewWriter(conn, ws.StateClientSide, ws.OpText)
	var encoder = json.NewEncoder(writer)
	if err := encoder.Encode(&msg); err != nil {
		t.Error("failed to encode json: ")
	}
	if err := writer.Flush(); err != nil {
		t.Error("failed to write ws: ")
	}
	assert.NoError(t, err)

	reader := wsutil.NewReader(conn, ws.StateClientSide)
	decoder := json.NewDecoder(reader)

	//req := model.ResponseMessage{}

	hdr, err := reader.NextFrame()
	if hdr.OpCode == ws.OpClose {
		return
	}
	if hdr.OpCode != ws.OpText {
		return
	}

	assert.NoError(t, err)

	err = decoder.Decode(&req)
	assert.NoError(t, err)

	err = writer.Flush()
	assert.NoError(t, err)

	conn.Close()
	return
}

//emulate DSN server
func RunMockServer(hostport string, t *testing.T, ctx context.Context, okDevId string) {
	http.ListenAndServe(hostport, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, _, _, err := ws.UpgradeHTTP(r, w)
		assert.NoError(t, err)
		id := strings.Split(r.URL.Path, "/")[3]
		if id != okDevId {
			buf := make([]byte, 2+len("get out"))
			binary.BigEndian.PutUint16(buf, uint16(4001))
			copy(buf[2:], "get out")

			writer := wsutil.NewWriter(conn, ws.StateServerSide, ws.OpClose)
			_, err = writer.Write(buf)
			assert.NoError(t, err)
			err = writer.Flush()
			assert.NoError(t, err)

		}

		go func() {
			defer conn.Close()

			var (
				writer  = wsutil.NewWriter(conn, ws.StateServerSide, ws.OpText)
				encoder = json.NewEncoder(writer)
			)

			//	for {
			time.Sleep(time.Second * 1)
			if ctx.Err() != nil {
				return
			}
			err := conn.SetWriteDeadline(time.Now().Add(15 * time.Second))

			assert.NoError(t, err)

			resp := model.DeviceStatusFromDSN{
				Status: 1,
			}
			err = encoder.Encode(&resp)
			assert.NoError(t, err)

			err = writer.Flush()
			assert.NoError(t, err)

			//	time.Sleep(time.Second * 2)
			//}
		}()
	}))
}

func startRF(url string) *http.Server {

	u, _ := ur.Parse(url)
	m := mux.NewRouter()

	m.HandleFunc(u.Path+"{projectId}", func(rw http.ResponseWriter, req *http.Request) {
		rw.WriteHeader(http.StatusOK)
	})

	srv := http.Server{
		Addr:    u.Host,
		Handler: m,
	}

	go srv.ListenAndServe()
	return &srv
}
