"""支付服务"""

from datetime import datetime
from decimal import Decimal
from typing import Any, List, Optional

import stripe
from databases import Database

from app.config import settings
from app.constants.user import UserConstant
from app.exceptions import BusinessException, ErrorCode
from app.models.enums import PaymentStatusEnum, ProductTypeEnum
from app.schemas.payment import PaymentRecordVO


class PaymentService:
    """支付服务"""

    CURRENCY_USD = "usd"
    CENTS_MULTIPLIER = Decimal("100")

    def __init__(self, db: Database):
        self.db = db
        stripe.api_key = settings.stripe_api_key

    async def create_vip_payment_session(self, user_id: int) -> str:
        """创建 VIP 永久会员支付会话"""
        self._ensure_stripe_ready(require_webhook=False)
        user = await self._get_user_or_throw(user_id)
        if user["userRole"] == UserConstant.VIP_ROLE:
            raise BusinessException(ErrorCode.OPERATION_ERROR, "您已经是永久会员")

        product_type = ProductTypeEnum.VIP_PERMANENT
        amount_cents = int(product_type.price * self.CENTS_MULTIPLIER)

        session = stripe.checkout.Session.create(
            mode="payment",
            success_url=settings.stripe_success_url,
            cancel_url=settings.stripe_cancel_url,
            line_items=[
                {
                    "price_data": {
                        "currency": self.CURRENCY_USD,
                        "unit_amount": amount_cents,
                        "product_data": {
                            "name": product_type.description,
                            "description": "解锁全部高级功能，无限创作配额，终身有效",
                        },
                    },
                    "quantity": 1,
                }
            ],
            metadata={
                "userId": str(user_id),
                "productType": product_type.value,
            },
        )

        await self.db.execute(
            query="""
                INSERT INTO payment_record (
                    userId, stripeSessionId, amount, currency, status, productType, description
                )
                VALUES (
                    :userId, :stripeSessionId, :amount, :currency, :status, :productType, :description
                )
            """,
            values={
                "userId": user_id,
                "stripeSessionId": session.id,
                "amount": str(product_type.price),
                "currency": self.CURRENCY_USD,
                "status": PaymentStatusEnum.PENDING.value,
                "productType": product_type.value,
                "description": product_type.description,
            },
        )
        return session.url

    def construct_event(self, payload: str, sig_header: str) -> Any:
        """验证 Stripe webhook 签名并构造事件"""
        self._ensure_stripe_ready(require_webhook=True)
        return stripe.Webhook.construct_event(payload, sig_header, settings.stripe_webhook_secret)

    async def handle_payment_success(self, session: Any):
        """处理支付成功回调（幂等）"""
        session_id = getattr(session, "id", None) or session.get("id")
        metadata = getattr(session, "metadata", None) or session.get("metadata", {})
        user_id_value = metadata.get("userId")
        payment_intent = getattr(session, "payment_intent", None) or session.get("payment_intent")
        payment_intent_id = payment_intent if isinstance(payment_intent, str) else getattr(payment_intent, "id", None)

        if not session_id or not user_id_value:
            return

        record = await self.db.fetch_one(
            query="""
                SELECT id, status
                FROM payment_record
                WHERE stripeSessionId = :stripeSessionId
            """,
            values={"stripeSessionId": session_id},
        )
        if not record:
            return

        if record["status"] == PaymentStatusEnum.SUCCEEDED.value:
            return

        async with self.db.transaction():
            await self.db.execute(
                query="""
                    UPDATE payment_record
                    SET status = :status, stripePaymentIntentId = :stripePaymentIntentId
                    WHERE id = :id
                """,
                values={
                    "id": record["id"],
                    "status": PaymentStatusEnum.SUCCEEDED.value,
                    "stripePaymentIntentId": payment_intent_id,
                },
            )
            await self.db.execute(
                query="""
                    UPDATE user
                    SET userRole = :userRole, vipTime = :vipTime
                    WHERE id = :id
                """,
                values={
                    "id": int(user_id_value),
                    "userRole": UserConstant.VIP_ROLE,
                    "vipTime": datetime.now(),
                },
            )

    async def handle_refund(self, user_id: int, reason: Optional[str]) -> bool:
        """处理退款并撤销 VIP"""
        self._ensure_stripe_ready(require_webhook=False)
        user = await self._get_user_or_throw(user_id)
        if user["userRole"] != UserConstant.VIP_ROLE:
            raise BusinessException(ErrorCode.OPERATION_ERROR, "您不是会员，无法退款")

        payment_record = await self.db.fetch_one(
            query="""
                SELECT id, stripePaymentIntentId
                FROM payment_record
                WHERE userId = :userId
                  AND status = :status
                  AND productType = :productType
                ORDER BY createTime DESC
                LIMIT 1
            """,
            values={
                "userId": user_id,
                "status": PaymentStatusEnum.SUCCEEDED.value,
                "productType": ProductTypeEnum.VIP_PERMANENT.value,
            },
        )
        if not payment_record:
            raise BusinessException(ErrorCode.NOT_FOUND_ERROR, "未找到支付记录")
        if not payment_record["stripePaymentIntentId"]:
            raise BusinessException(ErrorCode.OPERATION_ERROR, "支付记录无效")

        refund = stripe.Refund.create(
            payment_intent=payment_record["stripePaymentIntentId"],
            reason="requested_by_customer",
        )
        if getattr(refund, "status", None) != "succeeded":
            return False

        async with self.db.transaction():
            await self.db.execute(
                query="""
                    UPDATE payment_record
                    SET status = :status, refundTime = :refundTime, refundReason = :refundReason
                    WHERE id = :id
                """,
                values={
                    "id": payment_record["id"],
                    "status": PaymentStatusEnum.REFUNDED.value,
                    "refundTime": datetime.now(),
                    "refundReason": reason,
                },
            )
            await self.db.execute(
                query="""
                    UPDATE user
                    SET userRole = :userRole, vipTime = NULL, quota = :quota
                    WHERE id = :id
                """,
                values={
                    "id": user_id,
                    "userRole": UserConstant.DEFAULT_ROLE,
                    "quota": UserConstant.DEFAULT_QUOTA,
                },
            )
        return True

    async def get_payment_records(self, user_id: int) -> List[PaymentRecordVO]:
        """获取用户支付记录"""
        records = await self.db.fetch_all(
            query="""
                SELECT *
                FROM payment_record
                WHERE userId = :userId
                ORDER BY createTime DESC
                """,
            values={"userId": user_id},
        )
        return [self._to_payment_record_vo(record) for record in records]

    async def _get_user_or_throw(self, user_id: int):
        user = await self.db.fetch_one(
            query="SELECT id, userRole FROM user WHERE id = :id AND isDelete = 0",
            values={"id": user_id},
        )
        if not user:
            raise BusinessException(ErrorCode.NOT_FOUND_ERROR, "用户不存在")
        return user

    def _to_payment_record_vo(self, record: Any) -> PaymentRecordVO:
        record_dict = dict(record)
        return PaymentRecordVO(
            id=record_dict["id"],
            userId=record_dict["userId"],
            stripeSessionId=record_dict.get("stripeSessionId"),
            stripePaymentIntentId=record_dict.get("stripePaymentIntentId"),
            amount=float(record_dict["amount"]),
            currency=record_dict["currency"],
            status=record_dict["status"],
            productType=record_dict["productType"],
            description=record_dict.get("description"),
            refundTime=record_dict["refundTime"].isoformat() if record_dict.get("refundTime") else None,
            refundReason=record_dict.get("refundReason"),
            createTime=record_dict["createTime"].isoformat(),
            updateTime=record_dict["updateTime"].isoformat(),
        )

    def _ensure_stripe_ready(self, require_webhook: bool):
        """校验 Stripe 配置是否可用"""
        if not settings.stripe_api_key:
            raise BusinessException(ErrorCode.SYSTEM_ERROR, "Stripe API Key 未配置")
        if require_webhook and not settings.stripe_webhook_secret:
            raise BusinessException(ErrorCode.SYSTEM_ERROR, "Stripe Webhook Secret 未配置")

