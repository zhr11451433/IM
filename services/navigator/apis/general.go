package apis

import (
	"IM/services/navigator/middleware"
	"net/http"

	"github.com/gin-gonic/gin"
)

//导航处理函数

type GeneralResponse struct {
	AppKey  string   `json:"app_key"`
	UserID  string   `json:"user_id"`
	Servers []string `json:"servers"`
}

func General(servers []string) gin.HandlerFunc {
	return func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{
			"code": 0,
			"msg":  "success",
			"data": GeneralResponse{
				AppKey:  c.GetString(middleware.CtxAppKey),
				UserID:  c.GetString(middleware.CtxUserID),
				Servers: servers,
			},
		})
	}
}
