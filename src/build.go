package commands

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/legendaryos/builder/src/builder"
	"github.com/legendaryos/builder/src/config"
	"github.com/legendaryos/builder/src/ui"
	"github.com/spf13/cobra"
)

// ── build (parent) ────────────────────────────────────────────────────────────

var buildCmd = &cobra.Command{
	Use:   "build",
	Short: "Build targets (cloud | iso)",
	Long: `Build LegendaryOS targets.

  legendaryos build cloud   — build a bootc container image (for GitHub Actions / registry)
  legendaryos build iso     — build a bootable ISO from a bootc container image`,
	RunE: func(cmd *cobra.Command, args []string) error {
		ui.Banner()
		ui.Warn("Specify a target: 'build cloud' or 'build iso'")
		cmd.Help() //nolint:errcheck
		return nil
	},
}

func init() {
	buildCmd.AddCommand(buildCloudCmd)
	buildCmd.AddCommand(buildISOCmd)
}

// ── build cloud ───────────────────────────────────────────────────────────────

var (
	cloudFlagPush     bool
	cloudFlagSign     bool
	cloudFlagNoCache  bool
	cloudFlagPlatform string
)

var buildCloudCmd = &cobra.Command{
	Use:   "cloud",
	Short: "Build bootc container image and push to registry",
	Long: `Build a bootc-compatible OCI container image from your project.

The image is built with podman/buildah and can be pushed to any OCI registry.
This is the target used in GitHub Actions CI/CD pipelines.

Reads:
  config.toml         → image name, registry, tag, base version
  install.packages    → packages to install in the container
  remove.packages     → packages to remove
  files/before/       → overlay applied before package install
  files/after/        → overlay applied after package install
  scripts/            → hooks run inside the container
  repos/              → additional DNF repo files`,
	RunE: func(cmd *cobra.Command, args []string) error {
		ui.SmallBanner()
		return runBuildCloud(cwd(), flagVerbose, flagRelease, cloudFlagPush, cloudFlagSign, cloudFlagNoCache, cloudFlagPlatform)
	},
}

func init() {
	buildCloudCmd.Flags().BoolVar(&cloudFlagPush, "push", false, "Push image to registry after build")
	buildCloudCmd.Flags().BoolVar(&cloudFlagSign, "sign", false, "Sign image with sigstore/cosign after push")
	buildCloudCmd.Flags().BoolVar(&cloudFlagNoCache, "no-cache", false, "Disable layer cache (fresh build)")
	buildCloudCmd.Flags().StringVar(&cloudFlagPlatform, "platform", "linux/amd64", "Target platform (linux/amd64, linux/arm64)")
}

