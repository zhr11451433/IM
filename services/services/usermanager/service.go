package usermanager

import (
	"IM/commons/actor"
	"IM/commons/db"
	"IM/commons/token"
	"context"
	"errors"
	"time"

	"gorm.io/gorm"
)

//service

type RegisterResult struct {
	UserID string `json:"user_id"`
	Token  string `json:"token"`
}

type UserInfo struct {
	UserID       string `json:"user_id"`
	Nickname     string `json:"nickname"`
	UserPortrait string `json:"user_portrait"`
}

var ErrUserNotExist = errors.New("user not exist")

//用户管理模块初始化时，会向 Runtime 注册两个消息处理器（两个信箱）

func Register(rt *actor.Runtime, gdb *gorm.DB) error {
	//以后有人向 reg_user 发送消息，就执行我提供的这个函数
	if err := rt.Register("reg_user", func(ctx context.Context, msg *actor.Message) (any, error) {
		in := msg.Payload.(db.User)
		var app db.App
		err := gdb.Where("app_key = ?", msg.AppKey).Take(&app).Error
		if err != nil {
			return nil, err
		}
		if err := db.UpsertUser(gdb, msg.AppKey, in.UserID, in.Nickname, in.UserPortrait); err != nil {
			return nil, err
		}
		tok, err := token.Issue(msg.AppKey, app.AppSecureKey, in.UserID, time.Now())
		if err != nil {
			return nil, err
		}
		return RegisterResult{
			UserID: in.UserID,
			Token:  tok,
		}, nil
	}); err != nil {
		return err
	}
	return rt.Register("qry_user_info", func(ctx context.Context, msg *actor.Message) (any, error) {
		user, ok, err := db.FindUser(gdb, msg.AppKey, msg.TargetID)
		if err != nil {
			return nil, err
		}
		if !ok {
			return nil, ErrUserNotExist
		}
		return UserInfo{UserID: user.UserID, Nickname: user.Nickname, UserPortrait: user.UserPortrait}, nil
	})
}
