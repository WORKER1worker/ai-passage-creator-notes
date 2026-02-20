"""支付相关请求/响应模型"""

from typing import Optional
from pydantic import BaseModel, Field


class RefundRequest(BaseModel):
    """退款请求"""

    reason: Optional[str] = Field(None, description="退款原因")


class PaymentRecordVO(BaseModel):
    """支付记录视图"""

    id: int
    user_id: int = Field(..., alias="userId")
    stripe_session_id: Optional[str] = Field(None, alias="stripeSessionId")
    stripe_payment_intent_id: Optional[str] = Field(None, alias="stripePaymentIntentId")
    amount: float
    currency: str
    status: str
    product_type: str = Field(..., alias="productType")
    description: Optional[str] = None
    refund_time: Optional[str] = Field(None, alias="refundTime")
    refund_reason: Optional[str] = Field(None, alias="refundReason")
    create_time: str = Field(..., alias="createTime")
    update_time: str = Field(..., alias="updateTime")

    class Config:
        populate_by_name = True

