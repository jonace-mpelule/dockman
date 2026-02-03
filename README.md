# 🐳 Dockman

Dockman is a lightweight Go utility designed to streamline your Docker workflow by automatically injecting environment variables from an `.env` file into your `docker run` commands. It also handles automatic port mapping based on your environment configuration.

## ✨ Features

- **Env Injection**: Automatically parses `.env` files and passes them to Docker using `-e` flags.
- **Port Mapping**: Automatically maps the `PORT` defined in your `.env` file to the container (e.g., `-p 3000:3000`).
- **Simple Syntax**: Wraps the standard Docker CLI, making it feel natural to use.
- **Lightweight**: Written in Go with zero external dependencies.

## 🚀 Installation

### Using the Install Script (macOS/Linux)
You can install Dockman quickly using the following command:

```bash
curl -sSL https://raw.githubusercontent.com/jonace-mpelule/dockman/v1.0.1/scripts/install.sh | sh
```

### From Source
If you have Go installed, you can build from source:

1. Clone the repository:
   ```bash
   git clone https://github.com/jonace-mpelule/dockman.git
   cd dockman
   ```
2. Build the binary:
   ```bash
   go build -o dockman main.go
   ```
3. Move to your path:
   ```bash
   sudo mv dockman /usr/local/bin/
   ```

## 📖 Usage

Dockman acts as a proxy for the `docker run` command.

### Basic Usage
By default, Dockman looks for a `.env` file in the current directory and injects its variables into the container.

```bash
dockman --tc="my-image"
```

### Specifying an Environment File
You can specify a custom environment file using the `--env=` flag:

```bash
dockman --env=".env.prod" --tc="my-image"
```

### How it works
If your `.env` file contains:
```env
PORT=8080
DATABASE_URL=postgres://localhost:5432
```

Running `dockman --tc="my-image"` is equivalent to running:
```bash
docker run -p 8080:8080 -e PORT=8080 -e DATABASE_URL=postgres://localhost:5432 my-image
```

Running  `dockman --tc"-p 5000:500 my-image"` will use your custom port mapping instead

## 🛠️ Requirements
- [Docker](https://docs.docker.com/get-docker/) must be installed and in your PATH.
- [Go](https://golang.org/doc/install) (only if building from source).

## 📄 License
MIT
