package model

import (
	"encoding/json"

	uuid "github.com/gofrs/uuid"
)

type RequestMessage struct {
	TypeReq string      `json:"type"`
	Ids     []uuid.UUID `json:"ids"`
}

type ErrorResponseMessage struct {
	TypeRes string  `json:"type"`
	Reason  *string `json:"reason,omitempty"`
}

type ResponseMessage struct {
	closeCode      int
	TypeRes        string                `json:"type"`
	Id             string                `json:"id"`
	Online         *bool                 `json:"online,omitempty"`
	ExtendedStatus *json.RawMessage      `json:"extendedStatus,omitempty"`
	ErrorResp      *ErrorResponseMessage `json:"error,omitempty"`
}

type DeviceStatusFromDSN struct {
	Status          int              `json:"status"`
	DeviceTelemetry *json.RawMessage `json:"extendedStatus"`
}

type CloseMessage struct {
	TypeRes string `json:"type"`
	Code    int    `json:"code"`
	Reason  string `json:"reason"`
}
