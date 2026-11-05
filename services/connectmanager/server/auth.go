package server

import (
	"IM/commons/errs"
	"IM/commons/token"
	"IM/services/connectmanager/server/codec"
	"context"
	"errors"
	"time"
)

func (s *ImWebsocketServer) authenticate(ctx context.Context, body *codec.ConnectMsgBody) (userID string, code int32, err error) {
	if body == nil || body.ProtoId != codec.ProtoID || body.Token == "" || body.Appkey == "" {
		return "", errs.CodeConnectParamIllegal, nil
	}
	authApp, ok, err := s.lookup(ctx, body.Appkey)
	if err != nil {
		return "", 0, err
	}
	if !ok {
		return "", errs.CodeConnectAppNotExist, nil
	}
	userID, err = token.ParseWithExpiry(body.Appkey, authApp.SecureKey, body.Token, authApp.EffectiveMinutes, time.Now())
	if err != nil {
		switch {
		case errors.Is(err, token.ErrIllegal):
			return "", errs.CodeConnectTokenIllegal, nil
		case errors.Is(err, token.ErrExpired):
			return "", errs.CodeConnectTokenExpired, nil
		default:
			return "", errs.CodeConnectTokenAuthFail, nil
		}
	}
	return userID, errs.CodeSuccess, nil
}
