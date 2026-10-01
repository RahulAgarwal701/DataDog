# Demo commands

Copy-paste only. No talking points — just the commands, in order.
`<sub>` lines under each command are for you, not to be pasted.

Run Part 1 about 10 minutes before the supervisor arrives.

---

## Part 1 — before he gets there

```bash
make demo
```

<sub>~33s, ends "dashboard http://localhost:3000"</sub>

```bash
make -C demo preflight
```

<sub>must end <b>READY</b></sub>

```bash
curl -s localhost:8010/profile
```

<sub>must say <code>{"profile":"baseline","rps":5.0}</code> — never run <code>load-stop</code></sub>

<sub>then leave it alone 5+ min so the graphs build up history</sub>

### Optional — real eBPF collector (no password needed if you skip it)

```bash
make -C demo netmon-off
```

<sub>stops the simulator. <b>Must</b> run before the sudo command, or you double-count.</sub>

```bash
sudo ./linux/ebpf/bin/agent --pretty --topology-url http://127.0.0.1:9003 --registry ./darsan/contracts/registry.yaml
```

<sub>password, then leave in a <b>second terminal</b>. Closing it kills the agent.</sub>

```bash
make -C demo preflight
```

<sub>want: "simulator off -- real eBPF agent owns" + an agent PID listed</sub>

<sub>reload browser (Ctrl+Shift+R), Network tab should say <code>host conputrr</code></sub>

---

## Part 2 — the demo

### 1. Scope

<sub>click nothing on Overview</sub>

### 2. Overview

<sub>point at the 4 green cards, don't click</sub>

### 3. Metrics

<sub>select <code>payments</code>, wait for the line to draw (~20s)</sub>

### 4. Logs

```bash
curl -s "localhost:9002/api/v1/logs?limit=3" | head -c 300
```

<sub>optional. In the UI: click any <code>request_id</code> to trace it, then set
service <code>orders</code> + severity <code>ERROR</code> and leave it for step 8</sub>

### 5. Network

<sub>point at the arrow numbers, explain 300/min. Show the source label.</sub>

### 6. Latency fault

```bash
make -C demo fault-latency
```

```bash
sleep 15
```

<sub>p95 ~98ms -> ~950ms</sub>

### 7. Error fault

```bash
make -C demo fault-error
```

```bash
sleep 30
```

<sub>error rate climbs</sub>

### 8. Connection refusal — the strongest moment

```bash
make -C demo fault-refuse
```

```bash
sleep 10
```

<sub>orders→payments arrow <b>RED</b>. Switch to Logs tab, new <code>Errno 111</code> lines arriving.</sub>

### 9. Recovery

```bash
make -C demo fault-restore
```

```bash
sleep 60
```

<sub>arrow greys out on its own (~68s). Counts failures over a rolling 60s window.</sub>

### 10. Close

```bash
git log --oneline
```

```bash
git log --oneline --reverse | sed -n '7p'
```

<sub>the contract commit — your "proof not a claim" moment</sub>

---

## Part 3 — if it goes wrong

```bash
make -C demo reset
```

<sub>~5s. clears faults + wipes graph. safe mid-demo. arrow still red after 90s? run this.</sub>

```bash
make -C demo ebpf-agents
```

<sub>edge counts look doubled? two collectors running.</sub>

```bash
make -C demo netmon-sim-start
```

<sub>collector died and you don't want to type a password. label flips to SIMULATED.</sub>

```bash
make demo
```

<sub>total death</sub>

---

## Part 4 — if he asks about eBPF

```bash
python3 -c "
import json,collections
c=collections.Counter()
for l in open('linux/docs/samples/events.sample.jsonl'):
    c[json.loads(l)['event_type']]+=1
print(sum(c.values()),'events'); print(dict(c))"
```

<sub>2298 · 1108 CONNECT · 1108 CLOSE · 82 CONNECT_FAILED — real kernel capture, committed</sub>

```bash
ls -l linux/ebpf/internal/loader/netmon_bpfel.o
```

<sub>9320 bytes of compiled eBPF bytecode</sub>

```bash
git log --oneline -- linux/ebpf
```

<sub>your commits</sub>

---

## Never run these

| Command | Why |
|---|---|
| `make -C demo load-stop` | stops traffic — every panel goes flat and stays flat |
| `make -C demo down` | kills everything; takes 33s to bring back |
| `sudo make -C demo netmon-ebpf` | no polkit agent on this box, it cannot prompt |

---

## One-line summary

| Timing | |
|---|---|
| latency visible | 15s |
| error visible | 30s |
| refusal → red arrow | 10s |
| restore → arrow clears | 60s |