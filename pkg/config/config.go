package config

type Environment struct {
	LogLevel          string `long:"log-level" env:"LOG_LEVEL" required:"false" default:"debug"`
	WebSocketPort     int    `long:"websocket-port" env:"WS_PORT" required:"true" default:"8089"`
	DSNHostPort       string `long:"dsn-host-port" env:"DSN_HOST_PORT" required:"true" default:"localhost:8999"`
	RightVerifURL     string `long:"rf-url" env:"RF_URL" required:"true" default:""`
	AuthFieldName     string `long:"AuthFieldName" env:"AUTH_FIELD_NAME" required:"false" default:"auth_result"`
	RightVerifSkipTLS bool   `long:"RfSkipTLS" env:"RF_SKIP_TLS" required:"false"`
}
