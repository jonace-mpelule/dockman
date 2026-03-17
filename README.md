# 🐳 Dockman

Dockman is a lightweight Go utility designed to streamline Docker run and build workflows with explicit separation between runtime environment injection and build-time inputs. It also handles automatic port mapping based on your environment configuration.

## ✨ Features

- **Env Injection**: Automatically parses `.env` files and passes them to Docker using `-e` flags.
- **Port Mapping**: Automatically maps the `PORT` defined in your `.env` file to the container (e.g., `-p 3000:3000`).
- **Simple Syntax**: Wraps the standard Docker CLI, making it feel natural to use.
- **Lightweight**: Written in Go with zero external dependencies.
- **Config-Driven**: Optional `dockman.json` for zero-arg runs.
- **Managed Lifecycle**: Start, stop, restart, and upgrade named apps from config.
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

To update an existing Dockman install to the latest published release, run:

```bash
dockman --update
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

### GitHub Actions CI/CD
The repository includes a GitHub Actions workflow that:

- runs tests on Blacksmith runners
- builds and publishes multi-arch container images to GHCR
- supports private Go modules during both `go test` and Docker image builds

Set these repository or organization secrets before enabling the workflow:

- `PRIVATE_GO_MODULES_TOKEN`: a fine-grained PAT with read access to the private dependency repositories
- `GOPRIVATE_PATTERN`: your Go private module pattern, for example `github.com/your-org/*`

The workflow publishes images to:

```bash
ghcr.io/<owner>/dockman:main
ghcr.io/<owner>/dockman:latest
ghcr.io/<owner>/dockman:v1.2.3
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

If you want Dockman to continue when the resolved env file does not exist, add:

```bash
dockman --allow-no-env --tc="my-image"
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

### Managed Lifecycle
Managed lifecycle commands use `run.name` from config and keep ownership of the container set for that app:

```bash
dockman start
dockman stop
dockman restart
```

If `run.zero_downtime` is enabled, `dockman restart` starts a new revision, waits for it to become healthy, then cuts traffic over through a managed proxy container.

### Zero-Downtime Upgrade
Use `dockman upgrade` in CI/CD when you want the server to pull a fresh image from Docker Hub, GHCR, or another registry supported by Docker, then replace the running app without downtime:

```bash
dockman upgrade
dockman upgrade --image="ghcr.io/example/my-app:sha-abcdef"
```

`dockman upgrade` always runs `docker pull` first and relies on the host Docker client for registry authentication.

### Updating Dockman Itself
`dockman --update` updates the Dockman binary itself by re-running the published install script from GitHub. This is separate from `dockman upgrade`, which updates your managed application container.

### Build Command
Build using config defaults:

```bash
dockman build
```

Override build options:

```bash
dockman build --tag="my-image" --context="." --file="Dockerfile" --target="builder" --platform="linux/amd64" --no-cache --pull
```

### Build Overrides
These flags override the matching values from `dockman.json` for a single build:

```bash
dockman build \
  --tag="my-image:local" \
  --context="." \
  --file="Dockerfile" \
  --target="builder" \
  --platform="linux/amd64" \
  --no-cache \
  --pull
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
dockman restart --dry-run
dockman upgrade --dry-run
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
    "args": "--rm",
    "name": "my-app",
    "zero_downtime": true,
    "readiness": "healthcheck"
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

### Build Inputs Explained
Dockman separates build-time inputs into four different buckets. This matters because they are not interchangeable.

1. `build.args`
Use this for normal Docker build arguments such as versions, commit SHAs, feature flags, or `NODE_ENV`.

```json
{
  "build": {
    "args": {
      "NODE_ENV": "production",
      "GIT_SHA": "${GIT_SHA}",
      "APP_VERSION": "${APP_VERSION}"
    }
  }
}
```

```bash
export GIT_SHA=$(git rev-parse --short HEAD)
export APP_VERSION=v1.2.3
dockman build
```

Dockerfile example:

```dockerfile
ARG NODE_ENV=development
ARG GIT_SHA=dev
ARG APP_VERSION=local

RUN echo "building $APP_VERSION from $GIT_SHA in $NODE_ENV mode"
```

Use `build.args` for non-secret values. Even if dry-run masks them, build args are still not the right place for tokens or passwords.

2. `build.env`
Use this for environment variables that the `docker build` process itself should see on the host side. These are not mounted into the image as BuildKit secrets.

```json
{
  "build": {
    "env": {
      "DOCKER_CLI_HINTS": "false",
      "BUILDKIT_PROGRESS": "plain"
    }
  }
}
```

This is useful when the Docker CLI or BuildKit behavior depends on environment variables.

3. `build.secrets`
Use this for actual secrets. The value itself should live outside `dockman.json`, either in a host environment variable or in a local file.

Environment variable secret:

```json
{
  "build": {
    "secrets": {
      "github_token": {
        "env": "GITHUB_TOKEN"
      }
    }
  }
}
```

```bash
export GITHUB_TOKEN=your_token_here
dockman build
```

File secret:

```json
{
  "build": {
    "secrets": {
      "npmrc": {
        "file": ".npmrc"
      }
    }
  }
}
```

```bash
dockman build
```

Dockerfile usage:

```dockerfile
RUN --mount=type=secret,id=github_token \
    sh -c 'test -f /run/secrets/github_token'

RUN --mount=type=secret,id=npmrc,target=/root/.npmrc \
    npm ci
```

Important rules:
- Do not put secret values directly inside `dockman.json`.
- Do not use `build.args` for passwords, tokens, or private keys.
- If you use `{ "env": "GITHUB_TOKEN" }`, the user must export `GITHUB_TOKEN` before running `dockman build`.
- If you use `{ "file": ".npmrc" }`, the file must exist locally before running `dockman build`.
- Dockman will fail before Docker starts if a required secret env var or file is missing.

4. `build.ssh`
Use this when the build needs SSH agent forwarding, for example when cloning a private Git repository during image build.

```json
{
  "build": {
    "ssh": ["default"]
  }
}
```

Dockerfile usage:

```dockerfile
RUN --mount=type=ssh git clone git@github.com:your-org/private-repo.git
```

This requires an SSH agent to already be running on the host.

### Full Build Example
This example shows most build features together:

```json
{
  "image": "my-app",
  "run": {
    "env_file": ".env",
    "env": {
      "APP_ENV": "development"
    },
    "auto_port": true,
    "args": "--rm",
    "name": "my-app",
    "zero_downtime": true,
    "readiness": "healthcheck"
  },
  "build": {
    "context": ".",
    "dockerfile": "Dockerfile",
    "tag": "my-app",
    "buildkit": true,
    "env": {
      "BUILDKIT_PROGRESS": "plain"
    },
    "args": {
      "NODE_ENV": "production",
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
    "cache_from": ["type=registry,ref=my-app:cache"],
    "cache_to": ["type=inline"],
    "pull": true,
    "no_cache": false,
    "extra_args": "--progress=plain"
  }
}
```

```bash
export GIT_SHA=$(git rev-parse --short HEAD)
export GITHUB_TOKEN=your_token_here
dockman build --dry-run
dockman build
```

### Runtime Env Versus Build Inputs
Runtime env and build inputs are separate on purpose:

- `run.env_file` and `run.env` are for `docker run`.
- `build.args` is for `docker build --build-arg`.
- `build.env` is for the host environment of the `docker build` process.
- `build.secrets` is for BuildKit-mounted secrets.
- `build.ssh` is for SSH forwarding during build.

If a value is only needed when the container starts, keep it under `run.*`. If it is only needed while the image is being built, keep it under `build.*`.

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
    "args": "--rm",
    "name": "my-app",
    "zero_downtime": false,
    "readiness": "healthcheck"
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
- `run.name` is required for `dockman start`, `stop`, `restart`, and `upgrade`.
- `run.zero_downtime` enables managed blue/green replacement through a Dockman proxy container.
- `run.readiness` is currently `healthcheck` only, which means the image must define a Docker `HEALTHCHECK` for zero-downtime restart or upgrade.
- Build defaults are used by `dockman build` unless overridden.
- `build.args` is for non-secret Docker build arguments.
- `build.secrets` is for sensitive values used during image build.
- `build.env` is for environment variables on the host side of `docker build`.
- `build.ssh` forwards SSH agent access into the build.
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
