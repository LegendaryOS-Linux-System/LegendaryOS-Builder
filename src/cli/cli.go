package cli

import (
	"flag"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/legendaryos/builder/src/builder"
	"github.com/legendaryos/builder/src/config"
	"github.com/legendaryos/builder/src/ui"
)

var (
	Version   = "dev"
	Commit    = "unknown"
	BuildDate = "unknown"
)

func Execute() {
	if len(os.Args) < 2 {
		ui.PrintUsage(Version)
		os.Exit(0)
	}

	cmd := os.Args[1]
	rest := os.Args[2:]

	switch cmd {
		case "settings":
			cmdSettings(rest)
		case "build":
			cmdBuild(rest)
		case "init":
			cmdInit(rest)
		case "clean":
			cmdClean(rest)
		case "setup":
			cmdSetup(rest)
		case "validate":
			cmdValidate(rest)
		case "info":
			cmdInfo(rest)
		case "version", "--version":
			cmdVersion()
		case "help", "--help", "-h":
			ui.PrintUsage(Version)
		default:
			ui.SmallBanner()
			ui.Error("Unknown command: %q", cmd)
			ui.Newline()
			ui.PrintUsage(Version)
			os.Exit(1)
	}
}

// ── namedStep ─────────────────────────────────────────────────────────────────

type namedStep struct {
	name string
	fn   func() error
}

func runSteps(steps []namedStep, cfg *config.Config, start time.Time, imgTag, isoPath string) {
	bar := ui.NewProgressBar(len(steps), "building")
	var summary []ui.SummaryStep

	for i, s := range steps {
		bar.Set(i)
		if err := s.fn(); err != nil {
			bar.Done()
			summary = append(summary, ui.SummaryStep{Name: s.name, Status: "fail", Detail: err.Error()})
			ui.PrintBuildSummary(&ui.BuildSummary{
				ProjectName: cfg.Project.Name,
				Version:     cfg.Project.Version,
				Distro:      fmt.Sprintf("Fedora %d", cfg.Project.BaseVersion),
					     Steps:       summary,
					     Duration:    time.Since(start),
			})
			ui.Fatal("%v", err)
		}
		summary = append(summary, ui.SummaryStep{Name: s.name, Status: "ok"})
	}
	bar.Done()

	ui.PrintBuildSummary(&ui.BuildSummary{
		ProjectName: cfg.Project.Name,
		Version:     cfg.Project.Version,
		Distro:      fmt.Sprintf("Fedora %d", cfg.Project.BaseVersion),
			     Steps:       summary,
			     ImageTag:    imgTag,
			     ISOPath:     isoPath,
			     Duration:    time.Since(start),
	})
}

// ── build ─────────────────────────────────────────────────────────────────────

// cmdBuild dispatches build subcommands.
//
// Special invocation forms (no standard subcommand):
//
//	legendaryos-builder build
//	    → runs build.rb in the project directory (if it exists)
//
//	legendaryos-builder build ;;
//	    → shows all available tasks/commands defined in build.rb
//
//	legendaryos-builder build ''
//	    → shows available subcommands of build.rb
//
//	legendaryos-builder build -_ <file>
//	    → runs a custom build file instead of build.rb
func cmdBuild(args []string) {
	// ── special: -_ <file> — run a custom build file ──────────────────────
	if len(args) >= 2 && args[0] == "-_" {
		cmdBuildCustomFile(args[1], args[2:])
		return
	}

	// ── special: ;; — list all tasks in build.rb ──────────────────────────
	if len(args) == 1 && args[0] == ";;" {
		cmdBuildRubyList(cwd(), "build.rb", true)
		return
	}

	// ── special: '' (empty string) — list subcommands of build.rb ─────────
	if len(args) == 1 && args[0] == "" {
		cmdBuildRubyList(cwd(), "build.rb", false)
		return
	}

	// ── no args: run build.rb directly ────────────────────────────────────
	if len(args) == 0 {
		cmdBuildRubyDefault(cwd(), "build.rb")
		return
	}

	// ── standard subcommands ──────────────────────────────────────────────
	switch args[0] {
		case "cloud":
			cmdBuildCloud(args[1:])
		case "iso":
			cmdBuildISO(args[1:])
		case "--release", "release":
			cmdBuildRelease(args[1:])
		default:
			ui.SmallBanner()
			ui.Error("Unknown build target %q — use 'cloud', 'iso', '--release', or omit for build.rb", args[0])
			ui.Newline()
			fmt.Fprintln(ui.Out, "  Examples:")
			fmt.Fprintln(ui.Out, "    legendaryos build cloud")
			fmt.Fprintln(ui.Out, "    legendaryos build iso")
			fmt.Fprintln(ui.Out, "    legendaryos build            # runs build.rb")
			fmt.Fprintln(ui.Out, "    legendaryos build ;;         # list all tasks in build.rb")
			fmt.Fprintln(ui.Out, "    legendaryos build ''         # list subcommands in build.rb")
			fmt.Fprintln(ui.Out, "    legendaryos build -_ myscript.rb  # run custom build file")
			ui.Newline()
			os.Exit(1)
	}
}

