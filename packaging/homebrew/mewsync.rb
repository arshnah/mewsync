class Mewsync < Formula
  desc "Sync your Discord status with the song you're playing, line by line"
  homepage "https://github.com/arshnah/mewsync"
  version "0.1.1"

  on_macos do
    on_arm do
      url "https://github.com/arshnah/mewsync/releases/download/v0.1.1/mewsync_0.1.1_darwin_arm64.tar.gz"
      sha256 "1736dad033d6778df29e9a4096291a739f6036ca8569dc3dc879158508629f5d"
    end
    on_intel do
      url "https://github.com/arshnah/mewsync/releases/download/v0.1.1/mewsync_0.1.1_darwin_amd64.tar.gz"
      sha256 "62b648b594c222aef8751369582562df9c03a6230efb8cdadb9d0bd50f4efccf"
    end
  end

  on_linux do
    on_arm do
      url "https://github.com/arshnah/mewsync/releases/download/v0.1.1/mewsync_0.1.1_linux_arm64.tar.gz"
      sha256 "c95f3ebc89ae53f9037d44dac45cd71e2e96a615f5017befb417d7868a65f8ff"
    end
    on_intel do
      url "https://github.com/arshnah/mewsync/releases/download/v0.1.1/mewsync_0.1.1_linux_amd64.tar.gz"
      sha256 "1b07b01aadfb365989f938e69b045cdea288d770ee18772ffd9dd7b142b4b018"
    end
  end

  def install
    bin.install "mewsync"
  end

  test do
    assert_match "mewsync", shell_output("#{bin}/mewsync version")
  end
end
