# 🐳 Dockman

Dockman is a lightweight Go utility designed to streamline Docker run and build workflows with explicit separation between runtime environment injection and build-time inputs. It also handles automatic port mapping based on your environment configuration.

## ✨ Features

- **Env Injection**: Automatically parses `.env` files and passes them to Docker using `-e` flags.
- **Port Mapping**: Automatically maps the `PORT` defined in your `.env` file to the container (e.g., `-p 3000:3000`).
- **Simple Syntax**: Wraps the standard Docker CLI, making it feel natural to use.
- **Lightweight**: Written in Go with zero external dependencies.
- **Config-Driven**: Optional `dockman.json` for zero-arg runs.
- **Structured Build Support**: Build args, BuildKit secrets, SSH forwarding, cache flags, target/platform selection, and raw pass-through args.
- **Profiles**: Switch configs and env files with `--profile=dev`.
- **Dry Runs**: Print Docker commands without executing.
- **Secret-Safe Previews**: Dry-run output redacts env and build-arg values.

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
dockman build --tag="my-image" --context="." --file="Dockerfile" --target="builder" --platform="linux/amd64" --no-cache --pull
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
dockman doctor --dry-run
```

### Config Sanity Check And Migration
After upgrading Dockman, run:

```bash
dockman doctor
```

This will:
- load your config
- validate it against the current schema
- migrate legacy fields to the current `run.*` and `build.*` layout
- write a backup to `dockman.json.bak` before updating the file

If you only want to check whether a rewrite is needed, use:

```bash
dockman doctor --dry-run
```

### BuildKit Features
BuildKit is enabled by default for `dockman build`, so Dockerfiles that use secret mounts or SSH forwarding work without extra setup.

Example config:

```json
{
  "image": "my-image",
  "run": {
    "env_file": ".env",
    "env": {
      "APP_ENV": "development"
    },
    "auto_port": true,
    "args": "--rm"
  },
  "build": {
    "context": ".",
    "dockerfile": "Dockerfile",
    "tag": "my-image",
    "buildkit": true,
    "args": {
      "GIT_SHA": "${GIT_SHA}"
    },
    "secrets": {
      "github_token": {
        "env": "GITHUB_TOKEN"
      },
      "npmrc": {
        "file": ".npmrc"
      }
    },
    "ssh": ["default"],
    "target": "builder",
    "platform": "linux/amd64",
    "cache_from": ["type=registry,ref=my-image:cache"],
    "cache_to": ["type=inline"],
    "pull": true,
    "extra_args": "--progress=plain"
  }
}
```

Notes:
- `build.args` becomes `docker build --build-arg`.
- `build.env` sets environment variables for the `docker build` process only.
- `build.secrets` supports either `{ "env": "HOST_ENV_NAME" }` or `{ "file": "path/to/file" }`.
- `${VAR}` interpolation is resolved from the host environment and fails before Docker runs if a referenced variable is missing.
- `run.env` and `run.env_file` are kept separate from build inputs.

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
  "run": {
    "env_file": ".env",
    "env": {},
    "auto_port": true,
    "args": "--rm"
  },
  "build": {
    "context": ".",
    "dockerfile": "Dockerfile",
    "tag": "my-image",
    "buildkit": true
  }
}
```

Notes:
- `image` is the default image name (used for `dockman` or `dockman run`).
- `run.args` is appended before the image name. If you prefer, you can use `{image}` in `run.args` to place it manually.
- `run.auto_port` controls automatic `-p PORT:PORT` injection (based on `PORT` in the resolved runtime env).
- Build defaults are used by `dockman build` unless overridden.
- `schema_version` is written automatically and used by `dockman doctor` to keep configs on the current spec.
- Older top-level fields such as `env_file`, `auto_port`, `run_args`, and string-valued `build.args` are still accepted for compatibility.

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
