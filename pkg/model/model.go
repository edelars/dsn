package model

import (
	"fmt"

	uuid "github.com/gofrs/uuid"
)

func (hnd *DeviceStatusFromDSN) ResponseMessage(id uuid.UUID) *ResponseMessage {

	online := hnd.Status > 0
	responseMessage := ResponseMessage{
		TypeRes:        "status",
		Id:             id.String(),
		Online:         &online,
		ExtendedStatus: hnd.DeviceTelemetry,
	}

	return &responseMessage
}

func NewErrorResponseMessage(codeError uint16, err error, id uuid.UUID) *ResponseMessage {

	typeRes := "GENERIC"

	if codeError == 4001 || codeError == 4003 {
		typeRes = "NOT_FOUND"
	}

	ErrorResp := ErrorResponseMessage{
		TypeRes: typeRes,
	}
	if typeRes == "GENERIC" && err != nil {
		errStr := err.Error()
		ErrorResp.Reason = &errStr
	}
	responseMessage := ResponseMessage{
		TypeRes:   "sub-nack",
		Id:        id.String(),
		ErrorResp: &ErrorResp,
	}

	return &responseMessage
}

func NewErrorResponseMessageNoAccess(err error, id uuid.UUID) *ResponseMessage {
	return NewErrorResponseMessage(4003, err, id)
}

func NewErrorResponseMessageInternalError(err error, id uuid.UUID) *ResponseMessage {
	if err == nil {
		err = fmt.Errorf("internal error")
	}
	return NewErrorResponseMessage(5000, err, id)
}

func NewErrorResponseMessageTokenOutdated() *ResponseMessage {
	return &ResponseMessage{
		closeCode: 4001,
	}
}

func (m *ResponseMessage) GetCloseCode() int {
	return m.closeCode
}

func NewCloseMessage(code int, reason string) *CloseMessage {
	return &CloseMessage{
		TypeRes: "close",
		Code:    code,
		Reason:  reason,
	}
}
