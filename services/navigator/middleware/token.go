package middleware

import (
	"IM/commons/errs"
	"IM/commons/token"
	"errors"
	"net/http"

	"github.com/gin-gonic/gin"
)

const (
	CtxAppKey = "navigator.app_key"
	CtxUserID = "navigator.user_id"
)

// 返回应用的 app_secure_key，而不是 app_secret。
// found 区分“应用不存在”和“查询数据库失败”。

type SecureKeyLookup func(appKey string) (string, bool, error)

func reject(c *gin.Context, status, code int, msg string) {
	c.AbortWithStatusJSON(status, gin.H{
		"code": code,
		"msg":  msg,
	})
}

func CheckToken(lookup SecureKeyLookup) gin.HandlerFunc {
	return func(c *gin.Context) {
		appKey := c.GetHeader("x-appkey")
		tokenStr := c.GetHeader("x-token")
		if tokenStr == "" {
			reject(c, http.StatusUnauthorized,
				errs.CodeConnectTokenIllegal, "token illegal")
			return
		}
		if appKey == "" {
			reject(c, http.StatusOK,
				errs.CodeConnectAppNotExist, "app not exist")
			return
		}
		secureKey, ok, err := lookup(appKey)
		if err != nil {
			// 数据库故障不能当作“应用不存在”。
			c.AbortWithStatus(http.StatusInternalServerError)
			return
		}
		if !ok {
			reject(c, http.StatusOK,
				errs.CodeConnectAppNotExist, "app not exist")
			return
		}
		userID, err := token.Parse(appKey, secureKey, tokenStr)
		if err != nil {
			if errors.Is(err, token.ErrIllegal) {
				reject(c, http.StatusUnauthorized,
					errs.CodeConnectTokenIllegal, "token illegal")
			} else {
				reject(c, http.StatusUnauthorized,
					errs.CodeConnectTokenAuthFail, "token auth fail")
			}
			return
		}
		c.Set(CtxUserID, userID)
		c.Set(CtxAppKey, appKey)
		c.Next()
	}
}
