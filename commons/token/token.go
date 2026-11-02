package token

import (
	"IM/commons/crypto"
	"encoding/base64"
	"encoding/json/v2"
	"errors"
	"time"
)

var (
	ErrIllegal = errors.New("token illegal")
	ErrExpired = errors.New("token expired")
	ErrAuth    = errors.New("token auth fail")
)

type value struct {
	UserID    string `json:"userId"`
	DeviceID  string `json:"deviceId"`
	TokenTime int64  `json:"tokenTime"`
	UserType  int32  `json:"userType"`
}

type wrap struct {
	AppKey     string `json:"appKey"`
	TokenValue string `json:"tokenValue"`
}

func Issue(appKey, secureKey, userID string, now time.Time) (string, error) {
	if len(secureKey) != 16 {
		return "", errors.New("app_secure_key must be 16 bytes")
	}
	raw, err := json.Marshal(value{UserID: userID, TokenTime: now.UnixMilli()})
	if err != nil {
		return "", err
	}
	enc, err := crypto.Encrypt(raw, []byte(secureKey))
	if err != nil {
		return "", err
	}
	body, err := json.Marshal(wrap{AppKey: appKey, TokenValue: base64.StdEncoding.EncodeToString(enc)})
	if err != nil {
		return "", err
	}
	return base64.URLEncoding.EncodeToString(body), nil
}

func Parse(appKey, secureKey, tokenStr string) (string, error) {
	return ParseWithExpiry(appKey, secureKey, tokenStr, 0, time.Now())
}

func ParseWithExpiry(
	appKey, secureKey, tokenStr string,
	effectiveMinutes int,
	now time.Time,
) (string, error) {
	body, err := base64.URLEncoding.DecodeString(tokenStr)
	if err != nil {
		return "", ErrIllegal
	}
	var w wrap
	if err := json.Unmarshal(body, &w); err != nil {
		return "", ErrIllegal
	}
	if w.AppKey != appKey {
		return "", ErrAuth
	}
	if len(secureKey) != 16 {
		return "", ErrAuth
	}
	enc, err := base64.StdEncoding.DecodeString(w.TokenValue)
	if err != nil {
		return "", ErrIllegal
	}
	raw, err := crypto.Decrypt(enc, []byte(secureKey))
	if err != nil {
		return "", ErrAuth
	}
	var v value
	if err := json.Unmarshal(raw, &v); err != nil || v.UserID == "" {
		return "", ErrAuth
	}
	if effectiveMinutes > 0 {
		expiredAt := time.UnixMilli(v.TokenTime).
			Add(time.Duration(effectiveMinutes) * time.Minute)

		if expiredAt.UnixMilli() < now.UnixMilli() {
			return "", ErrExpired
		}
	}
	return v.UserID, nil
}
