package service

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"strings"

	"github.com/tmc/langchaingo/llms"
	"github.com/tmc/langchaingo/llms/openai"
	"github.com/yupi/ai-passage-creator/internal/common"
	"github.com/yupi/ai-passage-creator/internal/config"
	"github.com/yupi/ai-passage-creator/internal/model"
)

// ArticleAgentService 文章智能体编排服务
type ArticleAgentService struct {
	llm        llms.Model
	pexels     *PexelsService
	cos        *CosService
	sseManager *common.SSEManager
}

// NewArticleAgentService 创建文章智能体服务
// 使用 LangChainGo OpenAI 客户端连接 DashScope（OpenAI 兼容）
func NewArticleAgentService(cfg *config.Config, pexels *PexelsService, cos *CosService, sseManager *common.SSEManager) (*ArticleAgentService, error) {
	baseURL := "https://dashscope.aliyuncs.com/compatible-mode/v1"

	// 添加调试日志
	log.Printf("初始化 DashScope 客户端: BaseURL=%s, Model=%s, APIKey=%s...",
		baseURL, cfg.AI.DashScope.Model, maskAPIKey(cfg.AI.DashScope.APIKey))

	llm, err := openai.New(
		openai.WithToken(cfg.AI.DashScope.APIKey),
		openai.WithModel(cfg.AI.DashScope.Model),
		openai.WithBaseURL(baseURL),
	)
	if err != nil {
		log.Printf("创建 DashScope 客户端失败: %v", err)
		return nil, fmt.Errorf("create dashscope client: %w", err)
	}

	log.Printf("DashScope 客户端初始化成功")

	return &ArticleAgentService{
		llm:        llm,
		pexels:     pexels,
		cos:        cos,
		sseManager: sseManager,
	}, nil
}

// maskAPIKey 遮蔽 API Key 用于日志
func maskAPIKey(key string) string {
	if len(key) <= 10 {
		return "***"
	}
	return key[:10] + "***"
}

// Execute 执行完整的文章生成流程
func (s *ArticleAgentService) Execute(ctx context.Context, state *model.ArticleState) error {
	// 智能体1：生成标题
	log.Printf("智能体1：开始生成标题, taskId=%s", state.TaskID)
	if err := s.agent1GenerateTitle(ctx, state); err != nil {
		return fmt.Errorf("agent1 failed: %w", err)
	}
	s.sendMessage(state.TaskID, map[string]interface{}{
		"type":  "AGENT1_COMPLETE",
		"title": state.Title,
	})

	// 智能体2：生成大纲（流式）
	log.Printf("智能体2：开始生成大纲, taskId=%s", state.TaskID)
	if err := s.agent2GenerateOutlineStream(ctx, state); err != nil {
		return fmt.Errorf("agent2 failed: %w", err)
	}
	s.sendMessage(state.TaskID, map[string]interface{}{
		"type":    "AGENT2_COMPLETE",
		"outline": state.Outline.Sections,
	})

	// 智能体3：生成正文（流式）
	log.Printf("智能体3：开始生成正文, taskId=%s", state.TaskID)
	if err := s.agent3GenerateContent(ctx, state); err != nil {
		return fmt.Errorf("agent3 failed: %w", err)
	}
	s.sendMessage(state.TaskID, map[string]interface{}{
		"type": "AGENT3_COMPLETE",
	})

	// 智能体4：分析配图需求
	log.Printf("智能体4：开始分析配图需求, taskId=%s", state.TaskID)
	if err := s.agent4AnalyzeImageRequirements(ctx, state); err != nil {
		return fmt.Errorf("agent4 failed: %w", err)
	}
	s.sendMessage(state.TaskID, map[string]interface{}{
		"type":              "AGENT4_COMPLETE",
		"imageRequirements": state.ImageRequirements,
	})

	// 智能体5：生成配图
	log.Printf("智能体5：开始生成配图, taskId=%s", state.TaskID)
	if err := s.agent5GenerateImages(ctx, state); err != nil {
		return fmt.Errorf("agent5 failed: %w", err)
	}
	s.sendMessage(state.TaskID, map[string]interface{}{
		"type":   "AGENT5_COMPLETE",
		"images": state.Images,
	})

	// 图文合成：将配图插入正文
	log.Printf("开始图文合成, taskId=%s", state.TaskID)
	s.mergeImagesIntoContent(state)
	s.sendMessage(state.TaskID, map[string]interface{}{
		"type":        "MERGE_COMPLETE",
		"fullContent": state.FullContent,
	})

	log.Printf("文章生成完成, taskId=%s", state.TaskID)
	return nil
}

// agent1GenerateTitle 智能体1：生成标题
func (s *ArticleAgentService) agent1GenerateTitle(ctx context.Context, state *model.ArticleState) error {
	prompt := strings.ReplaceAll(common.Agent1TitlePrompt, "{topic}", state.Topic)

	log.Printf("智能体1：发送请求到 LLM, promptLength=%d", len(prompt))

	content, err := llms.GenerateFromSinglePrompt(ctx, s.llm, prompt)
	if err != nil {
		log.Printf("智能体1：LLM 调用失败, error=%v", err)
		return fmt.Errorf("LLM call failed: %w", err)
	}

	log.Printf("智能体1：收到响应, contentLength=%d, content preview=%s...",
		len(content), truncateString(content, 100))

	var title model.TitleResult
	if err := json.Unmarshal([]byte(content), &title); err != nil {
		log.Printf("智能体1：标题解析失败, content=%s", content)
		return fmt.Errorf("parse title failed: %w", err)
	}

	state.Title = &title
	log.Printf("智能体1：标题生成成功, mainTitle=%s", title.MainTitle)
	return nil
}

