package doubao

import (
	"angrymiao-ai-server/src/core/providers/auc"
	"angrymiao-ai-server/src/core/utils"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
)

// Ensure Provider implements auc.Provider interface
var _ auc.Provider = (*Provider)(nil)

// Provider 豆包AUC提供者实现
type Provider struct {
	*auc.BaseProvider
	appID       string
	accessToken string
	cluster     string
	serviceURL  string
	callbackURL string
	logger      *utils.Logger
}

// SubmitRequest 提交任务请求结构
type SubmitRequest struct {
	App       AppInfo           `json:"app"`
	User      UserInfo          `json:"user"`
	Audio     AudioInfo         `json:"audio"`
	Additions map[string]string `json:"additions"`
	Request   RequestInfo       `json:"request"`
}

type AppInfo struct {
	Appid   string `json:"appid"`
	Token   string `json:"token"`
	Cluster string `json:"cluster"`
}

type UserInfo struct {
	UID string `json:"uid"`
}

type AudioInfo struct {
	Format string `json:"format"`
	URL    string `json:"url"`
}

type RequestInfo struct {
	Callback string `json:"callback"`
}

// SubmitResponse 提交任务响应结构
type SubmitResponse struct {
	Resp struct {
		ID   string `json:"id"`
		Code int    `json:"code"`
	} `json:"resp"`
}

// QueryRequest 查询任务请求结构
type QueryRequest struct {
	Appid   string `json:"appid"`
	Token   string `json:"token"`
	ID      string `json:"id"`
	Cluster string `json:"cluster"`
}

// NewProvider 创建豆包AUC提供者实例
func NewProvider(config *auc.Config, logger *utils.Logger) (*Provider, error) {
	base := auc.NewBaseProvider(config, logger)

	// 从config.Data中获取配置
	appID, ok := config.Data["appid"].(string)
	if !ok {
		return nil, fmt.Errorf("缺少appid配置")
	}

	accessToken, ok := config.Data["access_token"].(string)
	if !ok {
		return nil, fmt.Errorf("缺少access_token配置")
	}

	cluster, ok := config.Data["cluster"].(string)
	if !ok {
		return nil, fmt.Errorf("缺少cluster配置")
	}

	callbackURL, ok := config.Data["callback_url"].(string)
	if !ok {
		return nil, fmt.Errorf("缺少callback_url配置")
	}

	requestURL, ok := config.Data["request_url"].(string)
	if !ok {
		return nil, fmt.Errorf("缺少request_url配置")
	}

	provider := &Provider{
		BaseProvider: base,
		appID:        appID,
		accessToken:  accessToken,
		cluster:      cluster,
		serviceURL:   requestURL,
		callbackURL:  callbackURL,
		logger:       logger,
	}

	return provider, nil
}

// SubmitTask 提交识别任务
func (p *Provider) SubmitTask(ctx context.Context, audioURL string, userID string) (string, error) {
	request := SubmitRequest{
		App: AppInfo{
			Appid:   p.appID,
			Token:   p.accessToken,
			Cluster: p.cluster,
		},
		User: UserInfo{
			UID: userID,
		},
		Audio: AudioInfo{
			Format: "wav",
			URL:    audioURL,
		},
		Additions: map[string]string{
			"with_speaker_info": "False",
		},
		Request: RequestInfo{
			Callback: p.callbackURL,
		},
	}

	jsonData, err := json.Marshal(request)
	if err != nil {
		return "", fmt.Errorf("marshal request failed: %w", err)
	}

	client := &http.Client{}
	req, err := http.NewRequestWithContext(ctx, "POST", p.serviceURL+"/submit", bytes.NewBuffer(jsonData))
	if err != nil {
		return "", fmt.Errorf("create request failed: %w", err)
	}

	req.Header.Set("Authorization", fmt.Sprintf("Bearer; %s", p.accessToken))
	req.Header.Set("Content-Type", "application/json")

	resp, err := client.Do(req)
	if err != nil {
		return "", fmt.Errorf("request failed: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("read response failed: %w", err)
	}

	p.logger.Info("AUC submit response: %s", string(body))

	var submitResp SubmitResponse
	if err := json.Unmarshal(body, &submitResp); err != nil {
		return "", fmt.Errorf("unmarshal response failed: %w", err)
	}

	if submitResp.Resp.Code != 1000 {
		return "", fmt.Errorf("submit task failed with code: %d", submitResp.Resp.Code)
	}

	p.logger.Info("AUC task submitted successfully, task ID: %s", submitResp.Resp.ID)
	return submitResp.Resp.ID, nil
}

// QueryTask 查询任务状态
func (p *Provider) QueryTask(ctx context.Context, taskID string) (*auc.QueryResponse, error) {
	queryReq := QueryRequest{
		Appid:   p.appID,
		Token:   p.accessToken,
		ID:      taskID,
		Cluster: p.cluster,
	}

	jsonData, err := json.Marshal(queryReq)
	if err != nil {
		return nil, fmt.Errorf("marshal query request failed: %w", err)
	}

	p.logger.Debug("AUC query request: %s", string(jsonData))

	client := &http.Client{}
	req, err := http.NewRequestWithContext(ctx, "POST", p.serviceURL+"/query", bytes.NewBuffer(jsonData))
	if err != nil {
		return nil, fmt.Errorf("create query request failed: %w", err)
	}

	req.Header.Set("Authorization", fmt.Sprintf("Bearer; %s", p.accessToken))
	req.Header.Set("Content-Type", "application/json")

	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("query request failed: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read query response failed: %w", err)
	}

	p.logger.Debug("AUC query response: %s", string(body))

	var queryResp auc.QueryResponse
	if err := json.Unmarshal(body, &queryResp); err != nil {
		return nil, fmt.Errorf("unmarshal query response failed: %w", err)
	}

	return &queryResp, nil
}

// Initialize 实现Provider接口的Initialize方法
func (p *Provider) Initialize() error {
	p.logger.Info("Doubao AUC provider initialized")
	return nil
}

// Cleanup 实现Provider接口的Cleanup方法
func (p *Provider) Cleanup() error {
	p.logger.Info("Doubao AUC provider cleaned up")
	return nil
}

func init() {
	// 注册豆包AUC提供者
	auc.Register("doubao", func(config *auc.Config, logger *utils.Logger) (auc.Provider, error) {
		return NewProvider(config, logger)
	})
}
