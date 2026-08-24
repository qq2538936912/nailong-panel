package middleware

import (
	"net/http"
	"strings"
	"time"

	"panel/config"
	"panel/database"
	"panel/model"

	"github.com/gin-gonic/gin"
	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
)

type Claims struct {
	Username  string `json:"username"`
	Role      string `json:"role"`
	TokenType string `json:"token_type"`
	// AppEpoch 仅 Open API 应用令牌使用：禁用应用或重置密钥时会抬升世代，旧 token 立刻失效。
	AppEpoch int `json:"app_epoch,omitempty"`
	jwt.RegisteredClaims
}

type TokenInfo struct {
	Token     string
	JTI       string
	ExpiresAt time.Time
}

func GenerateAccessToken(username, role string) (string, error) {
	info, err := GenerateAccessTokenInfo(username, role)
	if err != nil {
		return "", err
	}
	return info.Token, nil
}

func GenerateAccessTokenInfo(username, role string) (*TokenInfo, error) {
	return generateAccessTokenInfoWithTTL(username, role, config.C.JWT.AccessTokenExpire)
}

func GenerateTemporaryAccessToken(username, role string, ttl time.Duration) (string, error) {
	info, err := GenerateTemporaryAccessTokenInfo(username, role, ttl)
	if err != nil {
		return "", err
	}
	return info.Token, nil
}

// GenerateTemporaryAccessTokenInfo 与 GenerateTemporaryAccessToken 等价，但保留 jti 与到期时间。
// 签发方拿到 jti 才能在凭据用完后主动拉黑（IsTokenBlocked 按 jti 单条命中），
// 否则临时 token 只能等自然过期，面板自己都不知道它是谁。
func GenerateTemporaryAccessTokenInfo(username, role string, ttl time.Duration) (*TokenInfo, error) {
	return generateAccessTokenInfoWithTTL(username, role, ttl)
}

// GenerateOpenAppAccessToken 签发带 jti + app_epoch 的应用访问令牌。
// epoch 必须与 open_apps.token_epoch 一致，否则 JWTAuth 会拒绝。
func GenerateOpenAppAccessToken(appKey, scopes string, epoch int, ttl time.Duration) (*TokenInfo, error) {
	if ttl <= 0 {
		ttl = 24 * time.Hour
	}

	jti := generateJTI()
	expiresAt := time.Now().Add(ttl)
	claims := Claims{
		Username:  "app:" + appKey,
		Role:      "app:" + scopes,
		TokenType: "access",
		AppEpoch:  epoch,
		RegisteredClaims: jwt.RegisteredClaims{
			ExpiresAt: jwt.NewNumericDate(expiresAt),
			IssuedAt:  jwt.NewNumericDate(time.Now()),
			ID:        jti,
		},
	}
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	tokenStr, err := token.SignedString([]byte(config.C.JWT.Secret))
	if err != nil {
		return nil, err
	}
	return &TokenInfo{Token: tokenStr, JTI: jti, ExpiresAt: expiresAt}, nil
}

func generateAccessTokenInfoWithTTL(username, role string, ttl time.Duration) (*TokenInfo, error) {
	if ttl <= 0 {
		ttl = config.C.JWT.AccessTokenExpire
	}

	jti := generateJTI()
	expiresAt := time.Now().Add(ttl)
	claims := Claims{
		Username:  username,
		Role:      role,
		TokenType: "access",
		RegisteredClaims: jwt.RegisteredClaims{
			ExpiresAt: jwt.NewNumericDate(expiresAt),
			IssuedAt:  jwt.NewNumericDate(time.Now()),
			ID:        jti,
		},
	}
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	tokenStr, err := token.SignedString([]byte(config.C.JWT.Secret))
	if err != nil {
		return nil, err
	}
	return &TokenInfo{Token: tokenStr, JTI: jti, ExpiresAt: expiresAt}, nil
}

func GenerateRefreshToken(username, role string) (string, error) {
	info, err := GenerateRefreshTokenInfo(username, role)
	if err != nil {
		return "", err
	}
	return info.Token, nil
}

