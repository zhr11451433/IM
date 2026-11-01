package middleware

import (
	"IM/commons/actor"
	"IM/commons/errs"
	"crypto/sha1"
	"encoding/hex"
	"net/http"

	"github.com/gin-gonic/gin"
)

type SecretLookup func(appKey string) (secret string, ok bool, err error)

func Signature(Lookup SecretLookup) gin.HandlerFunc {
	return func(c *gin.Context) {
		appKey := c.GetHeader("appkey")
		nonce := c.GetHeader("nonce")
		timestamp := c.GetHeader("timestamp")
		signature := c.GetHeader("signature")
		switch {
		case appKey == "":
			fail(c, errs.AppKeyRequired())
			return
		case nonce == "":
			fail(c, errs.NonceRequired())
			return
		case timestamp == "":
			fail(c, errs.TimestampRequired())
			return
		case signature == "":
			fail(c, errs.SignatureRequired())
			return
		}
		secret, ok, err := Lookup(appKey)
		if err != nil {
			c.AbortWithStatus(http.StatusInternalServerError)
			return
		}
		if !ok {
			fail(c, errs.AppNotExisted())
			return
		}
		sum := sha1.Sum([]byte(secret + nonce + timestamp))
		if hex.EncodeToString(sum[:]) != signature {
			fail(c, errs.SignatureFail())
			return
		}
		c.Set("app_key", appKey)
		c.Request = c.Request.WithContext(actor.WithAppKey(c.Request.Context(), appKey))
		c.Next()
	}
}

func fail(c *gin.Context, apiErr errs.APIError) {
	c.AbortWithStatusJSON(apiErr.HTTPCode, apiErr)
}
