# ------------------------------------------------------------------------------------------------------------- #
# -- TEST
# ------------------------------------------------------------------------------------------------------------- #

DIR          := .
SETUP_FILE   := $(DIR)/.ignore.setup
CONFIG_FILE  := $(DIR)/config.yaml
UDLPB_CONFIG := $(DIR)/udplb.yaml
NETNS_FILE   := $(DIR)/.ignore.netns

IMAGE_URL    := https://dl-cdn.alpinelinux.org/alpine/v3.21/releases/cloud/nocloud_alpine-3.21.2-x86_64-bios-cloudinit-r0.qcow2
BASE_IMAGE   := $(DIR)/.ignore.nocloud_alpine.qcow2
IMAGE        := $(DIR)/.ignore.e2e.qcow2

RUN        := sudo -E nsenter -n$(NETNS_FILE) go run $(DIR)
RUN_STRACE := sudo -E nsenter -n$(NETNS_FILE) strace go run $(DIR)

exec-netns:
	@echo sudo nsenter -n$(NETNS_FILE)

$(BASE_IMAGE):
	curl -sflo $(BASE_IMAGE) $(IMAGE_URL)

$(IMAGE): $(BASE_IMAGE)
	echo TODO: prepare image

$(NETNS_FILE):
	touch $(NETNS_FILE)
	sudo unshare --net=$(NETNS_FILE) true

$(SETUP_FILE): $(NETNS_FILE) $(IMAGE)
	$(RUN) $(CONFIG_FILE) setup
	touch $(SETUP_FILE)

setup: $(SETUP_FILE)

run: $(SETUP_FILE)
	$(RUN) $(CONFIG_FILE) run

teardown:
	sudo umount $(NETNS_FILE)
	sleep 1
	sudo rm $(NETNS_FILE)

test: setup run teardown
