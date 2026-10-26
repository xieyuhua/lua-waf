package service

import (
	"context"
	"encoding/base64"
	"strings"
	"time"

	"wafadmin/internal/model"
	"wafadmin/internal/repository"
	"wafadmin/pkg/captcha"
	"wafadmin/pkg/crypto"
	"wafadmin/pkg/ratelimit"
)

// LoginRequest 是登录接口的入参。
type LoginRequest struct {
	Username    string `json:"username"`
	Password    string `json:"password"`
	CaptchaId   string `json:"captcha_id"`
	CaptchaCode string `json:"captcha_code"`
	// ClientIp 由接入层填充，用于登录失败限流。
	ClientIp string `json:"-"`
}

// CaptchaChallenge 是下发给前端的图形验证码。
type CaptchaChallenge struct {
	Id        string `json:"captcha_id"`
	Image     string `json:"image"`
	ExpiresIn int    `json:"expires_in"`
}

// AuthService 负责登录校验与身份读取。
type AuthService struct {
	Users   repository.UserRepository
	Hasher  crypto.Hasher
	Captcha *captcha.Manager
	Limiter *ratelimit.Limiter
}

// NewCaptcha 生成图形验证码，返回 base64 PNG。
func (s *AuthService) NewCaptcha() (*CaptchaChallenge, error) {
	id, code := s.Captcha.New()
	image, err := s.Captcha.Image(code)
	if err != nil {
		return nil, ErrInternal.Wrap(err, "生成验证码失败")
	}
	return &CaptchaChallenge{
		Id:        id,
		Image:     "data:image/png;base64," + base64.StdEncoding.EncodeToString(image),
		ExpiresIn: int(s.captchaTTL().Seconds()),
	}, nil
}

// Login 校验验证码与口令。
func (s *AuthService) Login(ctx context.Context, req LoginRequest) (*model.User, error) {
	username := strings.TrimSpace(req.Username)
	if username == "" || req.Password == "" {
		return nil, ErrInvalid.New("请输入用户名与口令")
	}

	if s.Limiter != nil && !s.Limiter.Allow(req.ClientIp) {
		return nil, ErrForbidden.New("登录尝试过于频繁，请稍后再试")
	}
	if !s.Captcha.Verify(req.CaptchaId, req.CaptchaCode) {
		return nil, ErrInvalid.New("验证码错误或已过期")
	}

	user, err := s.Users.GetByName(ctx, username)
	if err != nil {
		return nil, ErrInternal.Wrap(err, "查询账号失败")
	}
	if user == nil || !s.Hasher.Verify(req.Password, user.Password) {
		return nil, ErrUnauthorized.New("用户名或密码错误")
	}

	if err := s.Users.TouchLogin(ctx, user.Id, time.Now()); err != nil {
		return nil, ErrInternal.Wrap(err, "更新登录时间失败")
	}
	if s.Limiter != nil {
		s.Limiter.Reset(req.ClientIp)
	}
	return user, nil
}

// Profile 读取当前登录账号信息。
func (s *AuthService) Profile(ctx context.Context, uid int64) (*model.User, error) {
	user, err := s.Users.Get(ctx, uid)
	if err != nil {
		return nil, ErrInternal.Wrap(err, "查询账号失败")
	}
	if user == nil {
		return nil, ErrUnauthorized.New("账号不存在，请重新登录")
	}
	return user, nil
}

// captchaTTL 返回验证码有效期。
func (s *AuthService) captchaTTL() time.Duration {
	if s.Captcha == nil || s.Captcha.TTL <= 0 {
		return 5 * time.Minute
	}
	return s.Captcha.TTL
}
