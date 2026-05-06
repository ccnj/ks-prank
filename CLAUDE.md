# CLAUDE.md

This repository now uses `AGENTS.md` as the authoritative working guide.

`ks-prank` is no longer a Kuaishou-only helper. It is a LuckPets live-room prank assistant that supports both Kuaishou and Douyin accounts through the shared login, profile, prank-rule, MQTT, and action-dispatch flow.

Before making changes, read `AGENTS.md` and follow the current architecture there instead of older local-YAML examples or Kuaishou-only assumptions.

Quick commands:

```bash
wails dev
wails build
go test ./...
cd frontend && npm run build
```

Keep platform-specific names only where they describe platform code or server contracts, such as `internal/service/kuaishou.go`, `internal/service/douyin.go`, `proto/kuaishou.proto`, and `/api/v1/fight/low_security/add_ks_gift_log`.
