# AGENTS.md - Project Guidelines for AI Agents

## Overview
This repository contains a Go daemon and WebUI (`fm350-monitor`) designed to manage the Fibocom FM350-GL 5G USB modem on Linux.

## Core Rules for Agents
1. **Lazy Senior Developer Principle**: Build what's requested with minimal abstraction. Prefer Go stdlib over extra dependencies.
2. **Architecture** (layered):
   - `cmd/app/`: Entrypoint & wiring only (`go build -o fm350-manager ./cmd/app`).
   - `internal/pkg/domain/`: Pure domain models + string/int enums (RAT, reg, tech, PDP, SIM, power).
   - `internal/pkg/appdefaults/`: Process defaults (ports, intervals, recovery thresholds).
   - `internal/repository/`: Sysfs USB, serial AT, history persistence, mbimcli; AT cmds/paths in `at_cmd.go` / `paths.go`.
   - `internal/usecase/`: Application workflows & recovery policy.
   - `internal/handler/`: HTTP handlers, SSE, routing (no direct serial/sysfs).
   - `internal/template/`: Feature-based WebUI (`assets/layout` + `assets/features` + `assets/shared`); `go:embed` + `html/template` render.
   - `deploy/`: Systemd unit and packaging helpers.
   - `docs/`: Technical documentation and AT command reference.
3. **No magic strings for closed sets**: Use domain enums (`RATMode`, `RATModePref`, `RegState`, …). Wire protocol literals only in repository const files. JSON values must stay stable for the WebUI.
4. **Dependency direction**: `cmd/app` → handler/usecase/repository; `handler` → usecase + template (not repository); `usecase` → repository ports; `repository` → domain + external I/O only.
5. **No Unneeded Dependencies**: Only `go.bug.st/serial` is allowed for serial communication (transitive `golang.org/x/sys` OK). Web UI must remain zero-build-step vanilla HTML/CSS/JS embedded via `go:embed`. Prefer stdlib + optional external CLI (`mbimcli`) over new Go modules.
6. **Testing**: Non-trivial logic must leave runnable tests using Go's `testing` stdlib (domain enums, repository parsers/USB/history, usecase recovery, handler httptest).