func runBuildCloud(root string, verbose, release, push, sign, noCache bool, platform string) error {
	start := time.Now()

	cfg, err := config.Load(root)
	if err != nil {
		ui.Fatal("%v", err)
	}
	paths := config.GetPaths(root)

	// ── print plan ────────────────────────────────────────────────────────────
	ui.Section("Cloud Build Plan")
	tag := buildImageTag(cfg)
	ui.Info("Project   : %s %s", cfg.Project.Name, cfg.Project.Version)
	ui.Info("Base      : fedora-bootc:%d", cfg.Project.BaseVersion)
	ui.Info("Image tag : %s", tag)
	ui.Info("Platform  : %s", platform)
	ui.Info("Push      : %v", push)
	if release {
		ui.Info("Mode      : RELEASE")
	}
	ui.Newline()

	b := builder.NewCloud(cfg, paths, verbose, release)

	var steps []func() error
	var stepNames []string

	add := func(name string, fn func() error) {
		stepNames = append(stepNames, name)
		steps = append(steps, fn)
	}

	add("Validate project", func() error { return b.Validate() })
	add("Prepare build dir", func() error { return b.PrepareDirs() })
	add("Generate Containerfile", func() error { return b.GenerateContainerfile(platform) })
	add("Copy repos into context", func() error { return b.CopyRepos() })
	add("Build container image", func() error { return b.PodmanBuild(tag, noCache) })
	if push {
		add("Push to registry", func() error { return b.PodmanPush(tag) })
	}
	if sign && push {
		add("Sign image (cosign)", func() error { return b.CosignSign(tag) })
	}

	// ── execute ───────────────────────────────────────────────────────────────
	bar := ui.NewProgressBar(len(steps), "cloud build")
	var summary []ui.SummaryStep

	for i, fn := range steps {
		bar.Set(i)
		ui.Step(stepNames[i])
		if err := fn(); err != nil {
			bar.Done()
			summary = append(summary, ui.SummaryStep{Name: stepNames[i], Status: "fail", Detail: err.Error()})
			ui.PrintBuildSummary(&ui.BuildSummary{
				ProjectName: cfg.Project.Name,
				Version:     cfg.Project.Version,
				Distro:      fmt.Sprintf("Fedora %d", cfg.Project.BaseVersion),
				Steps:       summary,
				Duration:    time.Since(start),
			})
			ui.Fatal("%v", err)
		}
		summary = append(summary, ui.SummaryStep{Name: stepNames[i], Status: "ok"})
	}
	bar.Done()

	ui.PrintBuildSummary(&ui.BuildSummary{
		ProjectName: cfg.Project.Name,
		Version:     cfg.Project.Version,
		Distro:      fmt.Sprintf("Fedora %d", cfg.Project.BaseVersion),
		Steps:       summary,
		Duration:    time.Since(start),
	})
	return nil
}

// ── build iso ─────────────────────────────────────────────────────────────────

var (
	isoFlagSourceImage string
	isoFlagKickstart   string
	isoFlagLabel       string
	isoFlagOutput      string
)

var buildISOCmd = &cobra.Command{
	Use:   "iso",
	Short: "Build a bootable ISO from a bootc container image",
	Long: `Convert a bootc container image into a bootable hybrid ISO image.

Uses bootc-image-builder (or lorax as fallback) to produce a standard
El Torito / hybrid ISO that can be written to USB or used in VMs.

The source image can be:
  - a local container image (built with 'legendaryos build cloud')
  - a remote registry reference (e.g. ghcr.io/myorg/myos:latest)

Examples:
  legendaryos build iso
  legendaryos build iso --source ghcr.io/myorg/myos:latest
  legendaryos build iso --source ghcr.io/myorg/myos:latest --output ./dist/myos.iso`,
	RunE: func(cmd *cobra.Command, args []string) error {
		ui.SmallBanner()
		return runBuildISO(cwd(), flagVerbose, flagRelease, isoFlagSourceImage, isoFlagKickstart, isoFlagLabel, isoFlagOutput)
	},
}

func init() {
	buildISOCmd.Flags().StringVar(&isoFlagSourceImage, "source", "", "Source container image (default: from config.toml)")
	buildISOCmd.Flags().StringVar(&isoFlagKickstart, "kickstart", "", "Custom kickstart file path")
	buildISOCmd.Flags().StringVar(&isoFlagLabel, "label", "", "ISO volume label (default: from config.toml)")
	buildISOCmd.Flags().StringVar(&isoFlagOutput, "output", "", "Output ISO path (default: build/output/<name>.iso)")
}

