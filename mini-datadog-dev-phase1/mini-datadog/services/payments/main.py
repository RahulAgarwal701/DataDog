import asyncio, os, random, secrets
from fastapi import Request
from services.common.app import call, create_app
from services.common.models import ChargeRequest, ReserveRequest
app = create_app("payments", 8002)
INVENTORY_URL = os.getenv("INVENTORY_URL", "http://localhost:8003")

@app.post("/charge")
async def charge(body: ChargeRequest, request: Request):
    await asyncio.sleep(random.uniform(0.04, 0.06))  # ~50 ms baseline
    r = await call(request, "inventory", INVENTORY_URL, "/reserve",
                   ReserveRequest(order_id=body.order_id, item_id=body.item_id, quantity=body.quantity).model_dump())
    return {"payment_id": "pay_" + secrets.token_hex(6),
            "status": "captured" if r["status"] == "reserved" else "declined"}
