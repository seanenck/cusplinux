goflags := "-trimpath -buildmode=pie -mod=readonly -modcacherw -buildvcs=false"
objects := "target"

default: (build "obu") (build "alpine-image-builder")

build target:
  @echo 'Building {{target}}...'
  mkdir -p {{objects}}
  go build {{goflags}} -o "{{objects}}/{{target}}" "cmd/{{target}}/"*.go

clean:
  rm -f "{{objects}}/"*
