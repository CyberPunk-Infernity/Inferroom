package utils

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"io"
	"net"
)

// ParseKey ：把命令行传入的 key 转成 AES key bytes
// 约定：server 打印 base64(32字节)；client 也传这个 base64。
// 兼容：如果不是合法 base64/hex，就用 sha256(keyStr) 得到 32 字节。
func ParseKey(keyStr string) ([]byte, error) {
	if keyStr == "" {
		return nil, errors.New("empty key")
	}

	// 1) try base64
	if b, err := base64.StdEncoding.DecodeString(keyStr); err == nil {
		if len(b) == 16 || len(b) == 24 || len(b) == 32 {
			return b, nil
		}
	}

	// 2) try hex
	if b, err := hex.DecodeString(keyStr); err == nil {
		if len(b) == 16 || len(b) == 24 || len(b) == 32 {
			return b, nil
		}
	}

	// 3) fallback sha256
	sum := sha256.Sum256([]byte(keyStr))
	return sum[:], nil
}

func NewRandomKeyBase64(nBytes int) (key []byte, keyB64 string, err error) {
	if nBytes != 16 && nBytes != 24 && nBytes != 32 {
		return nil, "", errors.New("AES key size must be 16/24/32 bytes")
	}
	key = make([]byte, nBytes)
	if _, err := io.ReadFull(rand.Reader, key); err != nil {
		return nil, "", err
	}
	return key, base64.StdEncoding.EncodeToString(key), nil
}

func encryptGCM(key, plaintext []byte) ([]byte, error) {
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}

	nonce := make([]byte, gcm.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return nil, err
	}

	// 输出格式： nonce || ciphertext+tag
	ct := gcm.Seal(nil, nonce, plaintext, nil)
	out := make([]byte, 0, len(nonce)+len(ct))
	out = append(out, nonce...)
	out = append(out, ct...)
	return out, nil
}

func decryptGCM(key, data []byte) ([]byte, error) {
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}

	ns := gcm.NonceSize()
	if len(data) < ns {
		return nil, errors.New("ciphertext too short")
	}
	nonce := data[:ns]
	ct := data[ns:]
	return gcm.Open(nil, nonce, ct, nil)
}

// SecureWriteFrame ：plaintext -> AESGCM -> WriteFrame
func SecureWriteFrame(conn net.Conn, key []byte, plaintext []byte) error {
	enc, err := encryptGCM(key, plaintext)
	if err != nil {
		return err
	}
	return WriteFrame(conn, enc)
}

// SecureReadFrame ：ReadFrame -> AESGCM解密 -> plaintext
func SecureReadFrame(conn net.Conn, key []byte) ([]byte, error) {
	enc, err := ReadFrame(conn)
	if err != nil {
		return nil, err
	}
	return decryptGCM(key, enc)
}