// cmdBuildRubyDefault runs build.rb (or the nearest equivalent) with no extra args.
func cmdBuildRubyDefault(root, filename string) {
	ui.SmallBanner()
	path := filepath.Join(root, filename)
	if _, err := os.Stat(path); os.IsNotExist(err) {
		ui.Error("%s not found in %s", filename, root)
		ui.Info("Create a build.rb file next to config.toml, or use:")
		ui.Info("  legendaryos build cloud")
		ui.Info("  legendaryos build iso")
		os.Exit(1)
	}
	ui.Section(fmt.Sprintf("Running %s", filename))
	runRubyFile(path, nil)
}

// cmdBuildRubyList prints available tasks/subcommands from build.rb.
// When full=true it passes --tasks (Rake-style); otherwise --help.
func cmdBuildRubyList(root, filename string, full bool) {
	ui.SmallBanner()
	path := filepath.Join(root, filename)
	if _, err := os.Stat(path); os.IsNotExist(err) {
		ui.Error("%s not found in %s", filename, root)
		os.Exit(1)
	}
	var extraArgs []string
	if full {
		ui.Section(fmt.Sprintf("Tasks in %s", filename))
		extraArgs = []string{"--tasks"}
	} else {
		ui.Section(fmt.Sprintf("Subcommands in %s", filename))
		extraArgs = []string{"--help"}
	}
	runRubyFile(path, extraArgs)
}

// cmdBuildCustomFile runs a user-specified build file (not necessarily build.rb).
func cmdBuildCustomFile(filename string, extraArgs []string) {
	ui.SmallBanner()
	// Resolve relative to cwd
	if !filepath.IsAbs(filename) {
		filename = filepath.Join(cwd(), filename)
	}
	if _, err := os.Stat(filename); os.IsNotExist(err) {
		ui.Error("Build file not found: %s", filename)
		os.Exit(1)
	}
	ui.Section(fmt.Sprintf("Running custom build file: %s", filepath.Base(filename)))
	runRubyFile(filename, extraArgs)
}

// runRubyFile executes a Ruby script, passing extraArgs to it.
func runRubyFile(path string, extraArgs []string) {
	args := append([]string{path}, extraArgs...)
	cmd := exec.Command("ruby", args...)
	cmd.Stdin  = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	cmd.Dir    = filepath.Dir(path)
	cmd.Env    = os.Environ()
	if err := cmd.Run(); err != nil {
		ui.Fatal("build file failed: %v", err)
	}
}

// ── build cloud ───────────────────────────────────────────────────────────────

func cmdBuildCloud(args []string) {
	fs := flag.NewFlagSet("build cloud", flag.ExitOnError)
	verbose  := fs.Bool("verbose", false, "Show full podman output")
	vShort   := fs.Bool("v", false, "Show full podman output (short)")
	release  := fs.Bool("release", false, "Release mode: squash layers, strip debug")
	push     := fs.Bool("push", false, "Push image to registry after build")
	sign     := fs.Bool("sign", false, "Sign image with cosign after push")
	noCache  := fs.Bool("no-cache", false, "Disable podman layer cache (fresh build)")
	platform := fs.String("platform", "linux/amd64", "Target platform")
	fs.Parse(args) //nolint

	verb := *verbose || *vShort
	ui.SmallBanner()

	root  := cwd()
	start := time.Now()

	cfg, err := config.Load(root)
	if err != nil {
		ui.Fatal("%v", err)
	}
	uc, err := config.LoadUser(root)
	if err != nil {
		ui.Warn("Cannot read user.toml: %v", err)
		uc = &config.UserConfig{}
	}
	paths := config.GetPaths(root)
	tag   := imageTag(cfg, uc)

	token := askGitHubToken(uc)

	ui.Section("Cloud Build")
	ui.Info("Project  : %s %s", cfg.Project.Name, cfg.Project.Version)
	ui.Info("Mode     : %s", buildModeLabel(cfg))
	ui.Info("Base     : %s", cloudBaseImage(cfg))
	ui.Info("Image    : %s", tag)
	ui.Info("Platform : %s", *platform)
	ui.Info("Registry : %s", registryHost(cfg, uc))
	if cfg.Bootloader.Enabled {
		ui.Info("Bootloader: %s (custom)", cfg.ResolvedBootloader())
	}
	if *push    { ui.Info("Push     : yes") }
	if *release { ui.Info("Mode     : RELEASE") }
	ui.Newline()

	b := builder.NewCloud(cfg, paths, verb, *release)

	steps := []namedStep{
		{"Pre-scripts (host)",     func() error { return b.RunPreScripts() }},
		{"Validate project",       b.Validate},
		{"Prepare build dirs",     b.PrepareDirs},
		{"Registry login",         func() error { return b.RegistryLogin(registryHost(cfg, uc), uc.GitHub.Name, token) }},
		{"Copy repos",             b.CopyRepos},
		{"Generate Containerfile", func() error { return b.GenerateContainerfile(*platform) }},
		{"podman build",           func() error { return b.PodmanBuild(tag, *noCache) }},
	}
	if *push {
		steps = append(steps, namedStep{"podman push", func() error { return b.PodmanPush(tag) }})
	}
	if *sign && *push {
		steps = append(steps, namedStep{"cosign sign", func() error { return b.CosignSign(tag) }})
	}

	runSteps(steps, cfg, start, tag, "")
}

