// Package dingtalk: download file by downloadCode (token + robot messageFiles API)

package dingtalk

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/sipeed/picoclaw/pkg/logger"
	"github.com/sipeed/picoclaw/pkg/utils"
)

const (
	dingtalkTokenURL   = "https://oapi.dingtalk.com/gettoken"
	dingtalkDownloadURL = "https://api.dingtalk.com/v1.0/robot/messageFiles/download"
	tokenExpireMargin  = 5 * time.Minute
)

type tokenHolder struct {
	mu       sync.Mutex
	token    string
	expireAt time.Time
}

// DingTalkDownloader gets access token and downloads file content by downloadCode.
type DingTalkDownloader struct {
	clientID     string
	clientSecret string
	httpClient   *http.Client
	token        tokenHolder
}

// NewDingTalkDownloader creates a downloader with app credentials.
func NewDingTalkDownloader(clientID, clientSecret string) *DingTalkDownloader {
	return &DingTalkDownloader{
		clientID:     clientID,
		clientSecret: clientSecret,
		httpClient:   &http.Client{Timeout: 30 * time.Second},
	}
}

func (d *DingTalkDownloader) getToken(ctx context.Context) (string, error) {
	d.token.mu.Lock()
	defer d.token.mu.Unlock()
	if d.token.token != "" && time.Now().Before(d.token.expireAt) {
		return d.token.token, nil
	}
	req, err := http.NewRequestWithContext(ctx, "GET", dingtalkTokenURL, nil)
	if err != nil {
		return "", err
	}
	q := req.URL.Query()
	q.Set("appkey", d.clientID)
	q.Set("appsecret", d.clientSecret)
	req.URL.RawQuery = q.Encode()
	resp, err := d.httpClient.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", err
	}
	var out struct {
		AccessToken string `json:"access_token"`
		ExpireIn    int64  `json:"expire_in"`
		ErrCode     int    `json:"errcode"`
		ErrMsg      string `json:"errmsg"`
	}
	if err := json.Unmarshal(body, &out); err != nil {
		return "", fmt.Errorf("parse token response: %w", err)
	}
	if out.ErrCode != 0 {
		return "", fmt.Errorf("dingtalk token api: errcode=%d errmsg=%s", out.ErrCode, out.ErrMsg)
	}
	if out.AccessToken == "" {
		return "", fmt.Errorf("dingtalk token api: empty access_token")
	}
	expireSec := out.ExpireIn
	if expireSec <= 0 {
		expireSec = 7200
	}
	d.token.token = out.AccessToken
	d.token.expireAt = time.Now().Add(time.Duration(expireSec)*time.Second - tokenExpireMargin)
	return d.token.token, nil
}

// GetDownloadURL returns the temporary download URL for the given downloadCode.
func (d *DingTalkDownloader) GetDownloadURL(ctx context.Context, downloadCode string) (string, error) {
	token, err := d.getToken(ctx)
	if err != nil {
		return "", err
	}
	body := map[string]string{"downloadCode": downloadCode}
	bodyBytes, _ := json.Marshal(body)
	req, err := http.NewRequestWithContext(ctx, "POST", dingtalkDownloadURL, bytes.NewReader(bodyBytes))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("x-acs-dingtalk-access-token", token)
	resp, err := d.httpClient.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", err
	}
	var result struct {
		DownloadURL string `json:"downloadUrl"`
		Code        string `json:"code"`
		Message     string `json:"message"`
	}
	if err := json.Unmarshal(respBody, &result); err != nil {
		return "", fmt.Errorf("parse download response: %w", err)
	}
	if result.Code != "" && result.Code != "0" {
		return "", fmt.Errorf("dingtalk download api: code=%s message=%s", result.Code, result.Message)
	}
	if result.DownloadURL == "" {
		return "", fmt.Errorf("dingtalk download api: empty downloadUrl")
	}
	return result.DownloadURL, nil
}

// DownloadToPath fetches file by downloadCode and writes to localPath. Returns localPath or error.
func (d *DingTalkDownloader) DownloadToPath(ctx context.Context, downloadCode, localPath string) error {
	urlStr, err := d.GetDownloadURL(ctx, downloadCode)
	if err != nil {
		return err
	}
	dir := filepath.Dir(localPath)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return fmt.Errorf("mkdir: %w", err)
	}
	req, err := http.NewRequestWithContext(ctx, "GET", urlStr, nil)
	if err != nil {
		return err
	}
	resp, err := d.httpClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("download HTTP %d", resp.StatusCode)
	}
	f, err := os.Create(localPath)
	if err != nil {
		return err
	}
	_, err = io.Copy(f, resp.Body)
	f.Close()
	if err != nil {
		os.Remove(localPath)
		return err
	}
	return nil
}

// DownloadToTemp downloads by downloadCode into a temp file under cacheDir (e.g. dingtalk_cache/chatID).
// Returns the local file path. Caller may move or use for extraction.
func (d *DingTalkDownloader) DownloadToTemp(ctx context.Context, downloadCode, cacheDir, filename string) (string, error) {
	safe := utils.SanitizeFilename(filename)
	if safe == "" {
		safe = "file"
	}
	localPath := filepath.Join(cacheDir, safe)
	if err := d.DownloadToPath(ctx, downloadCode, localPath); err != nil {
		logger.WarnCF("dingtalk", "Download by code failed", map[string]any{
			"error": err.Error(),
		})
		return "", err
	}
	return localPath, nil
}
