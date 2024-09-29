all:
	mkdir -p build/
	find $(HOME)/build/
	go run main.go --config config.yaml
