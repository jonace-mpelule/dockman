# 🐳 Dockman

Dockman is a lightweight Go utility designed to streamline your Docker workflow by automatically injecting environment variables from an `.env` file into your `docker run` commands. It also handles automatic port mapping based on your environment configuration.

## ✨ Features

- **Env Injection**: Automatically parses `.env` files and passes them to Docker using `-e` flags.
- **Port Mapping**: Automatically maps the `PORT` defined in your `.env` file to the container (e.g., `-p 3000:3000`).
- **Simple Syntax**: Wraps the standard Docker CLI, making it feel natural to use.
- **Lightweight**: Written in Go with zero external dependencies.
- **Config-Driven**: Optional `dockman.json` for zero-arg runs.
- **Build Support**: Simple build command with configurable defaults.
- **Profiles**: Switch configs and env files with `--profile=dev`.
- **Dry Runs**: Print Docker commands without executing.

## 🚀 Installation

### Using the Install Script (macOS/Linux)
You can install Dockman quickly using the following command:

```bash
curl -sSL https://raw.githubusercontent.com/jonace-mpelule/dockman/main/scripts/install.sh | sh
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

### Config-First Workflow (Zero-Arg Run)
Initialize a config file:

```bash
dockman init
```

This creates `dockman.json` (editable), then you can just run:

```bash
dockman
```

### Build Command
Build using config defaults:

```bash
dockman build
```

Override build options:

```bash
dockman build --tag="my-image" --context="." --file="Dockerfile" --args="--no-cache"
```

### Profiles
Use a profile to switch configuration and env file defaults:

```bash
dockman --profile=dev
dockman build --profile=dev
```

This looks for:
- `dockman.dev.json` as config
- `.env.dev` as default env file

### Dry Run
Print the docker command without executing:

```bash
dockman --dry-run
dockman build --dry-run
```

### Help and Version
```bash
dockman --help
dockman --version
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

Running  `dockman --tc="-p 5000:500 my-image"` will use your custom port mapping instead

## ⚙️ Configuration (`dockman.json`)
Dockman will use `dockman.json` automatically when you run `dockman` with no arguments.

```json
{
  "image": "my-image",
  "env_file": ".env",
  "auto_port": true,
  "run_args": "--rm",
  "build": {
    "context": ".",
    "dockerfile": "Dockerfile",
    "tag": "my-image",
    "args": ""
  }
}
```

Notes:
- `image` is the default image name (used for `dockman` or `dockman run`).
- `run_args` is appended before the image name. If you prefer, you can use `{image}` in `run_args` to place it manually.
- `auto_port` controls automatic `-p PORT:PORT` injection (based on `PORT` in `.env`).
- Build defaults are used by `dockman build` unless overridden.

You can also select a custom config path:

```bash
dockman --config="path/to/dockman.json"
```

You can disable automatic port mapping at runtime:

```bash
dockman --no-port
```

## 🛠️ Requirements
- [Docker](https://docs.docker.com/get-docker/) must be installed and in your PATH.
- [Go](https://golang.org/doc/install) (only if building from source).

## 📄 License
MIT
