// Package card 提供卡片落地页渲染共用的安全原语。
//
// 闲鱼卡片页是给外部客户直接打开的（没有登录态），但页面上有三条埋点写口：
// POST /api/xianyu/{id}/{click|share|view}。如果只按 id 注册，任何人都能对
// 任意 id 刷浏览/点击/分享。这里沿用 visitor_token 的同一套做法
// （internal/pkg/security/visitor_token.go：HMAC-SHA256 + "." + 过期时间戳），
// 在**渲染卡片时**为这张卡片签一个凭证，上报请求带上它，服务端校验
// 凭证与 :id 的绑定关系。
//
// 密钥来源：既有的部署秘密 USER_JWT_SECRET / JWT_SECRET（utils.DefaultJWTConfig）。
// 不新增明文密钥，也不把同一把密钥直接当 HMAC key 用——先叠一层版本串做域分离，
// 与 storage.fileurl 对 JWT 秘密的用法同一取向。
package card

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	"hivemtk-user/internal/pkg/utils"
)

// XianyuReportTokenHeader 闲鱼卡片埋点上报凭证的请求头。
const XianyuReportTokenHeader = "X-Xianyu-Card-Token"

// XianyuReportTokenTTL 凭证默认有效期。
//
// 卡片页每次打开都会重新渲染、重新签发，7 天足够覆盖"页面被缓存几天后再上报"
// 的场景；过期只影响埋点，不影响卡片内容本身。
const XianyuReportTokenTTL = 7 * 24 * time.Hour

const xianyuReportTokenVersion = "hivemtk/xianyu-card-report/v1"

var (
	// ErrXianyuReportTokenMissing 请求没有带凭证。
	ErrXianyuReportTokenMissing = errors.New("缺少卡片访问凭证")
	// ErrXianyuReportTokenInvalid 凭证格式不对、签名不匹配，或与卡片 id 不绑定。
	ErrXianyuReportTokenInvalid = errors.New("卡片访问凭证无效")
	// ErrXianyuReportTokenExpired 签名正确但已过期。
	ErrXianyuReportTokenExpired = errors.New("卡片访问凭证已过期")
	// ErrXianyuReportTokenKeyUnavailable 部署没有可用的 JWT 秘密（生产装配错误）。
	ErrXianyuReportTokenKeyUnavailable = errors.New("卡片访问凭证签名密钥未配置")
)

// xianyuReportSigningKey 从既有 JWT 秘密派生本域的 HMAC 密钥。
func xianyuReportSigningKey() ([]byte, error) {
	secret := strings.TrimSpace(utils.DefaultJWTConfig.SecretKey)
	if secret == "" {
		return nil, ErrXianyuReportTokenKeyUnavailable
	}
	sum := sha256.Sum256([]byte(xianyuReportTokenVersion + "\x00" + secret))
	return sum[:], nil
}

// signXianyuReportToken 按 id + 过期时间签发凭证。
func signXianyuReportToken(cardID uint, expireTS int64) (string, error) {
	key, err := xianyuReportSigningKey()
	if err != nil {
		return "", err
	}
	mac := hmac.New(sha256.New, key)
	_, _ = fmt.Fprintf(mac, "%d|%d", cardID, expireTS)
	return hex.EncodeToString(mac.Sum(nil)) + "." + strconv.FormatInt(expireTS, 10), nil
}

// GenerateXianyuReportToken 为一张卡片签发埋点上报凭证。
//
// ttl <= 0 时使用 XianyuReportTokenTTL。
func GenerateXianyuReportToken(cardID uint, ttl time.Duration) (string, error) {
	if cardID == 0 {
		return "", ErrXianyuReportTokenInvalid
	}
	if ttl <= 0 {
		ttl = XianyuReportTokenTTL
	}
	return signXianyuReportToken(cardID, time.Now().Add(ttl).Unix())
}

// VerifyXianyuReportToken 校验凭证：格式、签名、过期时间，且必须与 cardID 绑定。
func VerifyXianyuReportToken(cardID uint, token string) error {
	return verifyXianyuReportTokenAt(cardID, token, time.Now())
}

func verifyXianyuReportTokenAt(cardID uint, token string, now time.Time) error {
	if strings.TrimSpace(token) == "" {
		return ErrXianyuReportTokenMissing
	}
	if cardID == 0 {
		return ErrXianyuReportTokenInvalid
	}
	parts := strings.SplitN(token, ".", 2)
	if len(parts) != 2 {
		return ErrXianyuReportTokenInvalid
	}
	expireTS, err := strconv.ParseInt(parts[1], 10, 64)
	if err != nil {
		return ErrXianyuReportTokenInvalid
	}
	expected, err := signXianyuReportToken(cardID, expireTS)
	if err != nil {
		return err
	}
	if !hmac.Equal([]byte(token), []byte(expected)) {
		return ErrXianyuReportTokenInvalid
	}
	if now.Unix() > expireTS {
		return ErrXianyuReportTokenExpired
	}
	return nil
}
