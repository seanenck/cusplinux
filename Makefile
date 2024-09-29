OUTPUT := build/

all:
	mkdir -p $(OUTPUT)
	go run main.go --config config.yaml --output $(OUTPUT)