func runBuildISO(root string, verbose, release bool, sourceImage, kickstart, label, output string) error {
	start := time.Now()

	cfg, err := config.Load(root)
	if err != nil {
		ui.Fatal("%v", err)
	}
	paths := config.GetPaths(root)

	// Resolve source image
	if sourceImage == "" {
		sourceImage = buildImageTag(cfg)
	}

	// Resolve output path
	if output == "" {
		name := strings.ReplaceAll(cfg.Project.Name, " ", "-")
		filename := cfg.Build.ISOFilename
		if filename == "" {
			filename = fmt.Sprintf("%s-%s-fedora%d.%s.iso",
				name, cfg.Project.Version,
				cfg.Project.BaseVersion, cfg.Project.Arch)
		}
		output = filepath.Join(paths.OutputDir, filename)
	}

	// Resolve label
	if label == "" {
		label = cfg.Build.ISOLabel
	}
	if label == "" {
		label = strings.ToUpper(strings.ReplaceAll(cfg.Project.Name, " ", "_"))
	}

	// Resolve kickstart
	if kickstart == "" && cfg.Anaconda.Enabled {
		kickstart = filepath.Join(paths.BuildDir, "anaconda.ks")
	}

	// ── print plan ────────────────────────────────────────────────────────────
	ui.Section("ISO Build Plan")
	ui.Info("Source image  : %s", sourceImage)
	ui.Info("Output        : %s", output)
	ui.Info("Label         : %s", label)
	ui.Info("Compression   : %s", cfg.Build.Compression)
	if kickstart != "" {
		ui.Info("Kickstart     : %s", kickstart)
	}
	ui.Newline()

	b := builder.NewISO(cfg, paths, verbose, release)

	var steps []func() error
	var stepNames []string

	add := func(name string, fn func() error) {
		stepNames = append(stepNames, name)
		steps = append(steps, fn)
	}

	add("Validate project", func() error { return b.Validate() })
	add("Prepare build dir", func() error { return b.PrepareDirs() })
	add("Check bootc-image-builder", func() error { return b.CheckTools() })
	if cfg.Anaconda.Enabled {
		add("Generate kickstart", func() error { return b.GenerateKickstart(kickstart) })
	}
	add("Pull source image", func() error { return b.PullImage(sourceImage) })
	add("Run bootc-image-builder", func() error { return b.BuildISO(sourceImage, output, label, kickstart) })
	add("Verify ISO", func() error { return b.VerifyISO(output) })

	// ── execute ───────────────────────────────────────────────────────────────
	bar := ui.NewProgressBar(len(steps), "iso build")
	var summary []ui.SummaryStep

	for i, fn := range steps {
		bar.Set(i)
		ui.Step(stepNames[i])
		if err := fn(); err != nil {
			bar.Done()
			summary = append(summary, ui.SummaryStep{Name: stepNames[i], Status: "fail", Detail: err.Error()})
			ui.PrintBuildSummary(&ui.BuildSummary{
				ProjectName: cfg.Project.Name,
				Version:     cfg.Project.Version,
				Distro:      fmt.Sprintf("Fedora %d", cfg.Project.BaseVersion),
				Steps:       summary,
				Duration:    time.Since(start),
			})
			ui.Fatal("%v", err)
		}
		summary = append(summary, ui.SummaryStep{Name: stepNames[i], Status: "ok"})
	}
	bar.Done()

	isoOut := output
	if _, err := os.Stat(output); os.IsNotExist(err) {
		isoOut = "" // build was simulated
	}

	ui.PrintBuildSummary(&ui.BuildSummary{
		ProjectName: cfg.Project.Name,
		Version:     cfg.Project.Version,
		Distro:      fmt.Sprintf("Fedora %d", cfg.Project.BaseVersion),
		Steps:       summary,
		ISOPath:     isoOut,
		Duration:    time.Since(start),
	})
	return nil
}

// buildImageTag returns the full OCI image tag from config
func buildImageTag(cfg *config.Config) string {
	registry := cfg.Container.Registry
	image := cfg.Container.Image
	tag := cfg.Container.Tag

	if registry == "" {
		registry = "ghcr.io/myorg"
	}
	if image == "" {
		image = strings.ToLower(strings.ReplaceAll(cfg.Project.Name, " ", "-"))
	}
	if tag == "" {
		tag = cfg.Project.Version
	}
	if tag == "" {
		tag = "latest"
	}
	return fmt.Sprintf("%s/%s:%s", registry, image, tag)
}
