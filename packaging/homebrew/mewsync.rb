class Mewsync < Formula
  desc "Sync your Discord status with the song you're playing, line by line"
  homepage "https://github.com/arshnah/mewsync"
  version "0.1.0"

  on_macos do
    on_arm do
      url "https://github.com/arshnah/mewsync/releases/download/v0.1.0/mewsync_0.1.0_darwin_arm64.tar.gz"
      sha256 "PLACEHOLDER_SHA256_DARWIN_ARM64"
    end
    on_intel do
      url "https://github.com/arshnah/mewsync/releases/download/v0.1.0/mewsync_0.1.0_darwin_amd64.tar.gz"
      sha256 "PLACEHOLDER_SHA256_DARWIN_AMD64"
    end
  end

  on_linux do
    on_arm do
      url "https://github.com/arshnah/mewsync/releases/download/v0.1.0/mewsync_0.1.0_linux_arm64.tar.gz"
      sha256 "PLACEHOLDER_SHA256_LINUX_ARM64"
    end
    on_intel do
      url "https://github.com/arshnah/mewsync/releases/download/v0.1.0/mewsync_0.1.0_linux_amd64.tar.gz"
      sha256 "PLACEHOLDER_SHA256_LINUX_AMD64"
    end
  end

  def install
    bin.install "mewsync"
  end

  test do
    assert_match "mewsync", shell_output("#{bin}/mewsync version")
  end
end
