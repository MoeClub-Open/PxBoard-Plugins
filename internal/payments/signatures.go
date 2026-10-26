package payments

import (
	"bytes"
	"crypto/hmac"
	"crypto/md5"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"time"
)

func verifyStripe(raw []byte, header, key string) bool {
	if key == "" {
		return false
	}
	var timestamp string
	sigs := []string{}
	for _, part := range strings.Split(header, ",") {
		kv := strings.SplitN(strings.TrimSpace(part), "=", 2)
		if len(kv) != 2 {
			continue
		}
		if kv[0] == "t" {
			timestamp = kv[1]
		} else if kv[0] == "v1" {
			sigs = append(sigs, kv[1])
		}
	}
	n, e := strconv.ParseInt(timestamp, 10, 64)
	if e != nil || time.Since(time.Unix(n, 0)).Abs() > 5*time.Minute {
		return false
	}
	mac := hmac.New(sha256.New, []byte(key))
	_, _ = mac.Write(append([]byte(timestamp+"."), raw...))
	for _, sig := range sigs {
		b, e := hex.DecodeString(sig)
		if e == nil && hmac.Equal(b, mac.Sum(nil)) {
			return true
		}
	}
	return false
}
func epaySign(v url.Values, key string) string {
	keys := []string{}
	for k := range v {
		if k != "sign" && k != "sign_type" && v.Get(k) != "" {
			keys = append(keys, k)
		}
	}
	sort.Strings(keys)
	var b bytes.Buffer
	for i, k := range keys {
		if i > 0 {
			b.WriteByte('&')
		}
		b.WriteString(k + "=" + v.Get(k))
	}
	b.WriteString(key)
	sum := md5.Sum(b.Bytes())
	return hex.EncodeToString(sum[:])
}
func minor(value string) (int64, error) {
	if strings.HasPrefix(value, "-") || strings.HasPrefix(value, "+") {
		return 0, fmt.Errorf("支付金额无效")
	}
	parts := strings.Split(value, ".")
	if len(parts) > 2 {
		return 0, fmt.Errorf("金额无效")
	}
	whole, e := strconv.ParseInt(parts[0], 10, 64)
	if e != nil {
		return 0, e
	}
	fraction := "00"
	if len(parts) == 2 {
		if len(parts[1]) > 2 {
			return 0, fmt.Errorf("金额精度无效")
		}
		fraction = (parts[1] + "00")[:2]
	}
	n, e := strconv.ParseInt(fraction, 10, 64)
	if e != nil || whole < 0 || n < 0 || n > 99 || whole > ((1<<53-1)-n)/100 {
		return 0, fmt.Errorf("支付金额无效")
	}
	return whole*100 + n, nil
}
