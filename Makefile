# Укажите здесь нужную версию Go
GO_VERSION=1.21

# Имя исполняемого файла
BINARY_NAME=gophermart

# Путь к приложению
APP_PATH=./cmd/gophermart

# Docker образ
DOCKER_IMAGE=debian:bookworm-slim

.PHONY: all build run clean docker-run

all: build

# Команда сборки Go приложения
build:
	@echo "Building $(BINARY_NAME)..."
	go build -o $(BINARY_NAME) $(APP_PATH)

# Команда для локального запуска
run: build
	@echo "Running $(BINARY_NAME)..."
	./$(BINARY_NAME)

# Очистка скомпилированных файлов
clean:
	@echo "Cleaning up..."
	rm -f $(BINARY_NAME)

# Запуск в Docker-контейнере
docker-run: build
	@echo "Running $(BINARY_NAME) in Docker..."
	sudo docker run --rm -v $(CURDIR):/app -w /app $(DOCKER_IMAGE) ./$(BINARY_NAME)