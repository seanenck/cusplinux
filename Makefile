OUTPUT  := build/
CONFIGS := $(wildcard *.yaml)

.PHONY: $(CONFIGS)

all: setup $(CONFIGS)

setup:
	mkdir -p $(OUTPUT)

$(CONFIGS):
	@go run main.go --config $@ --output $(PWD)/$(OUTPUT)

clean:
	rm -rf $(OUTPUT)
