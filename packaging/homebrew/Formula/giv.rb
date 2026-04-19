# Homebrew formula for giv (source build). Copy this file into a tap repository, e.g.
#   github.com/cjp2600/homebrew-giv/Formula/giv.rb
# then: brew tap cjp2600/giv && brew install giv

class Giv < Formula
  desc "Terminal UI for reviewing git changes with a file tree and syntax-highlighted preview"
  homepage "https://github.com/cjp2600/giv"
  url "https://github.com/cjp2600/giv/archive/refs/tags/v1.0.4.tar.gz"
  sha256 "7f8dd0ad0be1c7ec6bb85a55964831d617b0edb127fba5248185d5b12095c3cf"
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