// ── build iso ─────────────────────────────────────────────────────────────────

func cmdBuildISO(args []string) {
	fs := flag.NewFlagSet("build iso", flag.ExitOnError)
	verbose   := fs.Bool("verbose", false, "Verbose output")
	vShort    := fs.Bool("v", false, "Verbose output (short)")
	release   := fs.Bool("release", false, "Release mode")
	source    := fs.String("source", "", "Source container image (default: from config.toml)")
	kickstart := fs.String("kickstart", "", "Custom kickstart file path")
	label     := fs.String("label", "", "ISO volume label")
	output    := fs.String("output", "", "Output ISO path")
	fs.Parse(args) //nolint

	verb := *verbose || *vShort
	ui.SmallBanner()

	root  := cwd()
	start := time.Now()

	storageCfgPath := filepath.Join(root, "build", "storage.conf")
	if _, err := os.Stat(storageCfgPath); err == nil {
		if removeErr := os.Remove(storageCfgPath); removeErr != nil {
			ui.Warn("Cannot remove old storage.conf: %v", removeErr)
			ui.Warn("Try: sudo rm %s", storageCfgPath)
		}
	}

	cfg, err := config.Load(root)
	if err != nil {
		ui.Fatal("%v", err)
	}
	uc, err := config.LoadUser(root)
	if err != nil {
		ui.Warn("Cannot read user.toml: %v", err)
		uc = &config.UserConfig{}
	}
	paths := config.GetPaths(root)

	srcImage := *source
	if srcImage == "" {
		srcImage = imageTag(cfg, uc)
	}

	outPath := *output
	if outPath == "" {
		name := strings.ReplaceAll(cfg.Project.Name, " ", "-")
		fn := cfg.Build.ISOFilename
		if fn == "" {
			fn = fmt.Sprintf("%s-%s-fedora%d.x86_64.iso",
					 name, cfg.Project.Version, cfg.Project.BaseVersion)
		}
		outPath = filepath.Join(paths.OutputDir, fn)
	}

	isoLabel := *label
	if isoLabel == "" {
		isoLabel = cfg.Build.ISOLabel
	}
	if isoLabel == "" {
		isoLabel = strings.ToUpper(strings.ReplaceAll(cfg.Project.Name, " ", "_"))
	}

	ksPath := *kickstart
	if ksPath == "" && cfg.Anaconda.Enabled {
		ksPath = filepath.Join(paths.BuildDir, "anaconda.ks")
	}

	token := askGitHubToken(uc)

	ui.Section("ISO Build")
	ui.Info("Source     : %s", srcImage)
	ui.Info("Output     : %s", outPath)
	ui.Info("Label      : %s", isoLabel)
	ui.Info("Mode       : %s", buildModeLabel(cfg))
	if cfg.Bootloader.Enabled {
		ui.Info("Bootloader : %s (custom)", cfg.ResolvedBootloader())
	}
	if ksPath != ""  { ui.Info("Kickstart  : %s", ksPath) }
	if *release      { ui.Info("Mode       : RELEASE") }
	ui.Newline()

	bISO := builder.NewISO(cfg, paths, verb, *release)

	steps := []namedStep{
		{"Validate project",   bISO.Validate},
		{"Prepare build dirs", bISO.PrepareDirs},
		{"Check tools",        bISO.CheckTools},
		{"Registry login",     func() error { return bISO.RegistryLogin(registryHost(cfg, uc), uc.GitHub.Name, token) }},
	}
	if cfg.Anaconda.Enabled {
		ks := ksPath
		steps = append(steps, namedStep{"Generate kickstart",
			func() error { return bISO.GenerateKickstart(ks) }})
	}
	src, out, lbl, ks := srcImage, outPath, isoLabel, ksPath
	steps = append(steps,
		       namedStep{"Pull source image", func() error { return bISO.PullImage(src) }},
		       namedStep{"Build ISO",         func() error { return bISO.BuildISO(src, out, lbl, ks) }},
		       namedStep{"Verify ISO",        func() error { return bISO.VerifyISO(out) }},
	)

	runSteps(steps, cfg, start, "", outPath)
}

