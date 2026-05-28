package config

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

// Config is the full in-memory representation of config.toml
type Config struct {
	Project   ProjectConfig
	System    SystemConfig
	Desktop   DesktopConfig
	Boot      BootConfig
	Anaconda  AnacondaConfig
	Build     BuildConfig
	Container ContainerConfig
	Nvidia    NvidiaConfig
	Repos     []RepoConfig
	Branches  []BranchConfig
}

type ProjectConfig struct {
	Name        string
	Version     string
	Description string
	Author      string
	License     string
	BaseDistro  string
	BaseVersion int
	Arch        string
}

type SystemConfig struct {
	Hostname string
	Locale   string
	Timezone string
	Keyboard string
	Language string
	SELinux  string
	Firewall bool
	Services []string
	Disable  []string
}

type DesktopConfig struct {
	Environment   string
	DisplayServer string
	AutoLogin     bool
	AutoLoginUser string
}

type BootConfig struct {
	Bootloader string
	KernelArgs string
	Splash     bool
	Timeout    int
}

type AnacondaConfig struct {
	Enabled          bool
	KickstartEmbed   bool
	ProductName      string
	ProductVersion   string
	WebUI            bool
	HideShell        bool
	CustomLogo       string
	CustomBackground string
	DefaultLang      string
	DefaultKeyboard  string
	DefaultTimezone  string
	RootPasswordLock bool
	UserName         string
	UserGroups       []string
}

type BuildConfig struct {
	OutputDir   string
	CacheDir    string
	TmpDir      string
	Compression string
	ISOLabel    string
	ISOFilename string
	Jobs        int
	CleanBuild  bool
	// Filesystem type for the installed system partition
	// Supported by BIB: ext4, xfs, btrfs
	// Default: ext4 (most compatible, stable)
	Filesystem  string
}

type ContainerConfig struct {
	Enabled   bool
	// Registry modes:
	//   "custom" → ghcr.io/<user.toml name>/<image>
	//   "repo"   → ghcr.io/<container.repo>  (specific GitHub repo)
	//   other    → used as-is
	Registry  string
	Image     string
	Tag       string
	Push      bool
	SignImage  bool
	BootcMode bool
	// Repo: specific GitHub org/repo, used when Registry = "repo"
	// e.g. "LegendaryOS-Linux-System/legendaryos"
	Repo      string
}

// BranchConfig defines an OS variant with a different desktop environment.
// Branches share the base image but layer a different DE on top.
type BranchConfig struct {
	Name           string   // branch id: "gnome", "xfce", "kde"
	Desktop        string   // DE: "gnome", "xfce", etc.
	DisplayName    string   // "LegendaryOS GNOME Edition"
	Tag            string   // OCI tag, auto-generated if empty
	Description    string
	ExtraPackages  []string // extra packages on top of base
	RemovePackages []string // packages to strip from base
	Enabled        bool
}

// NvidiaConfig controls proprietary NVIDIA driver installation in the ISO.
// Drivers are installed via %post in the Anaconda kickstart using akmod-nvidia
// from RPM Fusion. They are NOT installed in the bootc container image.
type NvidiaConfig struct {
	// Enabled: install NVIDIA proprietary drivers during ISO installation
	Enabled bool
	// InstallCUDA: also install CUDA toolkit (large, optional)
	InstallCUDA bool
	// InstallNVIDIASettings: install nvidia-settings GUI tool
	InstallNVIDIASettings bool
	// InstallVAAPI: install nvidia-vaapi-driver for hardware video decode
	InstallVAAPI bool
	// InstallVulkan: install nvidia Vulkan ICD (usually included in driver)
	InstallVulkan bool
	// Blacklist nouveau: prevent open-source nouveau from loading
	BlacklistNouveau bool
	// KMS: enable kernel mode setting for NVIDIA (required for Wayland)
	EnableKMS bool
	// OpenDriver: use nvidia-open (open-source kernel module, Turing+ only)
	// Set to false for Maxwell/Pascal/Volta or if unsure
	OpenDriver bool
}

type RepoConfig struct {
	Name     string
	Enabled  bool
	BaseURL  string
	MetaLink string
	GPGCheck bool
	GPGKey   string
	Priority int
}

// Paths holds all well-known project directory paths
type Paths struct {
	Root          string
	Config        string
	PackagesDir       string
	InstallPkgs       string
	RemovePkgs        string
	FlatpakPkgs       string
	FlatpakRemovePkgs string
	FilesDir      string
	FilesAfter    string
	FilesBefore   string
	ScriptsDir    string
	ScriptsPre    string // run on HOST before anything else
	ScriptsBefore string // run inside container BEFORE dnf install
	ScriptsAfter  string // run inside container AFTER dnf install
	ReposDir      string
	BuildDir      string
	CacheDir      string
	OutputDir     string
	PodmanStorage string // isolated podman storage inside build/
}

