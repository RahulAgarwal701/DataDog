import asyncio, os, random, secrets
from services.common.app import create_app
from services.common.models import LoadProfile
GATEWAY_URL = os.getenv("GATEWAY_URL", "http://localhost:8000")
DEFAULT = {"baseline": 5.0, "surge": 20.0}
state = {"profile": "baseline" if os.getenv("AUTOSTART", "true") == "true" else "stop", "rps": 5.0}

async def fire(app):
    body = {"customer_id": f"c{random.randint(1, 50)}", "item_id": f"item{random.randint(1, 20)}",
            "quantity": random.randint(1, 3), "amount": round(random.uniform(5, 200), 2)}
    try:
        await app.state.client.post(GATEWAY_URL + "/orders", json=body,
                                    headers={"X-Request-ID": "req_" + secrets.token_hex(8)})
    except Exception:
        pass

async def loop(app):
    while True:
        if state["profile"] == "stop":
            await asyncio.sleep(0.5); continue
        asyncio.create_task(fire(app))
        await asyncio.sleep(1 / state["rps"])

async def startup(app):
    asyncio.create_task(loop(app))

app = create_app("loadgen", 8010, startup)

@app.get("/profile")
async def get_profile():
    return state

@app.post("/profile")
async def set_profile(p: LoadProfile):
    state["profile"] = p.profile
    if p.profile != "stop":
        state["rps"] = p.rps or DEFAULT[p.profile]
    return state
