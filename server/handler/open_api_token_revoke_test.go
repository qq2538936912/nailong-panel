package handler_test

import (
	"encoding/json"
	"fmt"
	"net/http"
	"testing"

	"panel/database"
	"panel/middleware"
	"panel/model"
	"panel/testutil"

	"github.com/gin-gonic/gin"
)

func issueOpenAPIToken(t *testing.T, engine *gin.Engine, appKey, appSecret string) string {
	t.Helper()

	body := fmt.Sprintf(`{"app_key":%q,"app_secret":%q}`, appKey, appSecret)
	rec := performJSONRequest(engine, http.MethodPost, "/api/v1/open-api/token", body, nil, "")
	if rec.Code != http.StatusOK {
		t.Fatalf("issue token: status=%d body=%s", rec.Code, rec.Body.String())
	}

	var payload struct {
		Data struct {
			AccessToken string `json:"access_token"`
		} `json:"data"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &payload); err != nil {
		t.Fatalf("decode token response: %v", err)
	}
	if payload.Data.AccessToken == "" {
		t.Fatal("expected non-empty access_token")
	}
	return payload.Data.AccessToken
}

func probeAppToken(t *testing.T, token string) int {
	t.Helper()

	engine := gin.New()
	engine.GET("/probe", middleware.JWTAuth(), func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"ok": true})
	})

	return performRequest(engine, http.MethodGet, "/probe", map[string]string{
		"Authorization": "Bearer " + token,
	}).Code
}

func TestOpenAPITokenCarriesJTIAndSurvivesWhileEpochMatches(t *testing.T) {
	testutil.SetupTestEnv(t)

	app := testutil.MustCreateOpenApp(t, "epoch-ok", "tasks")
	engine := newOpenAPIRouter()
	token := issueOpenAPIToken(t, engine, app.AppKey, app.AppSecret)

	claims, err := middleware.ParseToken(token)
	if err != nil {
		t.Fatalf("parse token: %v", err)
	}
	if claims.ID == "" {
		t.Fatal("open api token must carry jti")
	}
	if claims.AppEpoch != app.TokenEpoch {
		t.Fatalf("expected app_epoch=%d, got %d", app.TokenEpoch, claims.AppEpoch)
	}
	if code := probeAppToken(t, token); code != http.StatusOK {
		t.Fatalf("expected 200 while epoch matches, got %d", code)
	}
}

func TestOpenAPIResetSecretRevokesExistingTokens(t *testing.T) {
	testutil.SetupTestEnv(t)

	admin := testutil.MustCreateUser(t, "open-api-revoke-admin", "admin")
	adminToken := testutil.MustCreateAccessToken(t, admin.Username, admin.Role)
	app := testutil.MustCreateOpenApp(t, "epoch-reset", "tasks")
	engine := newOpenAPIRouter()

	oldToken := issueOpenAPIToken(t, engine, app.AppKey, app.AppSecret)
	if code := probeAppToken(t, oldToken); code != http.StatusOK {
		t.Fatalf("old token should work before reset, got %d", code)
	}

	resetRec := performRequest(engine, http.MethodPut, fmt.Sprintf("/api/v1/open-api/apps/%d/reset-secret", app.ID), map[string]string{
		"Authorization": "Bearer " + adminToken,
	})
	if resetRec.Code != http.StatusOK {
		t.Fatalf("reset secret: status=%d body=%s", resetRec.Code, resetRec.Body.String())
	}

	if code := probeAppToken(t, oldToken); code != http.StatusUnauthorized {
		t.Fatalf("old token should be revoked after reset-secret, got %d", code)
	}

	var updated model.OpenApp
	if err := database.DB.First(&updated, app.ID).Error; err != nil {
		t.Fatalf("reload app: %v", err)
	}
	if updated.TokenEpoch != app.TokenEpoch+1 {
		t.Fatalf("expected token_epoch=%d, got %d", app.TokenEpoch+1, updated.TokenEpoch)
	}

	newToken := issueOpenAPIToken(t, engine, updated.AppKey, updated.AppSecret)
	if code := probeAppToken(t, newToken); code != http.StatusOK {
		t.Fatalf("new token after reset should work, got %d", code)
	}
}

func TestOpenAPIDisableRevokesExistingTokens(t *testing.T) {
	testutil.SetupTestEnv(t)

	admin := testutil.MustCreateUser(t, "open-api-disable-admin", "admin")
	adminToken := testutil.MustCreateAccessToken(t, admin.Username, admin.Role)
	app := testutil.MustCreateOpenApp(t, "epoch-disable", "tasks")
	engine := newOpenAPIRouter()

	token := issueOpenAPIToken(t, engine, app.AppKey, app.AppSecret)
	disableRec := performRequest(engine, http.MethodPut, fmt.Sprintf("/api/v1/open-api/apps/%d/disable", app.ID), map[string]string{
		"Authorization": "Bearer " + adminToken,
	})
	if disableRec.Code != http.StatusOK {
		t.Fatalf("disable: status=%d body=%s", disableRec.Code, disableRec.Body.String())
	}

	if code := probeAppToken(t, token); code != http.StatusUnauthorized {
		t.Fatalf("token should be revoked after disable, got %d", code)
	}
}
