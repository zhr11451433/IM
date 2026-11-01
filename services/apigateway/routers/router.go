package routers

import (
	"net/http"

	"IM/commons/actor"
	"IM/commons/db"
	"IM/services/apigateway/apis"
	"IM/services/apigateway/middleware"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

func SetRouter(gdb *gorm.DB, rt *actor.Runtime) *gin.Engine {
	r := gin.Default()
	r.GET("/", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{
			"status": "ok",
		})
	})
	api := r.Group("/apigateway")
	api.Use(middleware.Signature(func(appKey string) (string, bool, error) {
		return db.FindAppSecret(gdb, appKey)
	}))
	api.POST("/users/register", apis.Register(rt))
	api.POST("/users/info", apis.QryUserInfo(rt))
	return r
}
