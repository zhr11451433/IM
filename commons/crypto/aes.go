package crypto

import (
	"bytes"
	"crypto/aes"
	"crypto/cipher"
	"errors"
)

func Encrypt(orig, key []byte) ([]byte, error) {
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}
	orig = pkcs7(orig, block.BlockSize())
	out := make([]byte, len(orig))
	cipher.NewCBCEncrypter(block, key[:block.BlockSize()]).CryptBlocks(out, orig)
	return out, nil
}
func Decrypt(crypted, key []byte) ([]byte, error) {
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}
	if len(crypted) == 0 || len(crypted)%block.BlockSize() != 0 {
		return nil, errors.New("invalid aes ciphertext")
	}
	out := make([]byte, len(crypted))
	cipher.NewCBCDecrypter(block, key[:block.BlockSize()]).CryptBlocks(out, crypted)
	return unpkcs7(out)
}
func pkcs7(data []byte, size int) []byte {
	n := size - len(data)%size
	return append(data, bytes.Repeat([]byte{byte(n)}, n)...)
}
func unpkcs7(data []byte) ([]byte, error) {
	if len(data) == 0 {
		return nil, errors.New("empty plaintext")
	}
	n := int(data[len(data)-1])
	if n == 0 || n > aes.BlockSize || n > len(data) {
		return nil, errors.New("bad padding")
	}

	for _, b := range data[len(data)-n:] {
		if b != byte(n) {
			return nil, errors.New("bad padding")
		}
	}

	return data[:len(data)-n], nil
}
