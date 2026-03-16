class Dibs < Formula
  desc "Check if a domain name is up for grabs across 1400+ TLDs"
  homepage "https://github.com/sitapix/dibs"
  url "https://github.com/sitapix/dibs/archive/refs/tags/v0.1.0.tar.gz"
  sha256 "PLACEHOLDER"
  license "MIT"
  head "https://github.com/sitapix/dibs.git", branch: "main"

  depends_on "go" => :build

  def install
    system "go", "build", *std_go_args(ldflags: "-s -w -X main.version=#{version}")
  end

  test do
    assert_match "dibs #{version}", shell_output("#{bin}/dibs --version")
    assert_match "Usage:", shell_output("#{bin}/dibs --help")
  end
end
