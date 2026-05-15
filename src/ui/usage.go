package ui

import "fmt"

func PrintUsage(version string) {
	Banner()
	fmt.Fprintf(Out, "  \033[90mVersion: %s\033[0m\n\n", version)
	fmt.Fprintf(Out, "  \033[97;1mUsage:\033[0m\n")
	fmt.Fprintf(Out, "    legendaryos <command> [flags]\n\n")

	type entry struct{ cmd, desc string }
	cmds := []entry{
		{"build cloud",                    "Build bootc container image with podman"},
		{"build cloud --push",             "Build and push to ghcr.io (asks for token)"},
		{"build cloud --push --sign",      "Build, push and sign with cosign"},
		{"build cloud --no-cache",         "Disable layer cache (fresh build)"},
		{"build cloud --release",          "Release mode: squash layers, strip debug"},
		{"build iso",                      "Build bootable ISO from container image"},
		{"build iso --source <img>",       "ISO from a specific container image"},
		{"build iso --output <path>",      "Write ISO to a specific path"},
		{"build --release",                "Full pipeline: cloud --push + iso (asks for token once)"},
		{"settings",                          "Interactive TUI config editor (saves config.toml)"},
		{"", ""},
		{"init [dir]",                     "Initialize project scaffold in dir"},
		{"init --fast",                    "Interactive wizard (asks questions)"},
		{"validate",                       "Validate project config without building"},
		{"info",                           "Show project info and package lists"},
		{"clean",                          "Remove build/output/"},
		{"clean --all",                    "Remove entire build/ directory"},
		{"setup",                          "Install host build dependencies (Fedora)"},
		{"setup --release",                "Also install cosign"},
		{"version",                        "Show builder version"},
	}

	fmt.Fprintf(Out, "  \033[97;1mCommands:\033[0m\n")
	for _, e := range cmds {
		if e.cmd == "" {
			fmt.Fprintln(Out)
			continue
		}
		fmt.Fprintf(Out, "    \033[96m%-38s\033[0m \033[90m%s\033[0m\n", "legendaryos "+e.cmd, e.desc)
	}

	fmt.Fprintf(Out, "\n  \033[97;1mGlobal flags:\033[0m\n")
	fmt.Fprintf(Out, "    \033[96m--verbose, -v\033[0m  Show full command output\n")
	fmt.Fprintf(Out, "    \033[96m--release\033[0m      Release mode\n\n")

	fmt.Fprintf(Out, "  \033[97;1mHow it works:\033[0m\n")
	fmt.Fprintf(Out, "    \033[90mbuild cloud\033[0m  →  generates Containerfile  →  podman build  →  (podman push)\n")
	fmt.Fprintf(Out, "    \033[90mbuild iso\033[0m    →  bootc-image-builder (podman)  →  .iso\n\n")
}
