# Homebrew formula for giv (source build). Copy this file into a tap repository, e.g.
#   github.com/cjp2600/homebrew-giv/Formula/giv.rb
# then: brew tap cjp2600/giv && brew install giv

class Giv < Formula
  desc "Terminal UI for reviewing git changes with a file tree and syntax-highlighted preview"
  homepage "https://github.com/cjp2600/giv"
  url "https://github.com/cjp2600/giv/archive/refs/tags/v1.0.6.tar.gz"
  sha256 "0f31345427c9ec1aa0af95ed024bb21111483c27924452eb0c58769c6b9f4385"
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
