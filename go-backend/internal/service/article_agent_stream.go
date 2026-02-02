package service

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"strings"

	"github.com/tmc/langchaingo/llms"
	"github.com/yupi/ai-passage-creator/internal/common"
	"github.com/yupi/ai-passage-creator/internal/model"
)

// agent2GenerateOutlineStream 智能体2：生成大纲（流式输出）
func (s *ArticleAgentService) agent2GenerateOutlineStream(ctx context.Context, state *model.ArticleState) error {
	prompt := strings.ReplaceAll(common.Agent2OutlinePrompt, "{mainTitle}", state.Title.MainTitle)
	prompt = strings.ReplaceAll(prompt, "{subTitle}", state.Title.SubTitle)

	var contentBuilder strings.Builder

	// 流式生成
	_, err := s.llm.GenerateContent(ctx, []llms.MessageContent{
		llms.TextParts(llms.ChatMessageTypeHuman, prompt),
	}, llms.WithStreamingFunc(func(ctx context.Context, chunk []byte) error {
		text := string(chunk)
		contentBuilder.WriteString(text)

		// 推送流式内容
		s.sendMessage(state.TaskID, map[string]interface{}{
			"type":    "AGENT2_STREAMING",
			"content": text,
		})
		return nil
	}))

	if err != nil {
		return err
	}

	content := contentBuilder.String()

	var outline model.OutlineResult
	if err := json.Unmarshal([]byte(content), &outline); err != nil {
		log.Printf("智能体2：大纲解析失败, content=%s", content)
		return fmt.Errorf("parse outline failed: %w", err)
	}

	state.Outline = &outline
	log.Printf("智能体2：大纲生成成功, sections=%d", len(outline.Sections))
	return nil
}

// mergeImagesIntoContent 图文合成：将配图插入正文对应位置
func (s *ArticleAgentService) mergeImagesIntoContent(state *model.ArticleState) {
	content := state.Content
	images := state.Images

	if len(images) == 0 {
		state.FullContent = content
		return
	}

	var fullContent strings.Builder

	// 处理封面图（position=1）
	for _, img := range images {
		if img.Position == 1 {
			fullContent.WriteString(fmt.Sprintf("![封面图](%s)\n\n", img.URL))
			break
		}
	}

	// 按行处理正文，在章节标题后插入对应图片
	lines := strings.Split(content, "\n")
	for _, line := range lines {
		fullContent.WriteString(line)
		fullContent.WriteString("\n")

		// 检查是否是章节标题（以 ## 开头）
		if strings.HasPrefix(line, "## ") {
			sectionTitle := strings.TrimSpace(strings.TrimPrefix(line, "## "))
			s.insertImageAfterSection(&fullContent, images, sectionTitle)
		}
	}

	state.FullContent = fullContent.String()
	log.Printf("图文合成完成, fullContentLength=%d", len(state.FullContent))
}

// insertImageAfterSection 在章节标题后插入对应图片
func (s *ArticleAgentService) insertImageAfterSection(fullContent *strings.Builder, images []model.ImageResult, sectionTitle string) {
	for _, image := range images {
		if image.Position > 1 &&
			image.SectionTitle != "" &&
			strings.Contains(sectionTitle, strings.TrimSpace(image.SectionTitle)) {
			fullContent.WriteString(fmt.Sprintf("\n![%s](%s)\n", image.Description, image.URL))
			break
		}
	}
}