// truncateString 截断字符串用于日志
func truncateString(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen]
}

// agent2GenerateOutline 智能体2：生成大纲
func (s *ArticleAgentService) agent2GenerateOutline(ctx context.Context, state *model.ArticleState) error {
	prompt := strings.ReplaceAll(common.Agent2OutlinePrompt, "{mainTitle}", state.Title.MainTitle)
	prompt = strings.ReplaceAll(prompt, "{subTitle}", state.Title.SubTitle)

	content, err := llms.GenerateFromSinglePrompt(ctx, s.llm, prompt)
	if err != nil {
		return err
	}

	var outline model.OutlineResult
	if err := json.Unmarshal([]byte(content), &outline); err != nil {
		log.Printf("智能体2：大纲解析失败, content=%s", content)
		return fmt.Errorf("parse outline failed: %w", err)
	}

	state.Outline = &outline
	log.Printf("智能体2：大纲生成成功, sections=%d", len(outline.Sections))
	return nil
}

// agent3GenerateContent 智能体3：生成正文（流式）
func (s *ArticleAgentService) agent3GenerateContent(ctx context.Context, state *model.ArticleState) error {
	outlineJSON, _ := json.Marshal(state.Outline.Sections)
	prompt := strings.ReplaceAll(common.Agent3ContentPrompt, "{mainTitle}", state.Title.MainTitle)
	prompt = strings.ReplaceAll(prompt, "{subTitle}", state.Title.SubTitle)
	prompt = strings.ReplaceAll(prompt, "{outline}", string(outlineJSON))

	var contentBuilder strings.Builder

	// 流式生成
	_, err := s.llm.GenerateContent(ctx, []llms.MessageContent{
		llms.TextParts(llms.ChatMessageTypeHuman, prompt),
	}, llms.WithStreamingFunc(func(ctx context.Context, chunk []byte) error {
		text := string(chunk)
		contentBuilder.WriteString(text)

		// 推送流式内容
		s.sendMessage(state.TaskID, map[string]interface{}{
			"type":    "AGENT3_STREAMING",
			"content": text,
		})
		return nil
	}))

	if err != nil {
		return err
	}

	state.Content = contentBuilder.String()
	log.Printf("智能体3：正文生成成功, length=%d", len(state.Content))
	return nil
}

// agent4AnalyzeImageRequirements 智能体4：分析配图需求
func (s *ArticleAgentService) agent4AnalyzeImageRequirements(ctx context.Context, state *model.ArticleState) error {
	prompt := strings.ReplaceAll(common.Agent4ImagePrompt, "{mainTitle}", state.Title.MainTitle)
	prompt = strings.ReplaceAll(prompt, "{content}", state.Content)

	content, err := llms.GenerateFromSinglePrompt(ctx, s.llm, prompt)
	if err != nil {
		return err
	}

	var requirements []model.ImageRequirement
	if err := json.Unmarshal([]byte(content), &requirements); err != nil {
		log.Printf("智能体4：配图需求解析失败, content=%s", content)
		return fmt.Errorf("parse image requirements failed: %w", err)
	}

	state.ImageRequirements = requirements
	log.Printf("智能体4：配图需求分析成功, count=%d", len(requirements))
	return nil
}

// agent5GenerateImages 智能体5：生成配图
func (s *ArticleAgentService) agent5GenerateImages(ctx context.Context, state *model.ArticleState) error {
	var imageResults []model.ImageResult

	for _, req := range state.ImageRequirements {
		log.Printf("智能体5：开始检索配图, position=%d, keywords=%s", req.Position, req.Keywords)

		// 调用 Pexels API
		imageURL, err := s.pexels.SearchImage(req.Keywords)
		method := "PEXELS"

		// 降级策略
		if err != nil {
			imageURL = s.pexels.GetFallbackImage(req.Position)
			method = "PICSUM"
			log.Printf("智能体5：Pexels 检索失败,使用降级方案, position=%d", req.Position)
		}

		// MVP 阶段直接使用 URL
		finalURL := s.cos.UseDirectURL(imageURL)

		result := model.ImageResult{
			Position:     req.Position,
			URL:          finalURL,
			Method:       method,
			Keywords:     req.Keywords,
			SectionTitle: req.SectionTitle,
			Description:  req.Type,
		}

		imageResults = append(imageResults, result)

		// 推送单张配图完成
		s.sendMessage(state.TaskID, map[string]interface{}{
			"type":  "IMAGE_COMPLETE",
			"image": result,
		})

		log.Printf("智能体5：配图检索成功, position=%d, method=%s", req.Position, method)
	}

	state.Images = imageResults
	log.Printf("智能体5：所有配图生成完成, count=%d", len(imageResults))
	return nil
}

// sendMessage 发送 SSE 消息
func (s *ArticleAgentService) sendMessage(taskID string, data interface{}) {
	s.sseManager.Send(taskID, data)
}
