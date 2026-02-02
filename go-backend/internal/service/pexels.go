package service

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"

	"github.com/yupi/ai-passage-creator/internal/config"
)

// PexelsService Pexels 图片检索服务
type PexelsService struct {
	apiKey string
	client *http.Client
}

// NewPexelsService 创建 Pexels 服务
func NewPexelsService(cfg *config.Config) *PexelsService {
	return &PexelsService{
		apiKey: cfg.Pexels.APIKey,
		client: &http.Client{},
	}
}

// SearchImage 根据关键词检索图片
func (s *PexelsService) SearchImage(keywords string) (string, error) {
	apiURL := fmt.Sprintf("https://api.pexels.com/v1/search?query=%s&per_page=1", url.QueryEscape(keywords))

	req, err := http.NewRequest("GET", apiURL, nil)
	if err != nil {
		return "", err
	}

	req.Header.Set("Authorization", s.apiKey)

	resp, err := s.client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("pexels API error: %d", resp.StatusCode)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", err
	}

	var result struct {
		Photos []struct {
			Src struct {
				Original string `json:"original"`
				Large    string `json:"large"`
			} `json:"src"`
		} `json:"photos"`
	}

	if err := json.Unmarshal(body, &result); err != nil {
		return "", err
	}

	if len(result.Photos) == 0 {
		return "", fmt.Errorf("no image found")
	}

	return result.Photos[0].Src.Large, nil
}

// GetFallbackImage 获取降级图片（Picsum）
func (s *PexelsService) GetFallbackImage(position int) string {
	return fmt.Sprintf("https://picsum.photos/seed/%d/800/600", position)
}
