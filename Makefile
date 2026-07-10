# Linux / macOS 构建脚本。Windows 用户请用 .\build.ps1
.PHONY: all engine frontend bundle build clean dev run package

OS := $(shell uname -s)
ifeq ($(OS),Darwin)
  RUST_TARGET := x86_64-apple-darwin
  LIB := libengine.dylib
else
  RUST_TARGET := x86_64-unknown-linux-gnu
  LIB := libengine.so
endif
BIN := build/bin

all: engine frontend bundle build

engine:
	cargo build --release --target $(RUST_TARGET) --manifest-path engine/Cargo.toml
	mkdir -p $(BIN)
	cp engine/target/$(RUST_TARGET)/release/*engine*.$(LIB:.so=so:.dylib=dylib) $(BIN)/ 2>/dev/null || true

frontend:
	cd frontend && npm install && npm run build

bundle:
	mkdir -p $(BIN)/frontend/dist
	cp -r frontend/dist/* $(BIN)/frontend/dist/

build: engine frontend bundle
	wails build -s

clean:
	cargo clean --manifest-path engine/Cargo.toml
	rm -rf frontend/dist frontend/node_modules $(BIN)

dev: engine
	cp engine/target/$(RUST_TARGET)/release/*engine* . 2>/dev/null || true
	wails dev

run: all
	$(BIN)/everevo

package: all
	cd build && tar -czf EverEvo-v0.1.0-$(OS).tar.gz bin/