// ── init ──────────────────────────────────────────────────────────────────────

func cmdInit(args []string) {
	fs := flag.NewFlagSet("init", flag.ExitOnError)
	fast := fs.Bool("fast", false, "Interactive wizard mode")
	fs.Parse(args) //nolint

	dir := "."
	if fs.NArg() > 0 {
		dir = fs.Arg(0)
	}

	ui.Banner()
	if *fast {
		runInitWizard(dir)
	} else {
		runInitDefault(dir)
	}
}

// ── clean ─────────────────────────────────────────────────────────────────────

func cmdClean(args []string) {
	fs := flag.NewFlagSet("clean", flag.ExitOnError)
	all := fs.Bool("all", false, "Remove entire build/ dir (uses sudo for root-owned files)")
	fs.Parse(args) //nolint

	ui.SmallBanner()
	ui.Section("Clean")

	paths := config.GetPaths(cwd())

	if !*all {
		targets := []string{paths.OutputDir}
		for _, t := range targets {
			if _, err := os.Stat(t); os.IsNotExist(err) {
				ui.Info("Already clean: %s", t)
				continue
			}
			if err := os.RemoveAll(t); err != nil {
				ui.Warn("Permission denied, retrying with sudo: %s", t)
				if sudoErr := sudoRemove(t); sudoErr != nil {
					ui.Fatal("Cannot remove %s: %v", t, sudoErr)
				}
			}
			ui.OK("Removed: %s", t)
		}
		os.MkdirAll(paths.OutputDir, 0755) //nolint
		os.MkdirAll(paths.CacheDir, 0755)  //nolint
		ui.OK("Clean complete")
		ui.Newline()
		return
	}

	buildDir := paths.BuildDir
	if _, err := os.Stat(buildDir); os.IsNotExist(err) {
		ui.Info("Already clean: %s", buildDir)
	} else {
		ui.Info("Removing build dir (sudo): %s", buildDir)
		if err := sudoRemove(buildDir); err != nil {
			ui.Fatal("Cannot remove %s: %v", buildDir, err)
		}
		ui.OK("Removed: %s", buildDir)
	}

	for _, d := range []string{paths.BuildDir, paths.OutputDir, paths.CacheDir} {
		if err := os.MkdirAll(d, 0755); err != nil {
			ui.Warn("Cannot recreate %s: %v", d, err)
		}
	}
	ui.OK("Build directories recreated")

	ui.Newline()
	ui.Info("Running: sudo podman system prune -a --volumes --force")
	pruneCmd := exec.Command("sudo", "podman", "system", "prune", "-a", "--volumes", "--force")
	pruneCmd.Stdout = os.Stdout
	pruneCmd.Stderr = os.Stderr
	pruneCmd.Stdin  = os.Stdin
	if err := pruneCmd.Run(); err != nil {
		ui.Warn("podman system prune returned error (may be harmless): %v", err)
	} else {
		ui.OK("podman system prune done")
	}

	ui.Newline()
	ui.OK("Full clean complete")
	ui.Newline()
}

func sudoRemove(path string) error {
	cmd := exec.Command("sudo", "rm", "-rf", path)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	cmd.Stdin  = os.Stdin
	return cmd.Run()
}

// ── setup ─────────────────────────────────────────────────────────────────────

