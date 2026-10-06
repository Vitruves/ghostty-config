class GhosttyConfig < Formula
  desc "Terminal UI for Ghostty themes, fonts and window settings"
  homepage "https://github.com/Vitruves/ghostty-config"
  url "https://github.com/Vitruves/ghostty-config/archive/refs/tags/0.1.0.tar.gz"
  sha256 "30e8170ab04458262093a9c620787dcc89dc15f823cd6fd5b5c8e639953573b5"
  license "MIT"
  head "https://github.com/Vitruves/ghostty-config.git", branch: "main"

  depends_on "go" => :build

  def install
    # The 0.1.0 tag hard-codes 1.0.0 as a constant. Allow the linker to set
    # the release version, as it does for newer sources.
    if build.stable? && version == "0.1.0"
      inreplace "cmd/ghostty-config/main.go", 'const version = "1.0.0"', 'var version = "1.0.0"'
    end

    system "go", "build", *std_go_args(ldflags: "-X main.version=#{version}"), "./cmd/ghostty-config"
  end

  test do
    output = shell_output("#{bin}/ghostty-config -version")
    assert_match "ghostty-config", output
    assert_match version.to_s, output unless build.head?

    # Exercise the embedded themes without a terminal or Ghostty installed.
    themes = testpath/"themes"
    system bin/"ghostty-config", "-export-collection", themes
    assert_path_exists themes/"powershell"
    assert_match "background =", (themes/"powershell").read

    # Resolve an explicit config without touching the user's real settings.
    config = testpath/"config.ghostty"
    config.write "font-size = 14\n"
    output = shell_output("#{bin}/ghostty-config -config #{config} -themes #{themes} -paths")
    assert_match config.to_s, output
    assert_match themes.to_s, output
    assert_equal "font-size = 14\n", config.read
  end
end