func GetPaths(root string) *Paths {
	return &Paths{
		Root:          root,
		Config:        filepath.Join(root, "config.toml"),
		PackagesDir:       filepath.Join(root, "packages"),
		InstallPkgs:       filepath.Join(root, "packages", "install.packages"),
		RemovePkgs:        filepath.Join(root, "packages", "remove.packages"),
		FlatpakPkgs:       filepath.Join(root, "packages", "flatpak.packages"),
		FlatpakRemovePkgs: filepath.Join(root, "packages", "flatpak.remove.packages"),
		FilesDir:      filepath.Join(root, "files"),
		FilesAfter:    filepath.Join(root, "files", "after"),
		FilesBefore:   filepath.Join(root, "files", "before"),
		ScriptsDir:    filepath.Join(root, "scripts"),
		ScriptsPre:    filepath.Join(root, "scripts", "pre"),
		ScriptsBefore: filepath.Join(root, "scripts", "before"),
		ScriptsAfter:  filepath.Join(root, "scripts", "after"),
		ReposDir:      filepath.Join(root, "repos"),
		BuildDir:      filepath.Join(root, "build"),
		CacheDir:      filepath.Join(root, "build", "cache"),
		OutputDir:     filepath.Join(root, "build", "output"),
		PodmanStorage: filepath.Join(root, "build", "podman-storage"),
	}
}

// Load parses config.toml using a hand-rolled TOML parser (stdlib only)
func Load(root string) (*Config, error) {
	paths := GetPaths(root)
	if _, err := os.Stat(paths.Config); os.IsNotExist(err) {
		return nil, fmt.Errorf("config.toml not found in %s\n  Run: legendaryos init", root)
	}
	f, err := os.Open(paths.Config)
	if err != nil {
		return nil, fmt.Errorf("cannot open config.toml: %w", err)
	}
	defer f.Close()

	cfg := &Config{}
	cfg.applyDefaults()

	section := ""
	var repoInProgress   *RepoConfig
	var branchInProgress *BranchConfig
	scanner := bufio.NewScanner(f)
	lineNum := 0

	for scanner.Scan() {
		lineNum++
		line := strings.TrimSpace(scanner.Text())

		// skip blank lines and comments
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}

		// section header
		if strings.HasPrefix(line, "[[") && strings.HasSuffix(line, "]]") {
			name := strings.TrimSuffix(strings.TrimPrefix(line, "[["), "]]")
			if repoInProgress != nil {
				cfg.Repos = append(cfg.Repos, *repoInProgress)
			}
			if name == "repo" {
				r := &RepoConfig{Enabled: true, GPGCheck: true, Priority: 99}
				repoInProgress = r
			}
			section = name
			continue
		}
		if strings.HasPrefix(line, "[") && strings.HasSuffix(line, "]") {
			if repoInProgress != nil {
				cfg.Repos = append(cfg.Repos, *repoInProgress)
				repoInProgress = nil
			}
			section = strings.TrimSuffix(strings.TrimPrefix(line, "["), "]")
			continue
		}

		// key = value
		eq := strings.Index(line, "=")
		if eq < 0 {
			continue
		}
		key := strings.TrimSpace(line[:eq])
		val := strings.TrimSpace(line[eq+1:])
		// strip inline comment
		if ci := strings.Index(val, " #"); ci >= 0 {
			val = strings.TrimSpace(val[:ci])
		}

		switch section {
		case "project":
			setProjectField(&cfg.Project, key, val)
		case "system":
			setSystemField(&cfg.System, key, val)
		case "desktop":
			setDesktopField(&cfg.Desktop, key, val)
		case "boot":
			setBootField(&cfg.Boot, key, val)
		case "anaconda":
			setAnacondaField(&cfg.Anaconda, key, val)
		case "build":
			setBuildField(&cfg.Build, key, val)
		case "container":
			setContainerField(&cfg.Container, key, val)
		case "nvidia":
			setNvidiaField(&cfg.Nvidia, key, val)
		case "repo":
			if repoInProgress != nil {
				setRepoField(repoInProgress, key, val)
			}
		case "branch":
			if branchInProgress != nil {
				setBranchField(branchInProgress, key, val)
			}
		}
	}
	if repoInProgress != nil {
		cfg.Repos = append(cfg.Repos, *repoInProgress)
	}
	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("error reading config.toml: %w", err)
	}
	// Auto-detect license from LICENSE file if not specified in config
	if cfg.Project.License == "" {
		cfg.Project.License = DetectLicense(root)
	}
	return cfg, nil
}

// ── Field setters ─────────────────────────────────────────────────────────────

func str(v string) string {
	v = strings.Trim(v, `"'`)
	return v
}

