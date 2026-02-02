package service

// CosService 腾讯云 COS 服务
type CosService struct{}

// NewCosService 创建 COS 服务
func NewCosService() *CosService {
	return &CosService{}
}

// UseDirectURL MVP 阶段直接使用图片 URL，不上传到 COS
func (s *CosService) UseDirectURL(imageURL string) string {
	return imageURL
}
