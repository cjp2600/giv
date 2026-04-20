# Homebrew formula for giv (source build). Copy this file into a tap repository, e.g.
#   github.com/cjp2600/homebrew-giv/Formula/giv.rb
# then: brew tap cjp2600/giv && brew install giv

class Giv < Formula
  desc "Terminal UI for reviewing git changes with a file tree and syntax-highlighted preview"
  homepage "https://github.com/cjp2600/giv"
  url "https://github.com/cjp2600/giv/archive/refs/tags/v1.0.11.tar.gz"
  sha256 "798ae57e8b50a90728df8e597dcb82ee603d93aa037661c7311bdab81af9b25e"
  license "MIT"
  head "https://github.com/cjp2600/giv.git", branch: "main"

  depends_on "go" => :build

  def install
    system "go", "build", *std_go_args, "./cmd/giv"
  end

  test do
    output = shell_output("#{bin}/giv --hotkey")
    assert_match "Tab", output
  end
end
