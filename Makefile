all:
	mkdir -p $(HOME)/build/
	go run main.go --config config.yaml
