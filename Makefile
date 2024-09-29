OUTPUT  := build/
CONFIGS := $(wildcard *.yaml)

.PHONY: $(CONFIGS)

all: $(CONFIGS)
	mkdir -p $(OUTPUT)

$(CONFIGS):
	@go run main.go --config $@ --output $(PWD)/$(OUTPUT)

clean:
	rm -rf $(OUTPUT)
