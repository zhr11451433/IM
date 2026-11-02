package token

import (
	"errors"
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
	if _, err := Parse("app-b", "1234567890abcdef", tok); !errors.Is(err, ErrAuth) {
		t.Fatalf("other app key: %v", err)
	}
}

func TestParseRejectsOtherSecureKey(t *testing.T) {
	tok, err := Issue("app-a", "1234567890abcdef", "u1", time.UnixMilli(1))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := Parse("app-a", "abcdef1234567890", tok); !errors.Is(err, ErrAuth) {
		t.Fatalf("other secure key: %v", err)
	}
}

func TestParseRejectsMalformedToken(t *testing.T) {
	if _, err := Parse("app-a", "1234567890abcdef", "not-base64"); !errors.Is(err, ErrIllegal) {
		t.Fatalf("garbage: %v", err)
	}
	if _, err := Parse("app-a", "1234567890abcdef", ""); !errors.Is(err, ErrIllegal) {
		t.Fatalf("empty: %v", err)
	}
}

func TestParseRejectsEmptyUserID(t *testing.T) {
	tok, err := Issue("app-a", "1234567890abcdef", "", time.UnixMilli(1))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := Parse("app-a", "1234567890abcdef", tok); !errors.Is(err, ErrAuth) {
		t.Fatalf("empty user id: %v", err)
	}
}

func TestParseWithExpiry(t *testing.T) {
	const key = "1234567890abcdef"
	issuedAt := time.UnixMilli(1_700_000_000_000)

	tok, err := Issue("app-a", key, "u1", issuedAt)
	if err != nil {
		t.Fatal(err)
	}

	tests := []struct {
		name    string
		minutes int
		elapsed time.Duration
		expired bool
	}{
		{"未到期", 1, 59 * time.Second, false},
		{"到期边界", 1, time.Minute, false},
		{"同一毫秒", 1, time.Minute + time.Nanosecond, false},
		{"已经过期", 1, time.Minute + time.Millisecond, true},
		{"不限制有效期", 0, 24 * time.Hour, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			uid, err := ParseWithExpiry(
				"app-a", key, tok,
				tt.minutes, issuedAt.Add(tt.elapsed),
			)

			if tt.expired {
				if err != ErrExpired {
					t.Fatalf("got err=%v, want ErrExpired", err)
				}
				return
			}

			if err != nil || uid != "u1" {
				t.Fatalf("uid=%q, err=%v", uid, err)
			}
		})
	}
}
