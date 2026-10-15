# Template for Formula/termbelt.rb in github.com/rahatsagor/homebrew-tap.
# scripts/homebrew.cjs fills in the version and checksums from a release's SHA256SUMS.
class Termbelt < Formula
  desc "Developer tool belt: network, domain and local utilities in one terminal"
  homepage "https://termbelt.rsapplication.com"
  license "MIT"

  livecheck do
    url :stable
    strategy :github_latest
  end

  on_macos do
    on_arm do
      url "https://github.com/rahatsagor/termbelt/releases/download/v{{VERSION}}/termbelt_{{VERSION}}_darwin_arm64.tar.gz"
      sha256 "{{SHA256_DARWIN_ARM64}}"
    end
    on_intel do
      url "https://github.com/rahatsagor/termbelt/releases/download/v{{VERSION}}/termbelt_{{VERSION}}_darwin_amd64.tar.gz"
      sha256 "{{SHA256_DARWIN_AMD64}}"
    end
  end

  on_linux do
    on_arm do
      url "https://github.com/rahatsagor/termbelt/releases/download/v{{VERSION}}/termbelt_{{VERSION}}_linux_arm64.tar.gz"
      sha256 "{{SHA256_LINUX_ARM64}}"
    end
    on_intel do
      url "https://github.com/rahatsagor/termbelt/releases/download/v{{VERSION}}/termbelt_{{VERSION}}_linux_amd64.tar.gz"
      sha256 "{{SHA256_LINUX_AMD64}}"
    end
  end

  def install
    bin.install "termbelt"
    prefix.install "THIRD_PARTY_NOTICES"
    generate_completions_from_executable(bin/"termbelt", "completion")
  end

  test do
    assert_equal "termbelt version #{version}", shell_output("#{bin}/termbelt --version").strip
    assert_match(/\A\h{8}-\h{4}-4\h{3}-[89ab]\h{3}-\h{12}\z/, shell_output("#{bin}/termbelt uuid --raw").strip)
    assert_equal "aGVsbG8=", pipe_output("#{bin}/termbelt base64 --raw", "hello").strip
  end
end