func cmdSetup(args []string) {
	fs := flag.NewFlagSet("setup", flag.ExitOnError)
	release := fs.Bool("release", false, "Also install cosign")
	fs.Parse(args) //nolint

	ui.SmallBanner()
	ui.Section("Setup — Installing build dependencies")

	pkgs := []string{"podman", "buildah", "skopeo", "bootc-image-builder", "lorax"}
	if *release {
		pkgs = append(pkgs, "cosign")
	}
	ui.PackageListDisplay("Packages", pkgs)
	ui.Newline()

	dnfArgs := append([]string{"install", "-y"}, pkgs...)
	cmd := exec.Command("sudo", append([]string{"dnf"}, dnfArgs...)...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		ui.Fatal("dnf install failed: %v", err)
	}
	ui.OK("All dependencies installed")
	ui.Newline()
}

// cmdBuildRelease runs build cloud --push then build iso sequentially.
func cmdBuildRelease(args []string) {
	fs := flag.NewFlagSet("build --release", flag.ExitOnError)
	verbose := fs.Bool("verbose", false, "Verbose output")
	vShort  := fs.Bool("v", false, "Verbose output (short)")
	noCache := fs.Bool("no-cache", false, "Disable layer cache")
	fs.Parse(args) //nolint

	verb := *verbose || *vShort
	ui.SmallBanner()

	if os.Getuid() != 0 {
		ui.Section("Sudo required for ISO build")
		ui.Info("bootc-image-builder requires root access (rootful podman)")
		ui.Newline()

		self, err := os.Executable()
		if err != nil {
			ui.Fatal("cannot find own executable: %v", err)
		}

		sudoArgs := []string{"-E", self, "build", "--release"}
		if *verbose || *vShort {
			sudoArgs = append(sudoArgs, "--verbose")
		}
		if *noCache {
			sudoArgs = append(sudoArgs, "--no-cache")
		}

		cmd := exec.Command("sudo", sudoArgs...)
		cmd.Stdin  = os.Stdin
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr
		cmd.Env    = os.Environ()

		if err := cmd.Run(); err != nil {
			os.Exit(1)
		}
		os.Exit(0)
	}

	root := cwd()
	cfg, err := config.Load(root)
	if err != nil {
		ui.Fatal("%v", err)
	}
	uc, err := config.LoadUser(root)
	if err != nil {
		uc = &config.UserConfig{}
	}

	token := askGitHubToken(uc)

	tag      := imageTag(cfg, uc)
	reg      := registryHost(cfg, uc)
	username := uc.GitHub.Name

	ui.Section("Release Pipeline")
	ui.Info("Mode     : %s", buildModeLabel(cfg))
	ui.Info("Step 1/2 : build cloud --push")
	ui.Info("Step 2/2 : build iso")
	ui.Info("Image    : %s", tag)
	if cfg.Bootloader.Enabled {
		ui.Info("Bootloader: %s (custom)", cfg.ResolvedBootloader())
	}
	ui.Newline()

	ui.Section("Phase 1 — Cloud Build")
	start1 := time.Now()
	paths := config.GetPaths(root)
	b := builder.NewCloud(cfg, paths, verb, true)

	cloudSteps := []namedStep{
		{"Pre-scripts (host)",     func() error { return b.RunPreScripts() }},
		{"Validate project",       b.Validate},
		{"Prepare build dirs",     b.PrepareDirs},
		{"Registry login",         func() error { return b.RegistryLogin(reg, username, token) }},
		{"Copy repos",             b.CopyRepos},
		{"Generate Containerfile", func() error { return b.GenerateContainerfile("linux/amd64") }},
		{"podman build",           func() error { return b.PodmanBuild(tag, *noCache) }},
		{"podman push",            func() error { return b.PodmanPush(tag) }},
	}
	runSteps(cloudSteps, cfg, start1, tag, "")

	ui.Section("Phase 2 — ISO Build")
	start2 := time.Now()

	name := strings.ReplaceAll(cfg.Project.Name, " ", "-")
	fn := cfg.Build.ISOFilename
	if fn == "" {
		fn = fmt.Sprintf("%s-%s-fedora%d.x86_64.iso", name, cfg.Project.Version, cfg.Project.BaseVersion)
	}
	outPath  := filepath.Join(paths.OutputDir, fn)
	isoLabel := cfg.Build.ISOLabel
	if isoLabel == "" {
		isoLabel = strings.ToUpper(strings.ReplaceAll(cfg.Project.Name, " ", "_"))
	}
	ksPath := ""
	if cfg.Anaconda.Enabled {
		ksPath = filepath.Join(paths.BuildDir, "anaconda.ks")
	}

	bISO := builder.NewISO(cfg, paths, verb, true)

	isoSteps := []namedStep{
		{"Validate project",   bISO.Validate},
		{"Prepare build dirs", bISO.PrepareDirs},
		{"Check tools",        bISO.CheckTools},
		{"Registry login",     func() error { return bISO.RegistryLogin(reg, username, token) }},
	}
	if cfg.Anaconda.Enabled {
		ks := ksPath
		isoSteps = append(isoSteps, namedStep{"Generate kickstart",
			func() error { return bISO.GenerateKickstart(ks) }})
	}
	t, o, l, k := tag, outPath, isoLabel, ksPath
	isoSteps = append(isoSteps,
			  namedStep{"Pull source image", func() error { return bISO.PullImage(t) }},
			  namedStep{"Build ISO",         func() error { return bISO.BuildISO(t, o, l, k) }},
			  namedStep{"Verify ISO",        func() error { return bISO.VerifyISO(o) }},
	)
	runSteps(isoSteps, cfg, start2, "", outPath)
}

