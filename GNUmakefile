# custom_dev：GNU make 会优先读取本文件，上游 Makefile 保持原样以减少同步冲突。
# 命令行目标全部属于 CUSTOM_DEV_GOALS 时，改走 scripts/dev-custom.sh；
# 其余情况（包括不带目标、dev-app、dev-frontend）原样使用上游 Makefile。
CUSTOM_DEV_GOALS := dev-start dev-local dev-docker dev-docreader dev-stop dev-restart dev-logs dev-status

ifeq ($(MAKECMDGOALS),)
include Makefile
else ifneq ($(filter-out $(CUSTOM_DEV_GOALS),$(MAKECMDGOALS)),)
include Makefile
else
DEV_CUSTOM := ./scripts/dev-custom.sh

.PHONY: $(CUSTOM_DEV_GOALS)

dev-start:
	$(DEV_CUSTOM) start $(DEV_ARGS)

dev-local:
	$(DEV_CUSTOM) start --infra-only $(DEV_ARGS)

dev-docker:
	$(DEV_CUSTOM) start --docker $(DEV_ARGS)

dev-docreader:
	$(DEV_CUSTOM) docreader

dev-stop:
	$(DEV_CUSTOM) stop

dev-restart:
	$(DEV_CUSTOM) restart $(DEV_ARGS)

dev-logs:
	$(DEV_CUSTOM) logs

dev-status:
	$(DEV_CUSTOM) status
endif
