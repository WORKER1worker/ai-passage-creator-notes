package model

// CreateArticleRequest 创建文章请求
type CreateArticleRequest struct {
	Topic string `json:"topic" binding:"required"`
}

// QueryArticleRequest 查询文章请求
type QueryArticleRequest struct {
	UserID   *int64  `json:"userId"`
	Status   *string `json:"status"`
	PageNum  int64   `json:"pageNum"`
	PageSize int64   `json:"pageSize"`
}