func registryHost(cfg *config.Config, uc *config.UserConfig) string {
	switch cfg.Container.Registry {
		case "", "custom":
			return "ghcr.io"
		case "repo":
			return "ghcr.io"
		default:
			parts := strings.SplitN(cfg.Container.Registry, "/", 2)
			return parts[0]
	}
}

func cmdValidate(_ []string) {
	ui.SmallBanner()
	ui.Section("Validate")

	root := cwd()
	cfg, err := config.Load(root)
	if err != nil {
		ui.Fatal("%v", err)
	}
	paths := config.GetPaths(root)

	type check struct {
		name string
		fn   func() error
	}
	checks := []check{
		{"config.toml syntax", func() error { return nil }},
		{"base_distro = fedora", func() error {
			if cfg.Project.BaseDistro != "fedora" {
				return fmt.Errorf("unsupported: %s", cfg.Project.BaseDistro)
			}
			return nil
		}},
		{"base_version >= 44", func() error {
			if cfg.Project.BaseVersion < 44 {
				return fmt.Errorf("Fedora %d too old (minimum: 44)", cfg.Project.BaseVersion)
			}
			return nil
		}},
		{"special_type", func() error {
			st := cfg.Project.SpecialType
			ui.Info("special_type = %s", st)
			return nil
		}},
		{"bootloader", func() error {
			if cfg.Bootloader.Enabled {
				bl := cfg.Bootloader.Type
				ui.Info("custom bootloader = %s", bl)
				validBLs := map[string]bool{
					"grub2": true, "grub": true,
					"refind":       true,
					"limine":       true,
					"systemd-boot": true,
					"syslinux":     true,
				}
				if !validBLs[strings.ToLower(bl)] {
					ui.Warn("bootloader type %q is custom/unknown — no automatic setup steps", bl)
				}
			} else {
				ui.Info("bootloader = %s (legacy boot section)", cfg.Boot.Bootloader)
			}
			return nil
		}},
		{"desktop environment", func() error {
			env := cfg.Desktop.Environment
			valid := map[string]bool{
				"gnome": true, "kde": true, "xfce": true,
				"cinnamon": true, "mate": true, "lxqt": true,
				"cosmic": true, "none": true, "": true,
			}
			if !valid[env] {
				return fmt.Errorf("unknown environment %q", env)
			}
			if env == "cosmic" {
				cosmicRepoPath := filepath.Join(paths.ReposDir, "cosmic.repo")
				if _, err := os.Stat(cosmicRepoPath); os.IsNotExist(err) {
					ui.Warn("repos/cosmic.repo not found — COSMIC packages won't install")
				} else {
					ui.Info("repos/cosmic.repo present")
				}
			}
			ui.Info("environment = %s", env)
			return nil
		}},
		{"packages/install.packages", func() error {
			pkgs, err := config.ReadPackageList(paths.InstallPkgs)
			if err != nil { return err }
			ui.Info("%d packages to install (DNF)", len(pkgs))
			return nil
		}},
		{"packages/remove.packages", func() error {
			pkgs, err := config.ReadPackageList(paths.RemovePkgs)
			if err != nil { return err }
			ui.Info("%d packages to remove (DNF)", len(pkgs))
			return nil
		}},
		{"packages/flatpak.packages", func() error {
			pkgs, err := config.ReadPackageList(paths.FlatpakPkgs)
			if err != nil { return err }
			ui.Info("%d Flatpak apps to install (at install time via Anaconda)", len(pkgs))
			return nil
		}},
		{"packages/flatpak.remove.packages", func() error {
			pkgs, err := config.ReadPackageList(paths.FlatpakRemovePkgs)
			if err != nil { return err }
			ui.Info("%d Flatpak apps to remove (at install time via Anaconda)", len(pkgs))
			return nil
		}},
		{"files/before/",   func() error { ui.Info("%d files", countFiles(paths.FilesBefore)); return nil }},
		{"files/after/",    func() error { ui.Info("%d files", countFiles(paths.FilesAfter)); return nil }},
		{"scripts/before/", func() error { ui.Info("%d scripts", countFiles(paths.ScriptsBefore)); return nil }},
		{"scripts/after/",  func() error { ui.Info("%d scripts", countFiles(paths.ScriptsAfter)); return nil }},
		{"repos/",          func() error { ui.Info("%d repos", countFiles(paths.ReposDir)); return nil }},
	}

	allOK := true
	for _, c := range checks {
		if err := c.fn(); err != nil {
			ui.Error("%s: %v", c.name, err)
			allOK = false
		} else {
			ui.OK("%s", c.name)
		}
	}
	ui.Newline()
	if !allOK {
		ui.Fatal("Validation failed")
	}
	ui.OK("All checks passed — ready to build")
	ui.Newline()
}

