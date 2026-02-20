"""图文合并智能体适配器"""

from app.schemas.article import ArticleState


class ContentMergerAgent:
    """图文合并智能体"""

    def run(self, service, state: ArticleState):
        service.merge_images_into_content(state)

