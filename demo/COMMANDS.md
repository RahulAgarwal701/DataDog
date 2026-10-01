```bash
make demo
```

```bash
make -C demo preflight
```

```bash
make -C demo netmon-off
```

```bash
sudo ./linux/ebpf/bin/agent --pretty --topology-url http://127.0.0.1:9003 --registry ./darsan/contracts/registry.yaml
```

```bash
make -C demo preflight
```

```bash
make -C demo fault-latency
```

```bash
sleep 15
```

```bash
make -C demo fault-error
```

```bash
sleep 30
```

```bash
make -C demo fault-refuse
```

```bash
sleep 10
```

```bash
make -C demo fault-restore
```

```bash
sleep 60
```

```bash
git log --oneline
```

```bash
git log --oneline --reverse | sed -n '7p'
```

```bash
make -C demo reset
```