package service

import (
	"fmt"
	"strings"

	glb "ks-prank/internal/global"
	mytypes "ks-prank/internal/types"
)

// AdminLogin 调用 /admin/user/login，成功后返回 token
func AdminLogin(username, password string) (string, error) {
	if glb.HttpClient == nil {
		return "", fmt.Errorf("http client 未初始化")
	}

	var rsp struct {
		ErrCode int    `json:"errCode"`
		ErrMsg  string `json:"errMsg"`
	}
	resp, err := glb.HttpClient.R().
		SetBody(map[string]string{"username": username, "password": password}).
		SetResult(&rsp).
		Post("/api/v1/admin/user/login")
	if err != nil {
		return "", fmt.Errorf("请求失败: %w", err)
	}
	if !resp.IsSuccess() || rsp.ErrCode != 0 {
		msg := rsp.ErrMsg
		if msg == "" {
			msg = fmt.Sprintf("status=%d", resp.StatusCode())
		}
		return "", fmt.Errorf("登录失败: %s", msg)
	}

	// token 在 Authorization header
	auth := resp.Header().Get("Authorization")
	if auth == "" {
		return "", fmt.Errorf("登录成功但未返回 token")
	}
	return strings.TrimPrefix(auth, "Bearer "), nil
}

// SetAuthToken 把 JWT 挂到 resty 的默认 Authorization header
func SetAuthToken(token string) {
	if glb.HttpClient == nil {
		return
	}
	if token == "" {
		glb.HttpClient.Header.Del("Authorization")
		return
	}
	glb.HttpClient.SetHeader("Authorization", "Bearer "+token)
}

// GetProfile 拉取当前用户聚合信息（site / ar_boxes / live_accounts）
func GetProfile() (*mytypes.Profile, error) {
	if glb.HttpClient == nil {
		return nil, fmt.Errorf("http client 未初始化")
	}
	var rsp struct {
		ErrCode int             `json:"errCode"`
		ErrMsg  string          `json:"errMsg"`
		Data    *mytypes.Profile `json:"data"`
	}
	resp, err := glb.HttpClient.R().
		SetResult(&rsp).
		Post("/api/v1/admin/prank/my_profile")
	if err != nil {
		return nil, fmt.Errorf("请求 profile 失败: %w", err)
	}
	if !resp.IsSuccess() || rsp.ErrCode != 0 {
		return nil, fmt.Errorf("获取 profile 失败: status=%d errCode=%d errMsg=%s", resp.StatusCode(), rsp.ErrCode, rsp.ErrMsg)
	}
	if rsp.Data == nil {
		return nil, fmt.Errorf("profile 数据为空")
	}
	return rsp.Data, nil
}

// GetPrankConfig 获取指定场地+平台的整蛊配置
func GetPrankConfig(siteId, platform string) (*mytypes.PrankConfigData, error) {
	if glb.HttpClient == nil {
		return nil, fmt.Errorf("http client 未初始化")
	}
	var rsp struct {
		ErrCode int    `json:"errCode"`
		ErrMsg  string `json:"errMsg"`
		Data    struct {
			SiteId       string                `json:"site_id"`
			Platform     string                `json:"platform"`
			GiftTriggers []mytypes.GiftTrigger `json:"gift_triggers"`
			ChatTriggers []mytypes.ChatTrigger `json:"chat_triggers"`
			LikeTrigger  *mytypes.LikeTrigger  `json:"like_trigger"`
			UpdatedAt    string                `json:"updated_at"`
		} `json:"data"`
	}
	resp, err := glb.HttpClient.R().
		SetBody(map[string]string{"site_id": siteId, "platform": platform}).
		SetResult(&rsp).
		Post("/api/v1/admin/prank_config/get")
	if err != nil {
		return nil, fmt.Errorf("请求整蛊配置失败: %w", err)
	}
	if !resp.IsSuccess() || rsp.ErrCode != 0 {
		return nil, fmt.Errorf("获取整蛊配置失败: status=%d errCode=%d errMsg=%s", resp.StatusCode(), rsp.ErrCode, rsp.ErrMsg)
	}
	return &mytypes.PrankConfigData{
		GiftTriggers: rsp.Data.GiftTriggers,
		ChatTriggers: rsp.Data.ChatTriggers,
		LikeTrigger:  rsp.Data.LikeTrigger,
	}, nil
}
