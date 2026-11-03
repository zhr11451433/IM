package middleware_test

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"IM/commons/errs"
	"IM/commons/token"
	"IM/services/navigator/middleware"

	"github.com/gin-gonic/gin"
)

func TestCheckTokenExpiry(t *testing.T) {
	gin.SetMode(gin.TestMode)
	const key = "1234567890abcdef"

	tests := []struct {
		name       string
		age        time.Duration
		minutes    int
		configErr  bool
		wantStatus int
		wantCode   int
		wantPass   bool
	}{
		{"未过期", 0, 1, false, 200, 0, true},
		{"已过期", 2 * time.Minute, 1, false,
			200, errs.CodeConnectTokenExpired, false},
		{"不限制有效期", 24 * time.Hour, 0, false,
			200, 0, true},
		{"配置查询失败", 0, 1, true, 500, 0, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tok, err := token.Issue(
				"app-a", key, "u1", time.Now().Add(-tt.age),
			)
			if err != nil {
				t.Fatal(err)
			}

			reached := false
			r := gin.New()
			r.POST("/navigator/general",
				middleware.CheckToken(
					func(string) (string, bool, error) {
						return key, true, nil
					},
					func(string) (int, error) {
						if tt.configErr {
							return 0, errors.New("config unavailable")
						}
						return tt.minutes, nil
					},
				),
				func(c *gin.Context) {
					reached = true
					if c.GetString(middleware.CtxAppKey) != "app-a" ||
						c.GetString(middleware.CtxUserID) != "u1" {
						t.Error("verified identity missing")
					}
					c.JSON(http.StatusOK, gin.H{"code": 0})
				},
			)

			req := httptest.NewRequest(
				http.MethodPost, "/navigator/general", nil,
			)
			req.Header.Set("x-appkey", "app-a")
			req.Header.Set("x-token", tok)

			w := httptest.NewRecorder()
			r.ServeHTTP(w, req)

			if w.Code != tt.wantStatus || reached != tt.wantPass {
				t.Fatalf("status=%d reached=%v body=%s",
					w.Code, reached, w.Body.String())
			}
			if tt.wantStatus == http.StatusInternalServerError {
				return
			}

			var body struct {
				Code *int `json:"code"`
			}
			if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
				t.Fatal(err)
			}
			if body.Code == nil || *body.Code != tt.wantCode {
				t.Fatalf("unexpected code: %s", w.Body.String())
			}
		})
	}
}
