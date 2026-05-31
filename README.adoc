# ⬡ LegendaryOS Builder

> Fedora-based OS image builder — bootc compatible, inspired by Debian live-build

Build custom Fedora 44+ OS images as bootable ISOs or bootc container images, with a clean project layout, GitHub Actions integration, and Anaconda installer support.

---

## Installation

```bash
# From GitHub Releases
curl -fsSL https://github.com/legendaryos/builder/releases/latest/download/legendaryos-linux-amd64.tar.gz \
  | tar -xz
sudo mv legendaryos /usr/local/bin/legendaryos

# From source
git clone https://github.com/legendaryos/builder
cd builder
make install
```

---

## Quick Start

```bash
# Create a new project interactively
legendaryos init --fast

# Or non-interactively
legendaryos init ./my-distro

# Validate config
legendaryos validate

# Build container image (bootc)
legendaryos build cloud

# Build bootable ISO from the container
legendaryos build iso
```

---

## Project Layout

```
my-distro/
├── config.toml            ← main configuration (project, system, desktop, anaconda...)
├── install.packages       ← packages to install (one per line, # comments)
├── remove.packages        ← packages to remove
├── packages/              ← local .rpm files to install before package list
├── files/
│   ├── before/            ← filesystem overlay applied BEFORE package install
│   │   └── etc/
│   └── after/             ← filesystem overlay applied AFTER package install
│       └── etc/
├── scripts/               ← hooks: .sh .py .pl .rb — run in order
├── repos/                 ← custom .repo files (DNF repositories)
├── .github/
│   └── workflows/
│       └── build-cloud.yml   ← GitHub Actions CI (auto-generated)
└── build/
    ├── cache/             ← build cache
    └── output/            ← final .iso / container image artifacts
```

---

## Commands

| Command | Description |
|---|---|
| `legendaryos init` | Initialize project scaffold |
| `legendaryos init --fast` | Interactive wizard |
| `legendaryos build cloud` | Build bootc container image |
| `legendaryos build cloud --push` | Build and push to registry |
| `legendaryos build cloud --no-cache` | Fresh build (no layer cache) |
| `legendaryos build iso` | Build bootable ISO from container |
| `legendaryos build iso --source ghcr.io/…` | ISO from remote image |
| `legendaryos validate` | Validate project without building |
| `legendaryos info` | Show project info and package lists |
| `legendaryos clean` | Remove build outputs |
| `legendaryos clean --all` | Remove entire build/ directory |
| `legendaryos setup` | Install build dependencies (Fedora host) |
| `legendaryos version` | Show builder version |

Global flags: `--verbose` / `-v`, `--release`

---

## config.toml Reference

```toml
[project]
name         = "MyOS"
version      = "0.1.0"
base_distro  = "fedora"
base_version = 44
arch         = "x86_64"

[system]
hostname   = "myos"
locale     = "en_US.UTF-8"
timezone   = "Europe/Warsaw"
selinux    = "enforcing"
firewall   = true
services_enable  = ["sshd"]

[desktop]
environment    = "gnome"   # gnome | kde | xfce | cinnamon | none
display_server = "wayland"

[anaconda]
enabled        = true
webui          = true
product_name   = "MyOS"
kickstart_embed = true

[container]
enabled  = true
registry = "ghcr.io/myorg"
image    = "myos"
tag      = "latest"
push     = false
bootc_mode = true
```

---

## install.packages / remove.packages

Plain text, one package per line, comments with `#`:

```
# install.packages
bash-completion
vim-enhanced
@gnome-desktop-environment   # group install with @
firefox
```

---

## scripts/ Hooks

Scripts run in alphabetical order. Supported: `.sh`, `.py`, `.pl`, `.rb`

Environment variables available inside scripts:
- `LEGENDARYOS_ROOTFS` — path to the rootfs
- `LEGENDARYOS_BUILD` — build directory
- `LEGENDARYOS_PROJECT` — project name
- `LEGENDARYOS_VERSION` — project version

---

## GitHub Actions

`legendaryos init` generates `.github/workflows/build-cloud.yml` automatically.

The workflow:
1. Downloads the `legendaryos` binary from GitHub Releases
2. Validates the project
3. Builds the bootc container image with `legendaryos build cloud --push`
4. Signs the image with cosign (keyless)
5. Posts a build summary

Push a tag to trigger a release: `git tag v0.1.0 && git push --tags`

---

## How `build cloud` works

```
config.toml + install.packages + files/ + scripts/ + repos/
        ↓
  Containerfile (generated in build/)
        ↓
  podman build → OCI image
        ↓
  podman push → ghcr.io (optional)
        ↓
  cosign sign (optional)
```

## How `build iso` works

```
bootc container image (local or remote registry)
        ↓
  bootc-image-builder build --type iso
        ↓
  bootable hybrid ISO (El Torito + hybrid MBR)
        ↓
  build/output/<name>.iso
```

---

## Requirements

- **Build host**: Fedora 40+ (for `legendaryos setup`)
- **Tools**: `podman`, `buildah`, `bootc-image-builder`
- **For ISO**: root or `newuidmap` permissions for `bootc-image-builder`
- **For signing**: `cosign`

Install everything: `sudo legendaryos setup`