func GenerateRefreshTokenInfo(username, role string) (*TokenInfo, error) {
	jti := generateJTI()
	expiresAt := time.Now().Add(config.C.JWT.RefreshTokenExpire)
	claims := Claims{
		Username:  username,
		Role:      role,
		TokenType: "refresh",
		RegisteredClaims: jwt.RegisteredClaims{
			ExpiresAt: jwt.NewNumericDate(expiresAt),
			IssuedAt:  jwt.NewNumericDate(time.Now()),
			ID:        jti,
		},
	}
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	tokenStr, err := token.SignedString([]byte(config.C.JWT.Secret))
	if err != nil {
		return nil, err
	}
	return &TokenInfo{Token: tokenStr, JTI: jti, ExpiresAt: expiresAt}, nil
}

func ParseToken(tokenString string) (*Claims, error) {
	token, err := jwt.ParseWithClaims(tokenString, &Claims{}, func(token *jwt.Token) (interface{}, error) {
		return []byte(config.C.JWT.Secret), nil
	})
	if err != nil {
		return nil, err
	}
	if claims, ok := token.Claims.(*Claims); ok && token.Valid {
		return claims, nil
	}
	return nil, jwt.ErrSignatureInvalid
}

func IsTokenBlocked(jti string) bool {
	var count int64
	database.DB.Model(&model.TokenBlocklist{}).Where("jti = ?", jti).Count(&count)
	return count > 0
}

func ExtractBearerToken(authHeader string) string {
	if !strings.HasPrefix(authHeader, "Bearer ") {
		return ""
	}

	return strings.TrimSpace(strings.TrimPrefix(authHeader, "Bearer "))
}

func JWTAuth() gin.HandlerFunc {
	return func(c *gin.Context) {
		tokenStr := ExtractBearerToken(c.GetHeader("Authorization"))

		if tokenStr == "" {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "缺少授权令牌"})
			c.Abort()
			return
		}

		claims, err := ParseToken(tokenStr)
		if err != nil {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "令牌无效或已过期"})
			c.Abort()
			return
		}

		if claims.TokenType != "access" {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "令牌类型错误"})
			c.Abort()
			return
		}

		if IsTokenBlocked(claims.ID) {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "令牌已被撤销"})
			c.Abort()
			return
		}

		if isAppToken(claims.Username, claims.Role) {
			app, err := loadOpenAppByUsername(claims.Username)
			if err != nil || !app.Enabled {
				c.JSON(http.StatusUnauthorized, gin.H{"error": "应用不存在或已被禁用"})
				c.Abort()
				return
			}
			if claims.AppEpoch != app.TokenEpoch {
				c.JSON(http.StatusUnauthorized, gin.H{"error": "令牌已被撤销"})
				c.Abort()
				return
			}
			c.Set("token_kind", "app")
			c.Set("open_app_id", app.ID)
		} else {
			c.Set("token_kind", "user")
		}

		c.Set("username", claims.Username)
		c.Set("role", claims.Role)
		c.Set("jti", claims.ID)
		c.Next()
	}
}

func RequireAdmin() gin.HandlerFunc {
	return func(c *gin.Context) {
		role, exists := c.Get("role")
		if !exists || role.(string) != "admin" {
			c.JSON(http.StatusForbidden, gin.H{"error": "需要管理员权限"})
			c.Abort()
			return
		}
		c.Next()
	}
}

func RequireRole(minRole string) gin.HandlerFunc {
	roleLevel := map[string]int{
		"viewer":   1,
		"operator": 2,
		"admin":    3,
	}

	return func(c *gin.Context) {
		role, exists := c.Get("role")
		if !exists {
			c.JSON(http.StatusForbidden, gin.H{"error": "拒绝访问"})
			c.Abort()
			return
		}

		if c.GetString("token_kind") == "app" {
			if c.GetBool("app_scope_authorized") {
				c.Next()
				return
			}

			c.JSON(http.StatusForbidden, gin.H{"error": "应用令牌无权访问此接口"})
			c.Abort()
			return
		}

		if roleLevel[role.(string)] < roleLevel[minRole] {
			c.JSON(http.StatusForbidden, gin.H{"error": "权限不足"})
			c.Abort()
			return
		}
		c.Next()
	}
}

func generateJTI() string {
	return uuid.New().String()
}
