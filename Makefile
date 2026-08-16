BIN      := fm350-manager
PKG      := ./cmd/app
GO       ?= go
GOFLAGS  ?=
BIND     ?= 0.0.0.0
PORT     ?= 8080
SERIAL   ?=
ARGS     ?=
RUNTIME_DIR := .runtime
STATE_DIR   := .state
HISTORY     := $(STATE_DIR)/history.json
MAKEFLAGS += --no-print-directory

# Extra flags after BIND/PORT/history. SERIAL= sets -serial.
override RUN_FLAGS := -bind $(BIND) -port $(PORT) -history-file $(HISTORY)
ifneq ($(SERIAL),)
override RUN_FLAGS += -serial $(SERIAL)
endif
override RUN_FLAGS += $(ARGS)

.PHONY: help all build run run-sudo tty-access test test-race vet fmt fmt-check lint check dev tidy clean install uninstall setup dirs

all: build

help:
	@echo "fm350-manager"
	@echo
	@echo "  make build          go build -o $(BIN) $(PKG)"
	@echo "  make run            build + run (no sudo; $(BIND):$(PORT))"
	@echo "  make run-sudo       build + sudo run (AT/WAN/hotspot)"
	@echo "  make tty-access     ACL on /dev/ttyUSB* for this uid (docker)"
	@echo "  make dev            air live reload (no sudo)"
	@echo "  make test|check     tests / fmt-check + vet + test"
	@echo "  make setup          dialout + udev (one-time)"
	@echo
	@echo "run variables:"
	@echo "  BIND=$(BIND) PORT=$(PORT) SERIAL=$(SERIAL) ARGS='$(ARGS)'"
	@echo "  example: make run PORT=8081 SERIAL=/dev/ttyUSB2"

build:
	$(GO) build $(GOFLAGS) -o $(BIN) $(PKG)

dirs:
	@mkdir -p $(RUNTIME_DIR) $(STATE_DIR)

# Local runtime/state stay in-repo (gitignored). -no-elevate: this session may
# lack the dialout group even if $USER is in /etc/group — run `make tty-access`
# or re-login. Use run-sudo for WAN DHCP/hotspot.
run: build dirs
	@if ! id -nG | grep -qw dialout; then \
		if getent group dialout 2>/dev/null | grep -qw "$${USER}"; then \
			echo "NOTE: $${USER} is in dialout but this session is not. Re-login, or: make tty-access"; \
		else \
			echo "NOTE: $${USER} is not in dialout. make setup && re-login, or: make run-sudo"; \
		fi; \
	fi
	RUNTIME_DIRECTORY="$(CURDIR)/$(RUNTIME_DIR)" STATE_DIRECTORY="$(CURDIR)/$(STATE_DIR)" \
		./$(BIN) -no-elevate $(RUN_FLAGS)

# Grant this uid rw on FM350 ttyUSB* without a new login (needs docker group).
# /etc/group already has $USER in dialout; ACL lasts until unplug/replug.
tty-access:
	@docker run --rm --privileged -v /dev:/dev alpine:3.20 sh -c \
		'apk add --no-cache acl >/dev/null && setfacl -m u:$(shell id -u):rw /dev/ttyUSB* && echo OK && getfacl /dev/ttyUSB0 | head -12'

run-sudo: build dirs
	sudo env \
		RUNTIME_DIRECTORY="$(CURDIR)/$(RUNTIME_DIR)" \
		STATE_DIRECTORY="$(CURDIR)/$(STATE_DIR)" \
		./$(BIN) $(RUN_FLAGS)

test:
	$(GO) test ./...

test-race:
	$(GO) test -race ./...

vet:
	$(GO) vet ./...

GOFILES = $(shell find . -name '*.go' -not -path './.git/*' -not -path './RxTxSemi_FM350_Connect/*' -not -path './tmp/*')

fmt:
	gofmt -l -w $(GOFILES)

lint: vet

check: fmt-check vet test

fmt-check:
	@files="$$(gofmt -l $(GOFILES))"; \
	if [ -n "$$files" ]; then \
		echo "gofmt drift (run 'make fmt' to fix):"; \
		echo "$$files"; \
		exit 1; \
	else \
		echo "gofmt: clean"; \
	fi

# Live reload: air (https://github.com/air-verse/air). Restarts on .go changes.
# Extra app flags: edit args_bin in .air.toml.
dev: dirs
	@command -v air >/dev/null 2>&1 || { echo "air is not installed. Install it with: go install github.com/air-verse/air@latest"; exit 1; }
	air -c .air.toml

# One-time permissions so `make run`/`make dev` work without root:
#  1. Add $$USER to dialout (serial /dev/ttyUSB*)
#  2. udev: sysfs power, USBDEVFS_RESET, /dev/cdc-wdm*
#  3. Reload udev (re-login still needed for dialout)
setup:
	@if [ -z "$$USER" ]; then echo "cannot detect $$USER" >&2; exit 1; fi
	@command -v sudo >/dev/null 2>&1 || { echo "sudo is required for setup" >&2; exit 1; }
	sudo usermod -aG dialout "$$USER"
	@mkdir -p .udev
	printf '%s\n' 'SUBSYSTEM=="usb", ATTR{idVendor}=="0e8d", ATTR{idProduct}=="7127", TAG+="uaccess"' > .udev/99-fm350-monitor.rules
	printf '%s\n' 'SUBSYSTEM=="usb", ATTR{idVendor}=="0e8d", ATTR{idProduct}=="7126", TAG+="uaccess"' >> .udev/99-fm350-monitor.rules
	printf '%s\n' 'SUBSYSTEM=="tty", ATTRS{idVendor}=="0e8d", ATTRS{idProduct}=="7127", MODE="0660", GROUP="dialout"' >> .udev/99-fm350-monitor.rules
	printf '%s\n' 'SUBSYSTEM=="tty", ATTRS{idVendor}=="0e8d", ATTRS{idProduct}=="7126", MODE="0660", GROUP="dialout"' >> .udev/99-fm350-monitor.rules
	printf '%s\n' 'SUBSYSTEM=="usbmisc", KERNEL=="cdc-wdm*", MODE="0660", GROUP="dialout"' >> .udev/99-fm350-monitor.rules
	sudo install -m 0644 .udev/99-fm350-monitor.rules /etc/udev/rules.d/99-fm350-monitor.rules
	sudo udevadm control --reload-rules
	sudo udevadm trigger --subsystem-match=usb --subsystem-match=tty --subsystem-match=usbmisc
	@echo "Setup done. Re-login (or 'newgrp dialout') so dialout takes effect, then plug in the modem."

tidy:
	$(GO) mod tidy

clean:
	rm -f $(BIN)
	rm -rf tmp

install: build
	install -m 0755 $(BIN) /usr/local/bin/$(BIN)

uninstall:
	rm -f /usr/local/bin/$(BIN)
