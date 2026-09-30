import asyncio, random, secrets
from services.common.app import create_app
from services.common.models import ReserveRequest
app = create_app("inventory", 8003)

@app.post("/reserve")
async def reserve(body: ReserveRequest):
    await asyncio.sleep(random.uniform(0.008, 0.015))
    return {"reservation_id": "res_" + secrets.token_hex(6), "status": "reserved"}
