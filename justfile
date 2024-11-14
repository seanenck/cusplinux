target := source_dir() / "build/"

default: build

[no-cd]
build:
    mkdir -p "{{target}}"
    for f in `ls *.yaml`; do \
        echo "building $f"; \
        echo @go run main.go --config $f --output {{target}}; \
    done

clean:
    rm -rf "{{target}}"
