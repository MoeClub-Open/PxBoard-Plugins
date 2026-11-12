package admob

import (
	"crypto/ecdsa"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"crypto/x509"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"encoding/pem"
	"fmt"
	"net/http"
	"strings"
	"sync"
	"time"
)

const (
	googleVerifierKeysURL = "https://www.gstatic.com/admob/reward/verifier-keys.json"
	ConsoleVerifyToken    = "xbclient_admob_verify"
	ScenePlan             = "plan"
	ScenePoints           = "points"
)

type customDataClaims struct {
	UserID string `json:"userId"`
	Scene  string `json:"scene"`
	IAT    int64  `json:"iat"`
	EXP    int64  `json:"exp"`
	Nonce  string `json:"nonce"`
}

type verifierKeyCache struct {
	mu        sync.Mutex
	keys      map[string]*ecdsa.PublicKey
	fetchedAt time.Time
	fetch     func() (map[string]*ecdsa.PublicKey, error)
}

func newVerifierKeyCache() *verifierKeyCache {
	c := &verifierKeyCache{keys: map[string]*ecdsa.PublicKey{}}
	c.fetch = fetchGoogleVerifierKeys
	return c
}

func fetchGoogleVerifierKeys() (map[string]*ecdsa.PublicKey, error) {
	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Get(googleVerifierKeysURL)
	if err != nil {
		return nil, fmt.Errorf("获取 AdMob SSV 公钥失败：%w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("获取 AdMob SSV 公钥失败：HTTP %d", resp.StatusCode)
	}
	var payload struct {
		Keys []struct {
			KeyID  json.Number `json:"keyId"`
			PEM    string      `json:"pem"`
			Base64 string      `json:"base64"`
		} `json:"keys"`
	}
	dec := json.NewDecoder(resp.Body)
	dec.UseNumber()
	if err := dec.Decode(&payload); err != nil {
		return nil, fmt.Errorf("解析 AdMob SSV 公钥失败：%w", err)
	}
	out := make(map[string]*ecdsa.PublicKey, len(payload.Keys))
	for _, item := range payload.Keys {
		id := strings.TrimSpace(item.KeyID.String())
		if id == "" {
			continue
		}
		var der []byte
		if item.PEM != "" {
			if blk, _ := pem.Decode([]byte(item.PEM)); blk != nil {
				der = blk.Bytes
			}
		}
		if len(der) == 0 && item.Base64 != "" {
			der, _ = base64.StdEncoding.DecodeString(strings.TrimSpace(item.Base64))
		}
		if len(der) == 0 {
			continue
		}
		if pub, err := x509.ParsePKIXPublicKey(der); err == nil {
			if ec, ok := pub.(*ecdsa.PublicKey); ok {
				out[id] = ec
			}
		}
	}
	if len(out) == 0 {
		return nil, fmt.Errorf("AdMob SSV 公钥列表为空")
	}
	return out, nil
}

func (c *verifierKeyCache) key(keyID string) (*ecdsa.PublicKey, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	now := time.Now()
	if pub, ok := c.keys[keyID]; ok && now.Sub(c.fetchedAt) < time.Hour {
		return pub, nil
	}
	if len(c.keys) == 0 || now.Sub(c.fetchedAt) >= 30*time.Second {
		fresh, err := c.fetch()
		if err != nil {
			return nil, err
		}
		c.keys = fresh
		c.fetchedAt = now
	}
	pub, ok := c.keys[keyID]
	if !ok {
		return nil, fmt.Errorf("AdMob SSV key_id 无匹配公钥")
	}
	return pub, nil
}

func rawPercentDecode(s string) string {
	var out strings.Builder
	out.Grow(len(s))
	for i := 0; i < len(s); i++ {
		if s[i] == '%' && i+2 < len(s) {
			if b, err := hex.DecodeString(s[i+1 : i+3]); err == nil && len(b) == 1 {
				out.WriteByte(b[0])
				i += 2
				continue
			}
		}
		out.WriteByte(s[i])
	}
	return out.String()
}

func decodeBase64Flexible(s string) ([]byte, error) {
	s = strings.TrimRight(strings.NewReplacer("-", "+", "_", "/").Replace(strings.TrimSpace(s)), "=")
	return base64.RawStdEncoding.DecodeString(s)
}

func (c *verifierKeyCache) verifySignature(rawQuery string) (string, string, error) {
	decoded := rawPercentDecode(rawQuery)
	sigIdx := strings.Index(decoded, "&signature=")
	if sigIdx <= 0 {
		return "", "", fmt.Errorf("AdMob SSV 缺少签名")
	}
	signedDecoded := decoded[:sigIdx]
	tail := decoded[sigIdx+len("&signature="):]
	keyIdx := strings.Index(tail, "&key_id=")
	if keyIdx <= 0 {
		return "", "", fmt.Errorf("AdMob SSV 缺少 key_id")
	}
	sigValue := tail[:keyIdx]
	keyID := tail[keyIdx+len("&key_id="):]
	if keyID == "" || strings.Contains(keyID, "&") {
		return "", "", fmt.Errorf("AdMob SSV key_id 格式错误")
	}
	sigBytes, err := decodeBase64Flexible(sigValue)
	if err != nil || len(sigBytes) == 0 {
		return "", "", fmt.Errorf("AdMob SSV 签名编码无效")
	}
	pub, err := c.key(keyID)
	if err != nil {
		return "", "", err
	}
	sumDecoded := sha256.Sum256([]byte(signedDecoded))
	if ecdsa.VerifyASN1(pub, sumDecoded[:], sigBytes) {
		return sigValue, keyID, nil
	}
	if rawIdx := strings.Index(rawQuery, "&signature="); rawIdx > 0 {
		sumRaw := sha256.Sum256([]byte(rawQuery[:rawIdx]))
		if ecdsa.VerifyASN1(pub, sumRaw[:], sigBytes) {
			return sigValue, keyID, nil
		}
	}
	return "", "", fmt.Errorf("AdMob SSV 签名验证失败")
}

func randomSecretHex(bytesLen int) (string, error) {
	b := make([]byte, bytesLen)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}

func makeCustomData(secret, userID, scene string, ttlSec int) (string, error) {
	if strings.TrimSpace(secret) == "" {
		return "", fmt.Errorf("SSV 签名密钥未就绪")
	}
	nonce, err := randomSecretHex(16)
	if err != nil {
		return "", err
	}
	now := time.Now().Unix()
	if ttlSec < 60 {
		ttlSec = 900
	}
	claims := customDataClaims{
		UserID: userID,
		Scene:  scene,
		IAT:    now,
		EXP:    now + int64(ttlSec),
		Nonce:  nonce,
	}
	raw, err := json.Marshal(claims)
	if err != nil {
		return "", err
	}
	body := base64.RawURLEncoding.EncodeToString(raw)
	mac := hmac.New(sha256.New, []byte(secret))
	_, _ = mac.Write([]byte(body))
	sig := base64.RawURLEncoding.EncodeToString(mac.Sum(nil))
	return body + "." + sig, nil
}

func verifyCustomData(secret, customData string, checkExp bool, nowSec int64) (customDataClaims, error) {
	var claims customDataClaims
	if strings.TrimSpace(secret) == "" {
		return claims, fmt.Errorf("SSV 签名密钥未配置")
	}
	parts := strings.Split(strings.TrimSpace(customData), ".")
	if len(parts) != 2 {
		return claims, fmt.Errorf("AdMob SSV custom_data 格式错误")
	}
	mac := hmac.New(sha256.New, []byte(secret))
	_, _ = mac.Write([]byte(parts[0]))
	expected := base64.RawURLEncoding.EncodeToString(mac.Sum(nil))
	if !hmac.Equal([]byte(expected), []byte(parts[1])) {
		return claims, fmt.Errorf("AdMob SSV custom_data 签名错误")
	}
	raw, err := decodeBase64Flexible(parts[0])
	if err != nil || json.Unmarshal(raw, &claims) != nil {
		return claims, fmt.Errorf("AdMob SSV custom_data 内容无效")
	}
	if claims.UserID == "" || (claims.Scene != ScenePlan && claims.Scene != ScenePoints) || claims.EXP <= 0 {
		return claims, fmt.Errorf("AdMob SSV custom_data 字段无效")
	}
	if nowSec <= 0 {
		nowSec = time.Now().Unix()
	}
	if checkExp && claims.EXP < nowSec {
		return claims, fmt.Errorf("AdMob SSV custom_data 已过期")
	}
	return claims, nil
}

func verifyAdUnit(expected, actual string) bool {
	actual = strings.TrimSpace(actual)
	if actual == "" {
		return false
	}
	expected = strings.TrimSpace(expected)
	if expected == "" {
		return true
	}
	tail := expected
	if idx := strings.LastIndex(expected, "/"); idx >= 0 {
		tail = expected[idx+1:]
	}
	return actual == expected || actual == tail
}

func normalizeCallbackTimestamp(raw int64) int64 {
	if raw > 100_000_000_000_000 {
		return raw / 1_000_000
	}
	if raw > 10_000_000_000 {
		return raw / 1_000
	}
	return raw
}