// ── info ──────────────────────────────────────────────────────────────────────

func cmdInfo(_ []string) {
	ui.SmallBanner()
	root := cwd()
	cfg, err := config.Load(root)
	if err != nil {
		ui.Fatal("%v", err)
	}
	paths := config.GetPaths(root)

	kv := func(k, v string) {
		if v == "" { return }
		fmt.Fprintf(ui.Out, "  \033[90m%-22s\033[0m \033[97;1m%s\033[0m\n", k+":", v)
	}

	ui.Section("Project")
	kv("Name",        cfg.Project.Name)
	kv("Version",     cfg.Project.Version)
	kv("Description", cfg.Project.Description)
	kv("Author",      cfg.Project.Author)
	kv("Base",        fmt.Sprintf("Fedora %d (x86_64)", cfg.Project.BaseVersion))
	kv("Build mode",  buildModeLabel(cfg))

	ui.Section("System")
	kv("Hostname", cfg.System.Hostname)
	kv("Locale",   cfg.System.Locale)
	kv("Timezone", cfg.System.Timezone)
	kv("Keyboard", cfg.System.Keyboard)
	kv("SELinux",  cfg.System.SELinux)

	ui.Section("Desktop")
	kv("Environment", cfg.Desktop.Environment)
	kv("Display",     cfg.Desktop.DisplayServer)

	ui.Section("Boot / Bootloader")
	kv("boot.bootloader", cfg.Boot.Bootloader)
	kv("boot.kernel_args", cfg.Boot.KernelArgs)
	if cfg.Bootloader.Enabled {
		kv("bootloader.type",       cfg.Bootloader.Type)
		kv("bootloader.extra_args", cfg.Bootloader.ExtraArgs)
		if cfg.Bootloader.EFIDir != "" {
			kv("bootloader.efi_dir", cfg.Bootloader.EFIDir)
		}
	} else {
		kv("bootloader section", "disabled (using boot.bootloader)")
	}

	ui.Section("Container / bootc")
	if cfg.Container.Enabled {
		uc2, _ := config.LoadUser(root)
		kv("Registry mode", cfg.Container.Registry)
		if cfg.Container.Repo != "" {
			kv("Repo", cfg.Container.Repo)
		}
		kv("Image tag",  imageTag(cfg, uc2))
		kv("Auto push",  fmt.Sprintf("%v", cfg.Container.Push))
	} else {
		ui.Info("Container build disabled")
	}

	if len(cfg.Branches) > 0 {
		ui.Section("Branches")
		for _, b := range cfg.Branches {
			status := "disabled"
			if b.Enabled { status = "enabled" }
			ui.Info("[%s] %s  desktop=%s  (%s)", b.Name, b.DisplayName, b.Desktop, status)
		}
	}

	ui.Section("Anaconda")
	if cfg.Anaconda.Enabled {
		kv("Product", cfg.Anaconda.ProductName)
		kv("WebUI",   fmt.Sprintf("%v", cfg.Anaconda.WebUI))
	} else {
		ui.Info("Disabled")
	}

	ui.Section("Package Lists")
	if pkgs, err := config.ReadPackageList(paths.InstallPkgs); err == nil && len(pkgs) > 0 {
		ui.PackageListDisplay("DNF Install", pkgs)
	}
	if pkgs, err := config.ReadPackageList(paths.RemovePkgs); err == nil && len(pkgs) > 0 {
		ui.PackageListDisplay("DNF Remove", pkgs)
	}
	if pkgs, err := config.ReadPackageList(paths.FlatpakPkgs); err == nil && len(pkgs) > 0 {
		ui.PackageListDisplay("Flatpak Install (at install time)", pkgs)
	}
	if pkgs, err := config.ReadPackageList(paths.FlatpakRemovePkgs); err == nil && len(pkgs) > 0 {
		ui.PackageListDisplay("Flatpak Remove (at install time)", pkgs)
	}
	ui.Newline()
}

