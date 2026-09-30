# Mini Datadog - root entry points. Everything lives in demo/; see demo/DEMO.md.
.DEFAULT_GOAL := help
.PHONY: help demo up down smoke faults check ps logs clean

help:
	@$(MAKE) --no-print-directory -C demo help

demo:
	@$(MAKE) --no-print-directory -C demo demo
up:
	@$(MAKE) --no-print-directory -C demo up
down:
	@$(MAKE) --no-print-directory -C demo down
ps:
	@$(MAKE) --no-print-directory -C demo ps
logs:
	@$(MAKE) --no-print-directory -C demo logs
smoke:
	@$(MAKE) --no-print-directory -C demo smoke
check:
	@$(MAKE) --no-print-directory -C demo check
faults:
	@$(MAKE) --no-print-directory -C demo faults
clean:
	@$(MAKE) --no-print-directory -C demo nuke
