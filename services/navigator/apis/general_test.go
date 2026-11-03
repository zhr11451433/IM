package apis_test

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"IM/commons/errs"
	"IM/commons/token"
	"IM/services/navigator/apis"
	"IM/services/navigator/middleware"

	"github.com/gin-gonic/gin"
)

func TestNavigatorGeneral(t *testing.T) {
	gin.SetMode(gin.TestMode)

	const secureKey = "1234567890abcdef"

	validToken, err := token.Issue(
		"app-a", secureKey, "u1", time.Now(),
	)
	if err != nil {
		t.Fatal(err)
	}

	// 应用相同，但密钥错误，应当无法通过解密校验。
	wrongKeyToken, err := token.Issue(
		"app-a", "abcdef1234567890", "u1", time.Now(),
	)
	if err != nil {
		t.Fatal(err)
	}

	tests := []struct {
		name       string
		appKey     string
		token      string
		wantStatus int
		wantCode   int
		wantPass   bool
	}{
		{
			name:   "合法 Token",
			appKey: "app-a", token: validToken,
			wantStatus: 200, wantCode: 0, wantPass: true,
		},
		{
			name:       "缺少 Token",
			appKey:     "app-a",
			wantStatus: 401, wantCode: errs.CodeConnectTokenIllegal,
		},
		{
			name:       "缺少应用头",
			token:      validToken,
			wantStatus: 200, wantCode: errs.CodeConnectAppNotExist,
		},
		{
			name:   "应用不存在",
			appKey: "unknown", token: validToken,
			wantStatus: 200, wantCode: errs.CodeConnectAppNotExist,
		},
		{
			name:   "Token 格式非法",
			appKey: "app-a", token: "not-base64",
			wantStatus: 401, wantCode: errs.CodeConnectTokenIllegal,
		},
		{
			name:   "跨应用使用 Token",
			appKey: "app-b", token: validToken,
			wantStatus: 401, wantCode: errs.CodeConnectTokenAuthFail,
		},
		{
			name:   "Token 加密密钥错误",
			appKey: "app-a", token: wrongKeyToken,
			wantStatus: 401, wantCode: errs.CodeConnectTokenAuthFail,
		},
		{
			name:   "数据库查询失败",
			appKey: "db-error", token: validToken,
			wantStatus: 500,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			lookup := func(appKey string) (string, bool, error) {
				switch appKey {
				case "app-a", "app-b":
					// 故意使用相同密钥，确保跨应用拒绝
					// 来自 appKey 校验，而不是密钥不同。
					return secureKey, true, nil
				case "db-error":
					return "", false, errors.New("database unavailable")
				default:
					return "", false, nil
				}
			}

			reached := false
			r := gin.New()
			nav := r.Group("/navigator")
			nav.Use(middleware.CheckToken(lookup, func(string) (int, error) { return 0, nil }))
			nav.POST("/general",
				func(c *gin.Context) {
					reached = true
					c.Next()
				},
				apis.General([]string{"ws://127.0.0.1:9003"}),
			)

			req := httptest.NewRequest(
				http.MethodPost, "/navigator/general", nil,
			)
			req.Header.Set("x-appkey", tc.appKey)
			req.Header.Set("x-token", tc.token)

			w := httptest.NewRecorder()
			r.ServeHTTP(w, req)

			if w.Code != tc.wantStatus {
				t.Fatalf("HTTP = %d, want %d; body=%s",
					w.Code, tc.wantStatus, w.Body.String())
			}
			if reached != tc.wantPass {
				t.Fatalf("后续处理函数执行 = %v, want %v",
					reached, tc.wantPass)
			}

			// 当前数据库错误响应只有 HTTP 状态，没有 JSON。
			if tc.wantStatus == http.StatusInternalServerError {
				return
			}

			var resp struct {
				Code *int                  `json:"code"`
				Data *apis.GeneralResponse `json:"data"`
			}
			if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
				t.Fatalf("响应不是合法 JSON: %v; body=%s",
					err, w.Body.String())
			}
			if resp.Code == nil || *resp.Code != tc.wantCode {
				t.Fatalf("业务码不符合预期，want %d; body=%s",
					tc.wantCode, w.Body.String())
			}

			if !tc.wantPass {
				if resp.Data != nil {
					t.Fatalf("鉴权失败却返回导航数据: %+v", resp.Data)
				}
				return
			}

			if resp.Data == nil {
				t.Fatal("成功响应缺少 data")
			}
			if resp.Data.AppKey != "app-a" || resp.Data.UserID != "u1" {
				t.Fatalf("身份传递错误: %+v", resp.Data)
			}
			if len(resp.Data.Servers) != 1 ||
				resp.Data.Servers[0] != "ws://127.0.0.1:9003" {
				t.Fatalf("连接地址错误: %v", resp.Data.Servers)
			}
		})
	}
}
