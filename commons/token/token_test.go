package token

import (
	"testing"
	"time"
)

func TestIssueAndParse(t *testing.T) {
	tok, err := Issue("app-a", "1234567890abcdef", "u1", time.UnixMilli(1))
	if err != nil {
		t.Fatal(err)
	}
	uid, err := Parse("app-a", "1234567890abcdef", tok)
	if err != nil {
		t.Fatal(err)
	}
	if uid != "u1" {
		t.Fatalf("user id = %s", uid)
	}
}

func TestParseRejectsOtherAppBeforeDecrypt(t *testing.T) {
	tok, err := Issue("app-a", "1234567890abcdef", "u1", time.UnixMilli(1))
	if err != nil {
		t.Fatal(err)
	}
	// 外层 appKey 是 app-a。拿到 app-b 上校验时，密钥即使和签发时相同，也要先失败。
	if _, err := Parse("app-b", "1234567890abcdef", tok); err != ErrAuth {
		t.Fatalf("other app key: %v", err)
	}
}

func TestParseRejectsOtherSecureKey(t *testing.T) {
	tok, err := Issue("app-a", "1234567890abcdef", "u1", time.UnixMilli(1))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := Parse("app-a", "abcdef1234567890", tok); err != ErrAuth {
		t.Fatalf("other secure key: %v", err)
	}
}

func TestParseRejectsMalformedToken(t *testing.T) {
	if _, err := Parse("app-a", "1234567890abcdef", "not-base64"); err != ErrIllegal {
		t.Fatalf("garbage: %v", err)
	}
	if _, err := Parse("app-a", "1234567890abcdef", ""); err != ErrIllegal {
		t.Fatalf("empty: %v", err)
	}
}

func TestParseRejectsEmptyUserID(t *testing.T) {
	tok, err := Issue("app-a", "1234567890abcdef", "", time.UnixMilli(1))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := Parse("app-a", "1234567890abcdef", tok); err != ErrAuth {
		t.Fatalf("empty user id: %v", err)
	}
}
