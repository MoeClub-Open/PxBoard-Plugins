package admob

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"fmt"
	"net/url"
	"testing"
	"time"
)

func signSSVQuery(t *testing.T, priv *ecdsa.PrivateKey, keyID string, params [][2]string) string {
	t.Helper()
	parts := make([]string, 0, len(params))
	for _, kv := range params {
		parts = append(parts, kv[0]+"="+url.QueryEscape(kv[1]))
	}
	signedDecoded := rawPercentDecode(joinAmp(parts))
	sum := sha256.Sum256([]byte(signedDecoded))
	sig, err := ecdsa.SignASN1(rand.Reader, priv, sum[:])
	if err != nil {
		t.Fatal(err)
	}
	sigB64 := base64.RawURLEncoding.EncodeToString(sig)
	return joinAmp(parts) + "&signature=" + sigB64 + "&key_id=" + keyID
}

func joinAmp(parts []string) string {
	out := ""
	for i, p := range parts {
		if i > 0 {
			out += "&"
		}
		out += p
	}
	return out
}

func TestVerifierSignatureAndCustomData(t *testing.T) {
	priv, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	cache := newVerifierKeyCache()
	cache.fetch = func() (map[string]*ecdsa.PublicKey, error) {
		return map[string]*ecdsa.PublicKey{"3335741209": &priv.PublicKey}, nil
	}

	secret := "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"
	customData, err := makeCustomData(secret, "user-42", ScenePoints, 900)
	if err != nil {
		t.Fatal(err)
	}
	nowMs := fmt.Sprintf("%d", time.Now().UnixMilli())
	rawQuery := signSSVQuery(t, priv, "3335741209", [][2]string{
		{"ad_network", "5450213213286189855"},
		{"ad_unit", "4972849015"},
		{"custom_data", customData},
		{"reward_amount", "2"},
		{"reward_item", "points"},
		{"timestamp", nowMs},
		{"transaction_id", "tx-abc-123"},
		{"user_id", "user-42"},
	})

	if _, keyID, err := cache.verifySignature(rawQuery); err != nil || keyID != "3335741209" {
		t.Fatalf("expected valid Google SSV signature, got keyID=%s err=%v", keyID, err)
	}
	claims, err := verifyCustomData(secret, customData, true, time.Now().Unix())
	if err != nil || claims.UserID != "user-42" || claims.Scene != ScenePoints {
		t.Fatalf("unexpected custom_data claims: %+v err=%v", claims, err)
	}
	if !verifyAdUnit("ca-app-pub-6334740403240409/4972849015", "4972849015") {
		t.Fatal("expected tail ad_unit match")
	}
	if verifyAdUnit("ca-app-pub-6334740403240409/4972849015", "9999999999") {
		t.Fatal("unexpected mismatched ad_unit acceptance")
	}

	// Tampering with query parameter invalidates ECDSA signature.
	tampered := "ad_network=1&" + rawQuery
	if _, _, err := cache.verifySignature(tampered); err == nil {
		t.Fatal("expected tampered query signature to fail")
	}
}
