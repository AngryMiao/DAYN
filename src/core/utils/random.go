package utils

import (
	"math/rand"
	"time"

	gonanoid "github.com/matoous/go-nanoid/v2"
	// 新增: 加密相关
	"crypto/hmac"
	"crypto/sha256"
	"strconv"
)

var capitalLetters = []rune("ABCDEFGHJKLMNPQRSTUVWXYZ")
var letters = []rune("abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ")
var numbers = []rune("0123456789")
var lettersNumbers = []rune("abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789")

var EasyAlpha = []rune("ABCDEFGHJKLMNPQRSTUVWXYZ0123456789")

func randomCode(n int, keys []rune) string {
	random := rand.New(rand.NewSource(time.Now().UnixNano()))

	keySizes := len(keys)
	b := make([]rune, n)

	for i := range b {
		b[i] = keys[random.Intn(keySizes)]
	}
	return string(b)
}

func GenerateRandomNumber(n int) string {
	return randomCode(n, numbers)
}

func GenerateRandomKey(n int) string {
	return randomCode(n, lettersNumbers)
}

func GenerateRandomCapitalLetter(n int) string {
	return randomCode(n, capitalLetters)
}

func GenerateRandomKeyWithNanoid(n int) string {
	code, err := gonanoid.New(n)
	if err != nil {
		code = GenerateRandomKey(n)
	}
	return code
}

func GenerateAlphaRandomKeyWithNanoid(n int, keys []rune) string {
	code, err := gonanoid.Generate(string(keys), n)
	if err != nil {
		code = randomCode(n, keys)
	}
	return code
}

func HashUserIDWithSalt(userID uint, salt string, length int) string {
	alphabet := lettersNumbers // 安全字符集 [0-9a-zA-Z]
	if length <= 0 {
		length = 64 // 默认长度
	}
	if length > 512 {
		length = 512 // 上限保护，避免极端长度产生不必要的计算
	}

	msg := strconv.FormatUint(uint64(userID), 10)
	out := make([]rune, 0, length)
	counter := 0
	for len(out) < length {
		mac := hmac.New(sha256.New, []byte(salt))
		mac.Write([]byte(msg))
		mac.Write([]byte(":"))
		mac.Write([]byte(strconv.Itoa(counter)))
		sum := mac.Sum(nil) // 32 字节
		counter++
		for _, b := range sum {
			if len(out) >= length {
				break
			}
			idx := int(b) % len(alphabet)
			out = append(out, alphabet[idx])
		}
	}
	return string(out)
}