func boolean(v string) bool {
	v = strings.ToLower(strings.Trim(v, `"'`))
	return v == "true" || v == "yes" || v == "1"
}

func integer(v string) int {
	v = strings.Trim(v, `"'`)
	n, _ := strconv.Atoi(v)
	return n
}

func strSlice(v string) []string {
	// supports: ["a", "b"] or ["a","b"]
	v = strings.Trim(v, "[]")
	if v == "" {
		return nil
	}
	parts := strings.Split(v, ",")
	var out []string
	for _, p := range parts {
		p = strings.TrimSpace(strings.Trim(p, `"' `))
		if p != "" {
			out = append(out, p)
		}
	}
	return out
}

func setProjectField(p *ProjectConfig, k, v string) {
	switch k {
	case "name":
		p.Name = str(v)
	case "version":
		p.Version = str(v)
	case "description":
		p.Description = str(v)
	case "author":
		p.Author = str(v)
	case "license":
		p.License = str(v)
	case "base_distro":
		p.BaseDistro = str(v)
	case "base_version":
		p.BaseVersion = integer(v)
	case "arch":
		p.Arch = str(v)
	}
}

func setSystemField(s *SystemConfig, k, v string) {
	switch k {
	case "hostname":
		s.Hostname = str(v)
	case "locale":
		s.Locale = str(v)
	case "timezone":
		s.Timezone = str(v)
	case "keyboard":
		s.Keyboard = str(v)
	case "language":
		s.Language = str(v)
	case "selinux":
		s.SELinux = str(v)
	case "firewall":
		s.Firewall = boolean(v)
	case "services_enable":
		s.Services = strSlice(v)
	case "services_disable":
		s.Disable = strSlice(v)
	}
}

func setDesktopField(d *DesktopConfig, k, v string) {
	switch k {
	case "environment":
		d.Environment = str(v)
	case "display_server":
		d.DisplayServer = str(v)
	case "auto_login":
		d.AutoLogin = boolean(v)
	case "auto_login_user":
		d.AutoLoginUser = str(v)
	}
}

func setBootField(b *BootConfig, k, v string) {
	switch k {
	case "bootloader":
		b.Bootloader = str(v)
	case "kernel_args":
		b.KernelArgs = str(v)
	case "splash":
		b.Splash = boolean(v)
	case "timeout":
		b.Timeout = integer(v)
	}
}

func setAnacondaField(a *AnacondaConfig, k, v string) {
	switch k {
	case "enabled":
		a.Enabled = boolean(v)
	case "kickstart_embed":
		a.KickstartEmbed = boolean(v)
	case "product_name":
		a.ProductName = str(v)
	case "product_version":
		a.ProductVersion = str(v)
	case "webui":
		a.WebUI = boolean(v)
	case "hide_shell":
		a.HideShell = boolean(v)
	case "custom_logo":
		a.CustomLogo = str(v)
	case "custom_background":
		a.CustomBackground = str(v)
	case "default_lang":
		a.DefaultLang = str(v)
	case "default_keyboard":
		a.DefaultKeyboard = str(v)
	case "default_timezone":
		a.DefaultTimezone = str(v)
	case "root_password_lock":
		a.RootPasswordLock = boolean(v)
	case "default_user":
		a.UserName = str(v)
	case "default_user_groups":
		a.UserGroups = strSlice(v)
	}
}

func setBuildField(b *BuildConfig, k, v string) {
	switch k {
	case "output_dir":
		b.OutputDir = str(v)
	case "cache_dir":
		b.CacheDir = str(v)
	case "tmp_dir":
		b.TmpDir = str(v)
	case "compression":
		b.Compression = str(v)
	case "iso_label":
		b.ISOLabel = str(v)
	case "iso_filename":
		b.ISOFilename = str(v)
	case "jobs":
		b.Jobs = integer(v)
	case "clean_build":
		b.CleanBuild = boolean(v)
	case "filesystem":
		b.Filesystem = str(v)
	}
}

func setContainerField(ct *ContainerConfig, k, v string) {
	switch k {
	case "enabled":
		ct.Enabled = boolean(v)
	case "registry":
		ct.Registry = str(v)
	case "image":
		ct.Image = str(v)
	case "tag":
		ct.Tag = str(v)
	case "push":
		ct.Push = boolean(v)
	case "sign_image":
		ct.SignImage = boolean(v)
	case "bootc_mode":
		ct.BootcMode = boolean(v)
	case "repo":
		ct.Repo = str(v)
	}
}

func setNvidiaField(n *NvidiaConfig, k, v string) {
	switch k {
	case "enabled":
		n.Enabled = boolean(v)
	case "install_cuda":
		n.InstallCUDA = boolean(v)
	case "install_nvidia_settings":
		n.InstallNVIDIASettings = boolean(v)
	case "install_vaapi":
		n.InstallVAAPI = boolean(v)
	case "install_vulkan":
		n.InstallVulkan = boolean(v)
	case "blacklist_nouveau":
		n.BlacklistNouveau = boolean(v)
	case "enable_kms":
		n.EnableKMS = boolean(v)
	case "open_driver":
		n.OpenDriver = boolean(v)
	}
}


