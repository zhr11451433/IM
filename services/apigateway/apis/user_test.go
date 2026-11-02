package apis

import (
	"bytes"
	"context"
	"crypto/sha1"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"IM/commons/actor"
	"IM/commons/db"
	"IM/commons/errs"
	"IM/services/apigateway/middleware"
	"IM/services/services/usermanager"

	"github.com/gin-gonic/gin"
)

func init() { gin.SetMode(gin.TestMode) }

type storedUser struct {
	nickname string
	portrait string
}

func testRuntime(t *testing.T) *actor.Runtime {
	t.Helper()
	rt := actor.New()
	users := map[string]storedUser{}
	if err := rt.Register("reg_user", func(ctx context.Context, msg *actor.Message) (any, error) {
		in := msg.Payload.(db.User)
		users[msg.AppKey+"/"+in.UserID] = storedUser{nickname: in.Nickname, portrait: in.UserPortrait}
		return usermanager.RegisterResult{UserID: in.UserID, Token: "token-" + msg.AppKey + "-" + in.UserID}, nil
	}); err != nil {
		t.Fatal(err)
	}
	if err := rt.Register("qry_user_info", func(ctx context.Context, msg *actor.Message) (any, error) {
		user, ok := users[msg.AppKey+"/"+msg.TargetID]
		if !ok {
			return nil, usermanager.ErrUserNotExist
		}
		return usermanager.UserInfo{UserID: msg.TargetID, Nickname: user.nickname, UserPortrait: user.portrait}, nil
	}); err != nil {
		t.Fatal(err)
	}
	return rt
}

func testEngine(rt *actor.Runtime) *gin.Engine {
	r := gin.New()
	api := r.Group("/apigateway")
	api.Use(middleware.Signature(func(appKey string) (string, bool, error) {
		if appKey == "app-a" {
			return "secret-a", true, nil
		}
		if appKey == "app-b" {
			return "secret-b", true, nil
		}
		return "", false, nil
	}))
	api.POST("/users/register", Register(rt))
	api.POST("/users/info", QryUserInfo(rt))
	return r
}

func sign(secret, nonce, ts string) string {
	sum := sha1.Sum([]byte(secret + nonce + ts))
	return hex.EncodeToString(sum[:])
}

func call(r http.Handler, method, path, secret string, body any) *httptest.ResponseRecorder {
	var buf bytes.Buffer
	if body != nil {
		_ = json.NewEncoder(&buf).Encode(body)
	}
	req := httptest.NewRequest(method, path, &buf)
	req.Header.Set("appkey", "app-a")
	req.Header.Set("nonce", "n1")
	req.Header.Set("timestamp", "1000")
	req.Header.Set("signature", sign(secret, "n1", "1000"))
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	return w
}

func bodyCode(t *testing.T, w *httptest.ResponseRecorder) (int, map[string]any) {
	t.Helper()
	var payload map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &payload); err != nil {
		t.Fatalf("status %d body %s", w.Code, w.Body.String())
	}
	code, _ := payload["code"].(float64)
	return int(code), payload
}

func TestRegisterAndQuery(t *testing.T) {
	r := testEngine(testRuntime(t))

	w := call(r, http.MethodPost, "/apigateway/users/register", "secret-a", map[string]string{
		"user_id": "u1", "nickname": "一号", "user_portrait": "a.png",
	})
	code, payload := bodyCode(t, w)
	if w.Code != 200 || code != 0 {
		t.Fatalf("register: status %d body %s", w.Code, w.Body.String())
	}
	data := payload["data"].(map[string]any)
	if data["user_id"] != "u1" || data["token"] != "token-app-a-u1" {
		t.Fatalf("register data: %#v", data)
	}

	w = call(r, http.MethodPost, "/apigateway/users/register", "secret-a", map[string]string{
		"user_id": "u1", "nickname": "新昵称", "user_portrait": "b.png",
	})
	code, _ = bodyCode(t, w)
	if w.Code != 200 || code != 0 {
		t.Fatalf("second register: status %d body %s", w.Code, w.Body.String())
	}

	w = call(r, http.MethodPost, "/apigateway/users/info?user_id=u1", "secret-a", nil)
	code, payload = bodyCode(t, w)
	data = payload["data"].(map[string]any)
	if w.Code != 200 || code != 0 || data["nickname"] != "新昵称" || data["user_portrait"] != "b.png" {
		t.Fatalf("query after overwrite: status %d body %s", w.Code, w.Body.String())
	}
}

