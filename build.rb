require "open3"
require "fileutils"
require "time"

BINARY    = "legendaryos-builder"
DIST_DIR  = "dist"
INSTALL   = "/usr/local/bin/#{BINARY}"

# ── Helpers ───────────────────────────────────────────────────────────────────

def cyan(s)    = "\033[96m#{s}\033[0m"
def green(s)   = "\033[92m#{s}\033[0m"
def yellow(s)  = "\033[93m#{s}\033[0m"
def bold(s)    = "\033[97;1m#{s}\033[0m"
def gray(s)    = "\033[90m#{s}\033[0m"

def info(msg)  = $stderr.puts("  #{cyan("⬡")}  #{bold(msg)}")
def ok(msg)    = $stderr.puts("  #{green("✓")}  #{msg}")
def warn(msg)  = $stderr.puts("  #{yellow("⚠")}  #{msg}")

def run!(cmd, env: {})
  merged = ENV.to_h.merge(env.transform_keys(&:to_s))
  out, err, status = Open3.capture3(merged, cmd)
  $stdout.print(out) unless out.empty?
  $stderr.print(err) unless err.empty?
  abort "Command failed: #{cmd}" unless status.success?
end

def shell_capture(cmd)
  out, _err, status = Open3.capture3(cmd)
  status.success? ? out.strip : nil
end

# ── Version metadata (mirrors Makefile logic) ─────────────────────────────────

def version
  shell_capture("git describe --tags --always --dirty 2>/dev/null") || "v0.4.0"
end

def commit
  shell_capture("git rev-parse --short HEAD 2>/dev/null") || "unknown"
end

def build_date
  Time.now.utc.strftime("%Y-%m-%dT%H:%M:%SZ")
end

def ldflags
  v, c, d = version, commit, build_date
  "-s -w -X main.Version=#{v} -X main.Commit=#{c} -X main.BuildDate=#{d}"
end

GOFLAGS = "-trimpath"

# ── Tasks ─────────────────────────────────────────────────────────────────────

def task_build
  info "Building #{BINARY} #{version} ..."
  run! "go build #{GOFLAGS} -ldflags \"#{ldflags}\" -o #{BINARY} ."
  ok "#{BINARY} ready"
end

def task_release
  task_fmt
  task_vet
  info "Building release binary (linux/amd64) ..."
  FileUtils.mkdir_p(DIST_DIR)
  out_bin = File.join(DIST_DIR, BINARY)
  run!(
    "go build #{GOFLAGS} -ldflags \"#{ldflags}\" -o #{out_bin} .",
    env: { GOOS: "linux", GOARCH: "amd64", CGO_ENABLED: "0" }
  )
  tarball = "#{BINARY}-linux-amd64.tar.gz"
  Dir.chdir(DIST_DIR) do
    run! "tar -czf #{tarball} #{BINARY}"
    checksum = shell_capture("sha256sum #{tarball}")
    File.write("checksums.sha256", checksum + "\n")
    ok "#{DIST_DIR}/#{tarball}"
    $stderr.puts gray("  " + checksum.to_s)
  end
end

def task_install
  task_build
  info "Installing to #{INSTALL} ..."
  run! "sudo install -m 0755 #{BINARY} #{INSTALL}"
  ok "Installed"
end

def task_fmt
  info "go fmt ..."
  run! "go fmt ./..."
  ok "fmt done"
end

def task_vet
  info "go vet ..."
  run! "go vet ./..."
  ok "vet done"
end

def task_tidy
  info "go mod tidy ..."
  run! "go mod tidy"
  ok "tidy done"
end

def task_test
  info "go test ./... ..."
  run! "go test ./... -v -count=1"
  ok "tests passed"
end

def task_clean
  info "Cleaning ..."
  FileUtils.rm_f(BINARY)
  FileUtils.rm_rf(DIST_DIR)
  ok "Clean"
end

def task_help
  $stderr.puts ""
  $stderr.puts "  #{cyan("⬡")} #{bold("LegendaryOS Builder — build.rb")}"
  $stderr.puts ""
  $stderr.puts "  #{bold("Usage:")} ruby build.rb [task]"
  $stderr.puts ""
  rows = [
    ["(no args) / build", "build #{BINARY} binary"],
    ["release",           "build dist/#{BINARY}-linux-amd64.tar.gz + checksum"],
    ["install",           "sudo install to #{INSTALL}"],
    ["fmt",               "go fmt ./..."],
    ["vet",               "go vet ./..."],
    ["tidy",              "go mod tidy"],
    ["test",              "run tests"],
    ["clean",             "remove binary and #{DIST_DIR}/"],
    ["help",              "show this help"],
  ]
  rows.each do |cmd, desc|
    $stderr.puts "    #{cyan("ruby build.rb %-16s" % cmd)}  #{gray(desc)}"
  end
  $stderr.puts ""
end

# ── Dispatch ──────────────────────────────────────────────────────────────────

task = ARGV[0] || "build"

case task
when "build"    then task_build
when "release"  then task_release
when "install"  then task_install
when "fmt"      then task_fmt
when "vet"      then task_vet
when "tidy"     then task_tidy
when "test"     then task_test
when "clean"    then task_clean
when "help", "--help", "-h" then task_help
else
  warn "Unknown task: #{task.inspect}"
  task_help
  exit 1
end
