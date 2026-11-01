package middleware

import (
	"crypto/sha1"
	"encoding/hex"
	"net/http"
	"net/http/httptest"
	"testing"

	"IM/commons/actor"
	"IM/commons/errs"

	"github.com/gin-gonic/gin"
)

func init() { gin.SetMode(gin.TestMode) }

func testEngine(lookup SecretLookup) *gin.Engine {
	r := gin.New()
	g := r.Group("/apigateway")
	g.Use(Signature(lookup))
	g.GET("/ping", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"app_key": actor.AppKeyFrom(c.Request.Context())})
	})
	return r
}

func lookup(appKey string) (string, bool, error) {
	if appKey == "app-1" {
		return "secret-1", true, nil
	}
	return "", false, nil
}

func sign(secret, nonce, ts string) string {
	sum := sha1.Sum([]byte(secret + nonce + ts))
	return hex.EncodeToString(sum[:])
}

func do(r http.Handler, headers map[string]string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(http.MethodGet, "/apigateway/ping", nil)
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	return w
}

func TestMissingHeaders(t *testing.T) {
	r := testEngine(lookup)
	cases := []struct {
		name string
		h    map[string]string
		code int
	}{
		{"appkey", nil, errs.CodeAppKeyRequired},
		{"nonce", map[string]string{"appkey": "app-1"}, errs.CodeNonceRequired},
		{"timestamp", map[string]string{"appkey": "app-1", "nonce": "n"}, errs.CodeTimestampRequired},
		{"signature", map[string]string{"appkey": "app-1", "nonce": "n", "timestamp": "1"}, errs.CodeSignatureRequired},
	}
	for _, tc := range cases {
		w := do(r, tc.h)
		if w.Code != 400 || !containsCode(w.Body.String(), tc.code) {
			t.Fatalf("%s: status %d body %s", tc.name, w.Code, w.Body.String())
		}
	}
}

func TestAppMissingAndBadSignature(t *testing.T) {
	r := testEngine(lookup)
	h := map[string]string{"appkey": "missing", "nonce": "n", "timestamp": "1", "signature": "abcd"}
	w := do(r, h)
	if w.Code != 500 || !containsCode(w.Body.String(), errs.CodeAppNotExisted) {
		t.Fatalf("missing app: %d %s", w.Code, w.Body.String())
	}
	h["appkey"] = "app-1"
	w = do(r, h)
	if w.Code != 403 || !containsCode(w.Body.String(), errs.CodeSignatureFail) {
		t.Fatalf("bad sign: %d %s", w.Code, w.Body.String())
	}
}

func TestPass(t *testing.T) {
	r := testEngine(lookup)
	w := do(r, map[string]string{
		"appkey": "app-1", "nonce": "n", "timestamp": "1",
		"signature": sign("secret-1", "n", "1"),
	})
	if w.Code != 200 || w.Body.String() != `{"app_key":"app-1"}` {
		t.Fatalf("pass: %d %s", w.Code, w.Body.String())
	}
}

func containsCode(body string, code int) bool {
	return len(body) > 0 && (body == jsonCode(code) || contains(body, code))
}

func contains(body string, code int) bool {
	want := `"code":` + itoa(code)
	return len(body) >= len(want) && (indexOf(body, want) >= 0)
}

func jsonCode(code int) string { return "" }

func indexOf(s, sub string) int {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return i
		}
	}
	return -1
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	var b [8]byte
	i := len(b)
	for n > 0 {
		i--
		b[i] = byte('0' + n%10)
		n /= 10
	}
	return string(b[i:])
}
