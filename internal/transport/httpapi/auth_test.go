package httpapi_test

import (
	"encoding/json"
	"fmt"
	"net/http"
	"testing"

	"wafadmin/pkg/response"
)

// TestLoginFlow 校验「验证码 + 口令」登录并签发会话。
func TestLoginFlow(t *testing.T) {
	h := newHarness(t)
	id, code := h.captcha.New()

	body := fmt.Sprintf(`{"username":"admin","password":"admin","captcha_id":%q,"captcha_code":%q}`, id, code)
	recorder := h.do(t, http.MethodPost, "/api/auth/login", body, false, true)
	if recorder.Code != http.StatusOK {
		t.Fatalf("登录失败: %d %s", recorder.Code, recorder.Body.String())
	}

	payload := struct {
		User struct {
			UserName string `json:"username"`
		} `json:"user"`
	}{}
	if err := json.Unmarshal(decode(t, recorder).Data, &payload); err != nil {
		t.Fatalf("解析登录结果失败: %v", err)
	}
	if payload.User.UserName != "admin" {
		t.Fatalf("登录账号异常: %+v", payload.User)
	}

	var hasSession bool
	for _, cookie := range recorder.Result().Cookies() {
		if cookie.Name == "waf_session" && cookie.Value != "" && cookie.HttpOnly {
			hasSession = true
		}
	}
	if !hasSession {
		t.Fatal("登录成功应下发 HttpOnly 会话 Cookie")
	}
}

// TestLoginRejectsWrongCaptcha 校验验证码错误时拒绝登录。
func TestLoginRejectsWrongCaptcha(t *testing.T) {
	h := newHarness(t)
	id, _ := h.captcha.New()

	body := fmt.Sprintf(`{"username":"admin","password":"admin","captcha_id":%q,"captcha_code":"ZZZZ"}`, id)
	recorder := h.do(t, http.MethodPost, "/api/auth/login", body, false, true)
	if recorder.Code != http.StatusBadRequest {
		t.Fatalf("验证码错误应返回 400，实际 %d", recorder.Code)
	}
	if decode(t, recorder).Code != response.CodeInvalid {
		t.Fatal("响应码应为参数错误")
	}
}

// TestLoginRejectsWrongPassword 校验口令错误时返回未授权。
func TestLoginRejectsWrongPassword(t *testing.T) {
	h := newHarness(t)
	id, code := h.captcha.New()

	body := fmt.Sprintf(`{"username":"admin","password":"wrong-pass","captcha_id":%q,"captcha_code":%q}`, id, code)
	recorder := h.do(t, http.MethodPost, "/api/auth/login", body, false, true)
	if recorder.Code != http.StatusUnauthorized {
		t.Fatalf("口令错误应返回 401，实际 %d", recorder.Code)
	}
}

// TestCaptchaIsSingleUse 校验验证码校验一次后立即失效。
func TestCaptchaIsSingleUse(t *testing.T) {
	h := newHarness(t)
	id, code := h.captcha.New()

	body := fmt.Sprintf(`{"username":"admin","password":"admin","captcha_id":%q,"captcha_code":%q}`, id, code)
	first := h.do(t, http.MethodPost, "/api/auth/login", body, false, true)
	if first.Code != http.StatusOK {
		t.Fatalf("首次登录应成功，实际 %d", first.Code)
	}

	replay := h.do(t, http.MethodPost, "/api/auth/login", body, false, true)
	if replay.Code != http.StatusBadRequest {
		t.Fatalf("验证码复用应被拒绝，实际 %d", replay.Code)
	}
}