func setBranchField(b *BranchConfig, k, v string) {
	switch k {
	case "name":
		b.Name = str(v)
	case "desktop":
		b.Desktop = str(v)
	case "display_name":
		b.DisplayName = str(v)
	case "tag":
		b.Tag = str(v)
	case "description":
		b.Description = str(v)
	case "extra_packages":
		b.ExtraPackages = strSlice(v)
	case "remove_packages":
		b.RemovePackages = strSlice(v)
	case "enabled":
		b.Enabled = boolean(v)
	}
}

func setRepoField(r *RepoConfig, k, v string) {
	switch k {
	case "enabled":
		r.Enabled = boolean(v)
	case "baseurl":
		r.BaseURL = str(v)
	case "metalink":
		r.MetaLink = str(v)
	case "gpgcheck":
		r.GPGCheck = boolean(v)
	case "gpgkey":
		r.GPGKey = str(v)
	case "priority":
		r.Priority = integer(v)
	}
}

func (c *Config) applyDefaults() {
	c.Project.BaseDistro = "fedora"
	c.Project.BaseVersion = 44
	c.Project.Arch = "x86_64"
	c.Build.OutputDir = "build/output"
	c.Build.CacheDir = "build/cache"
	c.Build.Compression = "xz"
	c.Build.Jobs = 4
	c.Build.Filesystem = "ext4"
	c.Boot.Bootloader = "grub2"
	c.Boot.Timeout = 5
	c.System.SELinux = "enforcing"
	// NVIDIA defaults — sensible for a gaming/dev distro
	c.Nvidia.InstallNVIDIASettings = true
	c.Nvidia.InstallVAAPI = true
	c.Nvidia.InstallVulkan = true
	c.Nvidia.BlacklistNouveau = true
	c.Nvidia.EnableKMS = true
	c.Nvidia.OpenDriver = false // safe default — works on all cards
}

// ReadPackageList reads install.packages or remove.packages
func ReadPackageList(path string) ([]string, error) {
	f, err := os.Open(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	defer f.Close()

	var pkgs []string
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := scanner.Text()
		if i := strings.Index(line, "#"); i >= 0 {
			line = line[:i]
		}
		line = strings.TrimSpace(line)
		if line != "" {
			pkgs = append(pkgs, line)
		}
	}
	return pkgs, scanner.Err()
}

// DetectLicense reads the LICENSE file in root and guesses the SPDX identifier.
func DetectLicense(root string) string {
	data, err := os.ReadFile(filepath.Join(root, "LICENSE"))
	if err != nil {
		// try LICENSE.md, LICENSE.txt
		for _, name := range []string{"LICENSE.md", "LICENSE.txt", "COPYING"} {
			if d, err2 := os.ReadFile(filepath.Join(root, name)); err2 == nil {
				data = d
				break
			}
		}
		if data == nil {
			return ""
		}
	}
	text := strings.ToLower(string(data))
	switch {
	case strings.Contains(text, "mit license") || strings.Contains(text, "permission is hereby granted, free of charge"):
		return "MIT"
	case strings.Contains(text, "apache license") && strings.Contains(text, "version 2"):
		return "Apache-2.0"
	case strings.Contains(text, "gnu general public license") && strings.Contains(text, "version 3"):
		return "GPL-3.0"
	case strings.Contains(text, "gnu general public license") && strings.Contains(text, "version 2"):
		return "GPL-2.0"
	case strings.Contains(text, "gnu lesser general public license") && strings.Contains(text, "version 3"):
		return "LGPL-3.0"
	case strings.Contains(text, "gnu lesser general public license") && strings.Contains(text, "version 2"):
		return "LGPL-2.1"
	case strings.Contains(text, "mozilla public license") && strings.Contains(text, "2.0"):
		return "MPL-2.0"
	case strings.Contains(text, "bsd 2-clause") || (strings.Contains(text, "redistribution") && strings.Contains(text, "2 conditions")):
		return "BSD-2-Clause"
	case strings.Contains(text, "bsd 3-clause") || (strings.Contains(text, "redistribution") && strings.Contains(text, "3 conditions")):
		return "BSD-3-Clause"
	case strings.Contains(text, "isc license") || strings.Contains(text, "isc"):
		return "ISC"
	case strings.Contains(text, "unlicense") || strings.Contains(text, "public domain"):
		return "Unlicense"
	case strings.Contains(text, "creative commons") && strings.Contains(text, "4.0"):
		return "CC-BY-4.0"
	default:
		return "UNKNOWN"
	}
}
