import os, secrets
from fastapi import Request
from services.common.app import call, create_app
from services.common.models import ChargeRequest, OrderRequest
app = create_app("orders", 8001)
PAYMENTS_URL = os.getenv("PAYMENTS_URL", "http://localhost:8002")

@app.post("/orders", status_code=201)
async def orders(body: OrderRequest, request: Request):
    oid = "ord_" + secrets.token_hex(6)
    r = await call(request, "payments", PAYMENTS_URL, "/charge",
                   ChargeRequest(order_id=oid, amount=body.amount, item_id=body.item_id, quantity=body.quantity).model_dump())
    return {"order_id": oid, "status": "confirmed" if r["status"] == "captured" else "failed",
            "request_id": request.state.request_id}
