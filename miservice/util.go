package miservice

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"math/rand"
	"net/url"
	"time"
)

func getRandom(length int) string {
	charset := "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789"
	randomStr := make([]byte, length)
	for i := range randomStr {
		randomStr[i] = charset[rand.Intn(len(charset))]
	}
	return string(randomStr)
}

func signNonce(ssecurity string, nonce string) (string, error) {
	decodedSsecurity, err := base64.StdEncoding.DecodeString(ssecurity)
	if err != nil {
		return "", err
	}

	decodedNonce, err := base64.StdEncoding.DecodeString(nonce)
	if err != nil {
		return "", err
	}

	hash := sha256.New()
	hash.Write(decodedSsecurity)
	hash.Write(decodedNonce)
	return base64.StdEncoding.EncodeToString(hash.Sum(nil)), nil
}

var genNonce = func() string {
	nonce := make([]byte, 12)
	_, err := rand.Read(nonce[:8])
	if err != nil {
		return ""
	}
	binary.BigEndian.PutUint32(nonce[8:], uint32(time.Now().Unix()/60))
	return base64.StdEncoding.EncodeToString(nonce)
}

func signData(uri string, data any, ssecurity string) url.Values {
	var dataStr []byte
	if s, ok := data.(string); ok {
		dataStr = []byte(s)
	} else {
		var err error
		dataStr, err = json.Marshal(data)
		if err != nil {
			return nil
		}
	}

	encodedNonce := genNonce()
	snonce, err := signNonce(ssecurity, encodedNonce)
	if err != nil {
		return nil
	}
	msg := fmt.Sprintf("%s&%s&%s&data=%s", uri, snonce, encodedNonce, dataStr)
	sb, _ := base64.StdEncoding.DecodeString(snonce)
	sign := hmac.New(sha256.New, sb)
	sign.Write([]byte(msg))
	signature := base64.StdEncoding.EncodeToString(sign.Sum(nil))
	return url.Values{
		"_nonce":    {encodedNonce},
		"data":      {string(dataStr)},
		"signature": {signature},
	}
}
