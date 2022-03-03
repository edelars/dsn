package main_test

import (
	"reflect"
	"testing"

	uuid "github.com/gofrs/uuid"
	"gl.dev.boquar.com/backend/device-status-aggregator/pkg/model"
)

func TestNewErrorResponseMessageFromDeviceStatusFromDSN(t *testing.T) {
	type args struct {
		codeError uint16
		err       error
		id        uuid.UUID
	}

	id := uuid.Must(uuid.NewV4())

	tests := []struct {
		name string
		args args
		want *model.ResponseMessage
	}{
		{
			name: "one",
			args: args{
				codeError: 4001,
				err:       nil,
				id:        id,
			},
			want: &model.ResponseMessage{
				TypeRes: "sub-nack",
				Id:      id.String(),
				ErrorResp: &model.ErrorResponseMessage{
					TypeRes: "NOT_FOUND",
				},
			},
		},
		{
			name: "two",
			args: args{
				codeError: 4003,
				err:       nil,
				id:        id,
			},
			want: &model.ResponseMessage{
				TypeRes: "sub-nack",
				Id:      id.String(),
				ErrorResp: &model.ErrorResponseMessage{
					TypeRes: "NOT_FOUND",
				},
			},
		},
		{
			name: "three",
			args: args{
				codeError: 9999,
				err:       nil,
				id:        id,
			},
			want: &model.ResponseMessage{
				TypeRes: "sub-nack",
				Id:      id.String(),
				ErrorResp: &model.ErrorResponseMessage{
					TypeRes: "GENERIC",
				},
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := model.NewErrorResponseMessage(tt.args.codeError, tt.args.err, tt.args.id); !reflect.DeepEqual(got, tt.want) {
				t.Errorf("NewErrorResponseMessageFromDeviceStatusFromDSN() = %v, want %v", got, tt.want)
			}
		})
	}
}
