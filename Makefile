BINARY_NAME := cat2k
BIN_DIR := bin

.PHONY: build run clean

build:
	@mkdir -p $(BIN_DIR)
	go build -o $(BIN_DIR)/$(BINARY_NAME)

run: build
	$(BIN_DIR)/$(BINARY_NAME) run

clean:
	rm -rf $(BIN_DIR)
