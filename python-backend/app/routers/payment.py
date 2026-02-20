"""支付路由"""

from typing import List, Optional

from databases import Database
from fastapi import APIRouter, Depends, Header, Request

from app.database import get_db
from app.constants.user import UserConstant
from app.deps import require_login
from app.exceptions import BusinessException, ErrorCode
from app.schemas.common import BaseResponse
from app.schemas.payment import PaymentRecordVO
from app.schemas.user import LoginUserVO
from app.services.payment_service import PaymentService

payment_router = APIRouter(prefix="/payment", tags=["支付管理"])
webhook_router = APIRouter(prefix="/webhook", tags=["支付回调"])


@payment_router.post("/create-vip-session", response_model=BaseResponse[str])
async def create_vip_payment_session(
    db: Database = Depends(get_db),
    current_user: LoginUserVO = Depends(require_login),
):
    """创建 VIP 支付会话"""
    service = PaymentService(db)
    session_url = await service.create_vip_payment_session(current_user.id)
    return BaseResponse.success(data=session_url)


@payment_router.post("/refund", response_model=BaseResponse[bool])
async def refund(
    reason: Optional[str] = None,
    db: Database = Depends(get_db),
    current_user: LoginUserVO = Depends(require_login),
):
    """申请退款"""
    if current_user.user_role != UserConstant.VIP_ROLE:
        raise BusinessException(ErrorCode.NO_AUTH_ERROR, "仅 VIP 会员可退款")
    service = PaymentService(db)
    success = await service.handle_refund(current_user.id, reason)
    return BaseResponse.success(data=success)


@payment_router.get("/records", response_model=BaseResponse[List[PaymentRecordVO]])
async def get_payment_records(
    db: Database = Depends(get_db),
    current_user: LoginUserVO = Depends(require_login),
):
    """获取当前用户支付记录"""
    service = PaymentService(db)
    records = await service.get_payment_records(current_user.id)
    return BaseResponse.success(data=records)


@webhook_router.post("/stripe")
async def stripe_webhook(
    http_request: Request,
    stripe_signature: str = Header(..., alias="Stripe-Signature"),
    db: Database = Depends(get_db),
):
    """Stripe webhook 回调"""
    payload = (await http_request.body()).decode("utf-8")
    service = PaymentService(db)
    try:
        event = service.construct_event(payload, stripe_signature)
        event_type = getattr(event, "type", None) or event.get("type")
        data_object = None
        if hasattr(event, "data") and getattr(event.data, "object", None):
            data_object = event.data.object
        elif isinstance(event, dict):
            data_object = event.get("data", {}).get("object")

        if event_type in {"checkout.session.completed", "checkout.session.async_payment_succeeded"}:
            await service.handle_payment_success(data_object)
        return "success"
    except Exception:
        return "error"

