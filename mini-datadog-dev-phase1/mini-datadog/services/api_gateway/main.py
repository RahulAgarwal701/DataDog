import os
from fastapi import Request
from services.common.app import call, create_app
from services.common.models import OrderRequest
app = create_app("api-gateway", 8000)
ORDERS_URL = os.getenv("ORDERS_URL", "http://localhost:8001")

@app.post("/orders", status_code=201)
async def orders(body: OrderRequest, request: Request):
    return await call(request, "orders", ORDERS_URL, "/orders", body.model_dump())
