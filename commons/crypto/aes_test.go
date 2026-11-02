package crypto

import (
	"bytes"
	"testing"
)

func TestUnpkcs7(t *testing.T) {
	tests := []struct {
		name    string
		input   []byte
		want    []byte
		wantErr bool
	}{
		{"合法填充", []byte{'a', 1}, []byte{'a'}, false},
		{"整块填充", bytes.Repeat([]byte{16}, 16), []byte{}, false},
		{"填充不一致", []byte{'a', 2, 3}, nil, true},
		{"零填充", []byte{'a', 0}, nil, true},
		{"超过块大小", bytes.Repeat([]byte{17}, 17), nil, true},
		{"超过数据长度", []byte{3}, nil, true},
		{"空数据", nil, nil, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := unpkcs7(tt.input)
			if (err != nil) != tt.wantErr {
				t.Fatalf("err=%v, wantErr=%v", err, tt.wantErr)
			}
			if !tt.wantErr && !bytes.Equal(got, tt.want) {
				t.Fatalf("got=%v, want=%v", got, tt.want)
			}
		})
	}
}
