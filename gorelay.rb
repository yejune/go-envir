class Gorelay < Formula
  desc "Description of your project"
  homepage "https://github.com/yejune/gorelay"
  url "https://github.com/yejune/gorelay/archive/refs/tags/v0.0.2.tar.gz"
  sha256 "7be0b992a098c86dac808b1c58f6132a8d325867f28e8ece182c17b0b63c5f09"
  license "MIT"
  head "https://github.com/yejune/gorelay.git", branch: "main"

  depends_on "go" => :build

  def install
        system "go", "build", "."
    bin.install "gorelay"
  end

  def test
        assert_match "gorelay", shell_output("#{bin}/gorelay --version")
  end

  def caveats
    <<~EOS
            gorelay has been installed!
      
      Run 'gorelay --help' to get started.
    EOS
  end
end
