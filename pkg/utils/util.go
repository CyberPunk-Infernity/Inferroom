package utils

import (
	"crypto/rand"
	"fmt"
	"math/big"
)

// 默认字符集（你也可以改成只要数字/只要小写等）
const defaultCharset = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789"

// RandomString 生成长度为 n 的随机字符串（使用 crypto/rand，适合 token/ID 等场景）
func RandomString(n int) (string, error) {
	if n <= 0 {
		return "", fmt.Errorf("n must be > 0")
	}

	b := make([]byte, n)
	max := big.NewInt(int64(len(defaultCharset)))

	for i := 0; i < n; i++ {
		num, err := rand.Int(rand.Reader, max)
		if err != nil {
			return "", err
		}
		b[i] = defaultCharset[num.Int64()]
	}

	return string(b), nil
}
