package apis

import (
	"IM/services/usermanager"
	"errors"
	"net/http"
	"time"

	"IM/commons/actor"
	"IM/commons/db"
	"IM/commons/errs"

	"github.com/gin-gonic/gin"
)

//http层

type registerReq struct {
	UserID       string `json:"user_id"`
	Nickname     string `json:"nickname"`
	UserPortrait string `json:"user_portrait"`
}

func Register(rt *actor.Runtime) gin.HandlerFunc {
	return func(c *gin.Context) {
		var req registerReq
		if err := c.ShouldBindJSON(&req); err != nil || req.UserID == "" {
			c.AbortWithStatusJSON(400, errs.ReqBodyIllegal())
			return
		}
		appKey := actor.AppKeyFrom(c.Request.Context())
		ctx := actor.WithAppKey(c.Request.Context(), appKey)
		out, err := rt.Sync(ctx, "reg_user", req.UserID, db.User{
			UserID: req.UserID, Nickname: req.Nickname, UserPortrait: req.UserPortrait,
		}, 5*time.Second)
		if err != nil {
			writeCallErr(c, err)
			return
		}
		c.JSON(http.StatusOK, gin.H{"code": 0, "msg": "success", "data": out})
	}
}

func QryUserInfo(rt *actor.Runtime) gin.HandlerFunc {
	return func(c *gin.Context) {
		userID := c.Query("user_id")
		if userID == "" {
			c.AbortWithStatusJSON(400, errs.ParamRequired())
			return
		}
		appKey := actor.AppKeyFrom(c.Request.Context())
		ctx := actor.WithAppKey(c.Request.Context(), appKey)
		out, err := rt.Sync(ctx, "qry_user_info", userID, nil, 5*time.Second)
		if err != nil {
			writeCallErr(c, err)
			return
		}
		c.JSON(http.StatusOK, gin.H{"code": 0, "msg": "success", "data": out})
	}
}

func writeCallErr(c *gin.Context, err error) {
	switch {
	case errors.Is(err, actor.ErrTimeout):
		c.AbortWithStatusJSON(500, errs.InternalTimeout())
	case errors.Is(err, usermanager.ErrUserNotExist):
		c.AbortWithStatusJSON(200, errs.UserNotExist())
	default:
		c.AbortWithStatusJSON(500, errs.APIError{HTTPCode: 500, Code: 10009, Msg: "internal service error"})
	}
}