// ── version ───────────────────────────────────────────────────────────────────

func cmdVersion() {
	ui.SmallBanner()
	fmt.Fprintf(ui.Out, "  \033[97;1mLegendaryOS Builder\033[0m  \033[96m%s\033[0m\n", Version)
	fmt.Fprintf(ui.Out, "  \033[90mCommit : %s\033[0m\n", Commit)
	fmt.Fprintf(ui.Out, "  \033[90mBuilt  : %s\033[0m\n", BuildDate)
	fmt.Fprintf(ui.Out, "  \033[90mFedora : 44+ (x86_64 only)\033[0m\n\n")
}

// ── helpers ───────────────────────────────────────────────────────────────────

func cwd() string {
	d, err := os.Getwd()
	if err != nil {
		ui.Fatal("cannot get working directory: %v", err)
	}
	return d
}

// buildModeLabel returns a human-readable build mode string.
func buildModeLabel(cfg *config.Config) string {
	if cfg.Project.IsClassic() {
		return "classic (plain Fedora, mutable)"
	}
	return "default (immutable, bootc/ostree)"
}

// cloudBaseImage returns the base image URL used in the Containerfile.
func cloudBaseImage(cfg *config.Config) string {
	if cfg.Project.IsClassic() {
		return fmt.Sprintf("registry.fedoraproject.org/fedora:%d", cfg.Project.BaseVersion)
	}
	return fmt.Sprintf("quay.io/fedora/fedora-bootc:%d", cfg.Project.BaseVersion)
}

func imageTag(cfg *config.Config, uc *config.UserConfig) string {
	reg := cfg.Container.Registry
	img := cfg.Container.Image
	tag := cfg.Container.Tag

	switch reg {
		case "", "custom":
			if uc != nil && uc.Registry() != "" {
				reg = uc.Registry()
			} else {
				reg = "ghcr.io/myorg"
			}
		case "repo":
			if cfg.Container.Repo != "" {
				if tag == "" { tag = cfg.Project.Version }
				if tag == "" { tag = "latest" }
				return strings.ToLower(fmt.Sprintf("ghcr.io/%s:%s", cfg.Container.Repo, tag))
			}
			if uc != nil && uc.Registry() != "" {
				reg = uc.Registry()
			} else {
				reg = "ghcr.io/myorg"
			}
	}

	if img == "" {
		img = strings.ReplaceAll(cfg.Project.Name, " ", "-")
	}
	if tag == "" { tag = cfg.Project.Version }
	if tag == "" { tag = "latest" }
	return strings.ToLower(fmt.Sprintf("%s/%s:%s", reg, img, tag))
}

func askGitHubToken(uc *config.UserConfig) string {
	if t := os.Getenv("GITHUB_TOKEN"); t != "" {
		ui.Info("Token: using $GITHUB_TOKEN from environment")
		return t
	}
	if uc != nil && uc.GitHub.Token != "" {
		ui.Info("Token: found in user.toml")
		return uc.GitHub.Token
	}
	ui.Newline()
	fmt.Fprintf(ui.Out, "  \033[93m⚠\033[0m  GitHub token required for registry access\n")
	fmt.Fprintf(ui.Out, "  \033[90mGet one at: https://github.com/settings/tokens\033[0m\n")
	fmt.Fprintf(ui.Out, "  \033[90mRequired scopes: write:packages, read:packages\033[0m\n")
	fmt.Fprintf(ui.Out, "  \033[90m(input hidden)\033[0m\n\n")
	fmt.Fprintf(ui.Out, "  \033[96m›\033[0m \033[97;1mGitHub Token\033[0m: ")

	token := readSecret()
	fmt.Fprintln(ui.Out)
	if token == "" {
		ui.Fatal("No token provided — cannot authenticate with ghcr.io")
	}
	return token
}

func readSecret() string {
	b, err := readNoEcho()
	if err == nil {
		return strings.TrimSpace(string(b))
	}
	var line string
	fmt.Scanln(&line)
	return strings.TrimSpace(line)
}

func countFiles(dir string) int {
	if _, err := os.Stat(dir); os.IsNotExist(err) {
		return 0
	}
	n := 0
	filepath.Walk(dir, func(_ string, info os.FileInfo, _ error) error { //nolint
		if info != nil && !info.IsDir() {
			n++
		}
		return nil
	})
	return n
}