func TestRegisterRejectsBadBody(t *testing.T) {
	r := testEngine(testRuntime(t))
	cases := []struct {
		name string
		body string
	}{
		{"empty object", `{}`},
		{"blank user id", `{"user_id":""}`},
		{"not json", `{`},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodPost, "/apigateway/users/register", bytes.NewBufferString(tc.body))
			req.Header.Set("appkey", "app-a")
			req.Header.Set("nonce", "n1")
			req.Header.Set("timestamp", "1000")
			req.Header.Set("signature", sign("secret-a", "n1", "1000"))
			w := httptest.NewRecorder()
			r.ServeHTTP(w, req)
			code, _ := bodyCode(t, w)
			if w.Code != 400 || code != errs.CodeReqBodyIllegal {
				t.Fatalf("status %d body %s", w.Code, w.Body.String())
			}
		})
	}
}

func TestQueryMissingUser(t *testing.T) {
	r := testEngine(testRuntime(t))

	w := call(r, http.MethodPost, "/apigateway/users/info", "secret-a", nil)
	code, _ := bodyCode(t, w)
	if w.Code != 400 || code != errs.CodeParamRequired {
		t.Fatalf("missing user_id: status %d body %s", w.Code, w.Body.String())
	}

	w = call(r, http.MethodPost, "/apigateway/users/info?user_id=nobody", "secret-a", nil)
	code, payload := bodyCode(t, w)
	if w.Code != 200 || code != errs.CodeUserNotExist || payload["msg"] != "user not exist" {
		t.Fatalf("unknown user: status %d body %s", w.Code, w.Body.String())
	}
}

func TestOtherAppCannotSeeUser(t *testing.T) {
	r := testEngine(testRuntime(t))
	w := call(r, http.MethodPost, "/apigateway/users/register", "secret-a", map[string]string{
		"user_id": "u1", "nickname": "只属于A",
	})
	if w.Code != 200 {
		t.Fatal(w.Body.String())
	}

	req := httptest.NewRequest(http.MethodPost, "/apigateway/users/info?user_id=u1", nil)
	req.Header.Set("appkey", "app-b")
	req.Header.Set("nonce", "n1")
	req.Header.Set("timestamp", "1000")
	req.Header.Set("signature", sign("secret-b", "n1", "1000"))
	w = httptest.NewRecorder()
	r.ServeHTTP(w, req)
	code, _ := bodyCode(t, w)
	if w.Code != 200 || code != errs.CodeUserNotExist {
		t.Fatalf("app-b sees app-a user: status %d body %s", w.Code, w.Body.String())
	}
}

func TestRegisterRequiresSignature(t *testing.T) {
	r := testEngine(testRuntime(t))
	req := httptest.NewRequest(http.MethodPost, "/apigateway/users/register", bytes.NewBufferString(`{"user_id":"u1"}`))
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	code, _ := bodyCode(t, w)
	if w.Code != 400 || code != errs.CodeAppKeyRequired {
		t.Fatalf("no headers: status %d body %s", w.Code, w.Body.String())
	}

	req = httptest.NewRequest(http.MethodPost, "/apigateway/users/register", bytes.NewBufferString(`{"user_id":"u1"}`))
	req.Header.Set("appkey", "app-a")
	req.Header.Set("nonce", "n1")
	req.Header.Set("timestamp", "1000")
	req.Header.Set("signature", sign("wrong-secret", "n1", "1000"))
	w = httptest.NewRecorder()
	r.ServeHTTP(w, req)
	code, _ = bodyCode(t, w)
	if w.Code != 403 || code != errs.CodeSignatureFail {
		t.Fatalf("bad signature: status %d body %s", w.Code, w.Body.String())
	}
}
