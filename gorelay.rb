class Gorelay < Formula
  desc "Description of your project"
  homepage "https://github.com/yejune/gorelay"
  url "https://github.com/yejune/gorelay/archive/refs/tags/v0.0.1.tar.gz"
  sha256 "b6c7c20428e451df1148627c67b5baeb3fc137c353ca63aa6ac169b56a80aab5"
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
