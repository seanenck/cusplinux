OUTPUT := build/

all:
	mkdir -p $(OUTPUT)
	go run main.go --config config.yaml --output $(PWD)/$(OUTPUT)

clean:
	rm -rf $(OUTPUT)
