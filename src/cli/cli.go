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

func cmdBuild(args []string) {
	if len(args) == 0 {
		ui.SmallBanner()
		ui.Warn("Specify a target: 'build cloud' or 'build iso'")
		ui.Newline()
		fmt.Fprintln(ui.Out, "  Examples:")
		fmt.Fprintln(ui.Out, "    legendaryos build cloud")
		fmt.Fprintln(ui.Out, "    legendaryos build cloud --push")
		fmt.Fprintln(ui.Out, "    legendaryos build iso")
		fmt.Fprintln(ui.Out, "    legendaryos build iso --source ghcr.io/org/myos:latest")
		ui.Newline()
		os.Exit(1)
	}
	switch args[0] {
	case "cloud":
		cmdBuildCloud(args[1:])
	case "iso":
		cmdBuildISO(args[1:])
	default:
		ui.SmallBanner()
		ui.Error("Unknown build target %q — use 'cloud' or 'iso'", args[0])
		os.Exit(1)
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

	root := cwd()
	start := time.Now()

	cfg, err := config.Load(root)
	if err != nil {
		ui.Fatal("%v", err)
	}
	paths := config.GetPaths(root)
	tag := imageTag(cfg)

	ui.Section("Cloud Build")
	ui.Info("Project  : %s %s", cfg.Project.Name, cfg.Project.Version)
	ui.Info("Base     : quay.io/fedora/fedora-bootc:%d", cfg.Project.BaseVersion)
	ui.Info("Image    : %s", tag)
	ui.Info("Platform : %s", *platform)
	if *push    { ui.Info("Push     : yes") }
	if *release { ui.Info("Mode     : RELEASE") }
	ui.Newline()

	b := builder.NewCloud(cfg, paths, verb, *release)

	steps := []namedStep{
		{"Validate project",       b.Validate},
		{"Prepare build dirs",     b.PrepareDirs},
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

	root := cwd()
	start := time.Now()

	cfg, err := config.Load(root)
	if err != nil {
		ui.Fatal("%v", err)
	}
	paths := config.GetPaths(root)

	srcImage := *source
	if srcImage == "" {
		srcImage = imageTag(cfg)
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

	ui.Section("ISO Build")
	ui.Info("Source   : %s", srcImage)
	ui.Info("Output   : %s", outPath)
	ui.Info("Label    : %s", isoLabel)
	if ksPath != ""  { ui.Info("Kickstart: %s", ksPath) }
	if *release      { ui.Info("Mode     : RELEASE") }
	ui.Newline()

	bISO := builder.NewISO(cfg, paths, verb, *release)

	steps := []namedStep{
		{"Validate project",   bISO.Validate},
		{"Prepare build dirs", bISO.PrepareDirs},
		{"Check tools",        bISO.CheckTools},
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
	all := fs.Bool("all", false, "Remove entire build/ dir including cache")
	fs.Parse(args) //nolint

	ui.SmallBanner()
	ui.Section("Clean")

	paths := config.GetPaths(cwd())
	targets := []string{paths.OutputDir}
	if *all {
		targets = []string{paths.BuildDir}
	}
	for _, t := range targets {
		if _, err := os.Stat(t); os.IsNotExist(err) {
			ui.Info("Already clean: %s", t)
			continue
		}
		if err := os.RemoveAll(t); err != nil {
			ui.Fatal("Cannot remove %s: %v", t, err)
		}
		ui.OK("Removed: %s", t)
	}
	os.MkdirAll(paths.OutputDir, 0755) //nolint
	os.MkdirAll(paths.CacheDir, 0755)  //nolint
	ui.OK("Clean complete")
	ui.Newline()
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

// ── validate ──────────────────────────────────────────────────────────────────

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
		{"install.packages", func() error {
			pkgs, err := config.ReadPackageList(paths.InstallPkgs)
			if err != nil {
				return err
			}
			ui.Info("%d packages to install", len(pkgs))
			return nil
		}},
		{"remove.packages", func() error {
			pkgs, err := config.ReadPackageList(paths.RemovePkgs)
			if err != nil {
				return err
			}
			ui.Info("%d packages to remove", len(pkgs))
			return nil
		}},
		{"files/before/", func() error { ui.Info("%d files", countFiles(paths.FilesBefore)); return nil }},
		{"files/after/",  func() error { ui.Info("%d files", countFiles(paths.FilesAfter)); return nil }},
		{"scripts/",      func() error { ui.Info("%d scripts", countFiles(paths.ScriptsDir)); return nil }},
		{"repos/",        func() error { ui.Info("%d repos", countFiles(paths.ReposDir)); return nil }},
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
		if v == "" {
			return
		}
		fmt.Fprintf(ui.Out, "  \033[90m%-20s\033[0m \033[97;1m%s\033[0m\n", k+":", v)
	}

	ui.Section("Project")
	kv("Name", cfg.Project.Name)
	kv("Version", cfg.Project.Version)
	kv("Description", cfg.Project.Description)
	kv("Author", cfg.Project.Author)
	kv("Base", fmt.Sprintf("Fedora %d (x86_64)", cfg.Project.BaseVersion))

	ui.Section("System")
	kv("Hostname", cfg.System.Hostname)
	kv("Locale", cfg.System.Locale)
	kv("Timezone", cfg.System.Timezone)
	kv("Keyboard", cfg.System.Keyboard)
	kv("SELinux", cfg.System.SELinux)

	ui.Section("Desktop")
	kv("Environment", cfg.Desktop.Environment)
	kv("Display", cfg.Desktop.DisplayServer)

	ui.Section("Container / bootc")
	if cfg.Container.Enabled {
		kv("Registry", cfg.Container.Registry)
		kv("Image tag", imageTag(cfg))
		kv("Auto push", fmt.Sprintf("%v", cfg.Container.Push))
	} else {
		ui.Info("Container build disabled")
	}

	ui.Section("Anaconda")
	if cfg.Anaconda.Enabled {
		kv("Product", cfg.Anaconda.ProductName)
		kv("WebUI", fmt.Sprintf("%v", cfg.Anaconda.WebUI))
	} else {
		ui.Info("Disabled")
	}

	ui.Section("Package Lists")
	if pkgs, err := config.ReadPackageList(paths.InstallPkgs); err == nil && len(pkgs) > 0 {
		ui.PackageListDisplay("Install", pkgs)
	}
	if pkgs, err := config.ReadPackageList(paths.RemovePkgs); err == nil && len(pkgs) > 0 {
		ui.PackageListDisplay("Remove", pkgs)
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

func imageTag(cfg *config.Config) string {
	reg := cfg.Container.Registry
	img := cfg.Container.Image
	tag := cfg.Container.Tag
	if reg == "" {
		reg = "ghcr.io/myorg"
	}
	if img == "" {
		img = strings.ToLower(strings.ReplaceAll(cfg.Project.Name, " ", "-"))
	}
	if tag == "" {
		tag = cfg.Project.Version
	}
	if tag == "" {
		tag = "latest"
	}
	return fmt.Sprintf("%s/%s:%s", reg, img, tag)
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
