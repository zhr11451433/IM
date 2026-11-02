package routers

import (
	"IM/commons/db"
	"IM/services/navigator/apis"
	"IM/services/navigator/middleware"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

func SetRouter(gdb *gorm.DB, servers []string) *gin.Engine {
	r := gin.Default()
	nav := r.Group("/navigator")
	nav.Use(middleware.CheckToken(
		func(appKey string) (string, bool, error) {
			return db.FindAppSecureKey(gdb, appKey)
		},
	))
	nav.POST("/general", apis.General(servers))

	return r
}
